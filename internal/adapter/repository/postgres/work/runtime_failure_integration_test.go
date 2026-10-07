package work_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	workhttp "local/rag-project/internal/adapter/http/work"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	ragservice "local/rag-project/internal/app/rag/service"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/stream"
)

func TestHTTPToolSuccessSurvivesModelOrAssistantPersistenceFailure(t *testing.T) {
	for _, databaseFailure := range []bool{false, true} {
		t.Run(fmt.Sprintf("assistant_database_failure_%t", databaseFailure), func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			user, key := testIdentity(t)
			work, err := workbootstrap.NewRuntime(db)
			if err != nil {
				t.Fatal(err)
			}
			topic, err := work.Store.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "执行结果恢复"})
			if err != nil {
				t.Fatal(err)
			}
			model := &failureAfterWriteModel{databaseFailure: databaseFailure}
			var memoryLoads atomic.Int32
			base := &conversationruntime.Runtime{Model: model, Tools: capability.NewRegistry(), Lifecycle: conversationruntime.NewLifecycle(postgresruntime.NewStore(db), func() (string, error) { id, err := distributedid.NextID(); return strconv.FormatInt(id, 10), err }), CoreMemoryContextLoader: func(context.Context, string) (string, error) { memoryLoads.Add(1); return "GLOBAL_SCOPE_SENTINEL", nil }}
			convRepo := postgresrag.NewConversationRepository(db)
			msgRepo := postgresrag.NewConversationMessageRepository(db)
			summaryRepo := postgresrag.NewConversationSummaryRepository(db)
			conv := ragservice.NewConversationService(convRepo, msgRepo, summaryRepo, nil, nil, 30, postgresrag.NewConversationDeleteTransaction(db))
			messages := ragservice.NewConversationMessageService(convRepo, msgRepo, summaryRepo, nil)
			if databaseFailure {
				callback := "work/test/assistant-save-failure"
				if err = db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if tx.Statement.Table != "t_message" || tx.Statement.Schema == nil {
						return
					}
					field := tx.Statement.Schema.LookUpField("Role")
					if field == nil {
						return
					}
					role, _ := field.ValueOf(tx.Statement.Context, tx.Statement.ReflectValue)
					if role == "assistant" {
						tx.AddError(errors.New("injected assistant database failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { db.Callback().Create().Remove(callback) })
			}
			if err = work.ConfigureChat(base, runtimeadapter.NewConversations(conv), runtimeadapter.NewConversationMessages(messages), stream.NewMemoryStreamManager()); err != nil {
				t.Fatal(err)
			}
			engine := gin.New()
			engine.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user, Role: "user"}); c.Next() })
			workhttp.RegisterRoutes(engine, work.Service)
			workhttp.RegisterChatRoutes(engine, work)
			server := httptest.NewServer(engine)
			defer server.Close()
			input := domain.ChatRequest{RequestID: key + "input", Question: "新建文档", Action: "create_document"}
			raw, _ := json.Marshal(input)
			response, err := http.Post(server.URL+"/work/topics/"+topic.ID+"/chat", "application/json", bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			wire, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			match := regexp.MustCompile(`"taskId":"([^"]+)"`).FindSubmatch(wire)
			if len(match) != 2 {
				t.Fatalf("missing meta: %s", wire)
			}
			turnID := string(match[1])
			var turn domain.Turn
			for attempt := 0; attempt < 100; attempt++ {
				turn, err = work.Store.GetTurn(ctx, user, topic.ID, turnID)
				if err != nil {
					t.Fatal(err)
				}
				if turn.Status != "running" {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			if turn.Status != "failed" || len(turn.Outputs) != 1 || turn.Outputs[0].ArtifactID == "" {
				t.Fatalf("missing durable failure results: %+v", turn)
			}
			artifact, err := work.Store.GetArtifact(ctx, user, topic.ID, turn.Outputs[0].ArtifactID)
			if err != nil || artifact.Version.Body.Text() != "已保存的正文" {
				t.Fatal("write was lost", err)
			}
			history, err := work.Store.ListMessages(ctx, user, topic.ID, turn.ConversationID, domain.Page{Limit: 30})
			if err != nil || len(history) != 1 || history[0].Role != "user" {
				t.Fatalf("unexpected assistant persisted: %+v %v", history, err)
			}
			// Rebuild the bootstrap and SSE cache from persisted runtime facts. No model
			// calls or document writes are repeated, even when the original reply failed.
			if databaseFailure {
				// The final answer is now pending publication. Restore the database
				// before reconnect so recovery can save it without repeating writes.
				db.Callback().Create().Remove("work/test/assistant-save-failure")
			}
			replacement, err := workbootstrap.NewRuntime(db)
			if err != nil {
				t.Fatal(err)
			}
			if err = replacement.ConfigureChat(base, runtimeadapter.NewConversations(conv), runtimeadapter.NewConversationMessages(messages), stream.NewMemoryStreamManager()); err != nil {
				t.Fatal(err)
			}
			recovered := gin.New()
			recovered.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user, Role: "user"}); c.Next() })
			workhttp.RegisterChatRoutes(recovered, replacement)
			fresh := httptest.NewServer(recovered)
			defer fresh.Close()
			replayed, err := http.Get(fresh.URL + "/work/topics/" + topic.ID + "/turns/" + turnID + "/stream?offset=999999")
			if err != nil {
				t.Fatal(err)
			}
			events, err := io.ReadAll(replayed.Body)
			replayed.Body.Close()
			if err != nil || !bytes.Contains(events, []byte("done")) {
				t.Fatalf("terminal replay: %s %v", events, err)
			}
			if databaseFailure {
				if !bytes.Contains(events, []byte("event: finish")) {
					t.Fatalf("pending answer was not published: %s", events)
				}
				recoveredTurn, err := replacement.Store.GetTurn(ctx, user, topic.ID, turnID)
				if err != nil || recoveredTurn.Status != "completed" || recoveredTurn.AssistantMessageID == "" {
					t.Fatalf("turn publication not atomic: %+v %v", recoveredTurn, err)
				}
			}
			if model.calls.Load() != 2 || memoryLoads.Load() != 0 {
				t.Fatalf("replay ran model or global memory: %d %d", model.calls.Load(), memoryLoads.Load())
			}
			again, err := work.Store.AcceptTurn(ctx, user, topic.ID, input)
			if err != nil || again.ID != turnID {
				t.Fatal("input replay duplicated execution", err)
			}
			// A subsequent human version prevents the late AI patch from replacing
			// it. The rejected patch remains readable through the Work HTTP route.
			model.beforeWrite = func() {
				body := artifact.Version.Body
				body.Root.Content[0].Content[0].Text = "人工更新的正文"
				_, err := work.Store.SaveArtifact(ctx, user, topic.ID, artifact.Artifact.ID, domain.SaveArtifact{Mutation: mutation(key+"human-late", 1), Title: artifact.Artifact.Title, Body: body})
				if err != nil {
					t.Error(err)
				}
			}
			model.arguments = capability.Value(fmt.Sprintf(`{"summary":"AI 拟补充","changes":[{"kind":"replace","targetId":%q,"node":{"id":%q,"type":"paragraph","content":[{"type":"text","text":"未保存的 AI 修改"}]}}]}`, artifact.Version.Body.Root.Content[0].ID, artifact.Version.Body.Root.Content[0].ID))
			model.calls.Store(0)
			model.databaseFailure = false
			late := domain.ChatRequest{RequestID: key + "late", Question: "修改正文", Action: "edit_document", ArtifactID: artifact.Artifact.ID, ArtifactRevision: 1}
			raw, _ = json.Marshal(late)
			response, err = http.Post(server.URL+"/work/topics/"+topic.ID+"/chat", "application/json", bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			wire, err = io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			match = regexp.MustCompile(`"taskId":"([^"]+)"`).FindSubmatch(wire)
			if len(match) != 2 {
				t.Fatalf("late meta: %s", wire)
			}
			lateID := string(match[1])
			preview, err := http.Get(server.URL + "/work/topics/" + topic.ID + "/turns/" + lateID + "/document-draft")
			if err != nil {
				t.Fatal(err)
			}
			var draftResponse struct {
				Data struct {
					Body         domain.Document
					BaseRevision int
				}
			}
			if err = json.NewDecoder(preview.Body).Decode(&draftResponse); err != nil {
				t.Fatal(err)
			}
			preview.Body.Close()
			if preview.StatusCode != 200 || draftResponse.Data.BaseRevision != 1 || draftResponse.Data.Body.Text() != "未保存的 AI 修改" {
				t.Fatalf("missing rejected patch preview: %+v", draftResponse)
			}
			latest, err := work.Store.GetArtifact(ctx, user, topic.ID, artifact.Artifact.ID)
			if err != nil || latest.Version.Body.Text() != "人工更新的正文" {
				t.Fatal("late AI result replaced current body", err)
			}
		})
	}
}

type failureAfterWriteModel struct {
	calls           atomic.Int32
	databaseFailure bool
	beforeWrite     func()
	arguments       capability.Value
}

func (m *failureAfterWriteModel) Stream(_ context.Context, request conversationruntime.ModelRequest, emit func(conversationruntime.ModelEvent) error) (conversationruntime.Turn, error) {
	call := m.calls.Add(1)
	if call == 1 {
		if m.beforeWrite != nil {
			m.beforeWrite()
		}
		if m.arguments != nil {
			return conversationruntime.Turn{ToolCalls: []conversationruntime.ToolCall{{ID: "document-call", CapabilityID: "work_write_document", Arguments: m.arguments}}}, nil
		}
		return conversationruntime.Turn{ToolCalls: []conversationruntime.ToolCall{{ID: "document-call", CapabilityID: "work_write_document", Arguments: capability.Value(`{"title":"共同文档","summary":"已保存正文","body":{"schemaVersion":1,"root":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"已保存的正文"}]}]}}}`)}}}, nil
	}
	if m.databaseFailure {
		if err := emit(conversationruntime.ModelEvent{Kind: conversationruntime.ModelEventContent, Text: "正文已保存"}); err != nil {
			return conversationruntime.Turn{}, err
		}
		return conversationruntime.Turn{Content: "正文已保存"}, nil
	}
	return conversationruntime.Turn{}, errors.New("injected response stream failure after successful tool")
}
