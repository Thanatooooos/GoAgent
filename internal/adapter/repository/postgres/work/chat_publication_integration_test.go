package work_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	raghttp "local/rag-project/internal/adapter/http/rag"
	workhttp "local/rag-project/internal/adapter/http/work"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	ragservice "local/rag-project/internal/app/rag/service"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
	workdomain "local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/stream"
)

const publicationAnswer = `saved answer <ref id="w1"/>`

type publicationSearch struct{}

func (publicationSearch) Search(context.Context, string) ([]capability.WebSearchResult, error) {
	return []capability.WebSearchResult{{URL: "https://example.com/evidence", Title: "source", Snippet: "evidence"}}, nil
}

type publicationContentProcessor struct{}

func (publicationContentProcessor) ProcessAddMessage(_ context.Context, input ragservice.AddConversationMessageInput) (ragservice.ProcessedConversationMessageContent, error) {
	if input.Role != convention.AssistantRole {
		return ragservice.ProcessedConversationMessageContent{Content: input.Content}, nil
	}
	return ragservice.ProcessedConversationMessageContent{Content: "summary: " + input.Content, RawContent: input.Content, ContentSummary: "summary", IsSummarized: true,
		SessionChunks: []ragservice.ProcessedConversationMessageChunk{{ChunkIndex: 1, Content: "first chunk"}, {ChunkIndex: 2, Content: "second chunk"}}}, nil
}

// Deliberately omit source tags from the summary: publication must preserve
// both the original display body and its sources.
type bodySummaryProcessor struct{}

func (bodySummaryProcessor) ProcessAddMessage(_ context.Context, input ragservice.AddConversationMessageInput) (ragservice.ProcessedConversationMessageContent, error) {
	if input.Role != convention.AssistantRole {
		return ragservice.ProcessedConversationMessageContent{Content: input.Content}, nil
	}
	return ragservice.ProcessedConversationMessageContent{Content: "short context summary", RawContent: input.Content, ContentSummary: "short context summary", IsSummarized: true}, nil
}

func TestChatAndWorkPublishCompleteVisibleBodyAndRecoverIt(t *testing.T) {
	for _, inWork := range []bool{false, true} {
		t.Run(fmt.Sprintf("work=%t", inWork), func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			user, key := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			var work *workbootstrap.Runtime
			var topicID string
			if inWork {
				var err error
				work, err = workbootstrap.NewRuntime(db)
				if err != nil {
					t.Fatal(err)
				}
				topic, err := work.Store.CreateTopic(ctx, user, workdomain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "complete body"})
				if err != nil {
					t.Fatal(err)
				}
				topicID = topic.ID
				turn, err := work.Store.AcceptTurn(ctx, user, topic.ID, workdomain.ChatRequest{RequestID: key + "turn", Question: "question", Action: "discuss"})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := work.Store.StartTurn(ctx, user, topic.ID, turn.ID); err != nil {
					t.Fatal(err)
				}
				f.id = turn.ID
				f.input = convruntime.ChatInput{TaskID: turn.ID, TraceID: turn.ID, ConversationID: turn.ConversationID, UserID: user, AcceptedUserMessageID: turn.UserMessageID, Question: turn.Question, Work: &capability.WorkScope{TopicID: topic.ID, TurnID: turn.ID}, Policy: convruntime.Policy{AllowWebSearch: true}}
				f.chat.SetConversationGuard(func(ctx context.Context, user, conversation string) error {
					return work.Store.ValidateWorkConversation(ctx, user, conversation)
				})
			}
			f.model.prefix = "先查资料。\n\n"
			f.messages.SetContentProcessor(bodySummaryProcessor{})
			// Use the normal message preparer, then fail before publication. A
			// new publisher must recover the full body without another model call.
			f.chat.SetPublication(postgresruntime.NewChatPublisher(db, publicationPreparerFunc(func(context.Context, convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
				return postgresruntime.PreparedPublication{}, errors.New("injected preparation failure")
			})))
			manager := stream.NewMemoryStreamManager()
			result, err := f.chat.Chat(ctx, f.input, raghttp.NewRuntimeEventSink(manager, f.id))
			if !convruntime.IsPublicationPending(err) {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			events, _, err := manager.GetEvents(ctx, f.id, 0)
			if err != nil {
				t.Fatal(err)
			}
			var visible strings.Builder
			for _, event := range events {
				if event.Name != "message" {
					continue
				}
				var delta struct{ Type, Delta string }
				if err := json.Unmarshal(event.Data, &delta); err != nil {
					t.Fatal(err)
				}
				if delta.Type == "response" {
					visible.WriteString(delta.Delta)
				}
			}
			want := visible.String()
			if !strings.HasPrefix(want, f.model.prefix) || !strings.Contains(want, "saved answer") || result.Runtime.AssistantContent != want {
				t.Fatalf("visible=%q result=%q", want, result.Runtime.AssistantContent)
			}
			session := f.session(t)
			publisher := postgresruntime.NewChatPublisher(db, f.adapter)
			message, finish, err := publisher.Publish(ctx, session.ID)
			if err != nil {
				t.Fatal(err)
			}
			var completion struct{ MessageID, Content string }
			if err := json.Unmarshal([]byte(finish.Detail), &completion); err != nil {
				t.Fatal(err)
			}
			persisted, err := postgresrag.NewConversationMessageRepository(db).GetByID(ctx, message.ID)
			if err != nil {
				t.Fatal(err)
			}
			if message.Content != want || persisted.RawContent != want || completion.Content != want || persisted.Content != "short context summary" || len(persisted.Sources) != 1 {
				t.Fatalf("message=%+v stored=%+v completion=%+v", message, persisted, completion)
			}
			again, replayed, err := publisher.Publish(ctx, session.ID)
			if err != nil || again.ID != message.ID || replayed.Detail != finish.Detail || f.model.calls.Load() != 2 {
				t.Fatalf("duplicate/replayed publication: %+v %v", again, err)
			}
			if err := raghttp.NewRuntimeEventSink(manager, f.id).Append(ctx, replayed); err != nil {
				t.Fatal(err)
			}
			events, _, err = manager.GetEvents(ctx, f.id, 0)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(events[len(events)-2].Data, &completion); err != nil || completion.Content != want {
				t.Fatalf("wire finish=%s %v", events[len(events)-2].Data, err)
			}
			if inWork {
				messages, err := work.Store.ListMessages(ctx, user, topicID, f.input.ConversationID, workdomain.Page{Limit: 20})
				if err != nil || len(messages) != 2 || messages[0].Content != want {
					t.Fatalf("Work display=%+v %v", messages, err)
				}
				storedTurn, err := work.Store.GetTurn(ctx, user, topicID, f.id)
				if err != nil || storedTurn.AssistantMessageID != message.ID || storedTurn.Status != "completed" {
					t.Fatalf("Work turn=%+v %v", storedTurn, err)
				}
				original, err := work.Store.GetMessage(ctx, user, topicID, message.ID)
				if err != nil || original.Content != want {
					t.Fatalf("Work original discussion=%+v %v", original, err)
				}
			} else {
				history := runtimeadapter.NewConversationHistory(postgresrag.NewConversationMessageRepository(db), nil)
				modelHistory, err := history.Messages(ctx, convruntime.RunRequest{ConversationID: f.input.ConversationID, UserID: user})
				if err != nil || len(modelHistory) != 2 || modelHistory[1].Content != "short context summary" {
					t.Fatalf("model history=%+v %v", modelHistory, err)
				}
			}
			// Reload through the formal HTTP handlers, not just repository reads.
			router := gin.New()
			router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
			path := "/conversations/" + f.input.ConversationID + "/messages"
			if inWork {
				workhttp.RegisterChatRoutes(router, work)
				path = "/work/topics/" + topicID + "/conversations/" + f.input.ConversationID + "/messages"
			} else {
				h := raghttp.NewHandler(f.conversations, f.messages, nil, nil, nil, manager)
				router.GET("/conversations/:conversationId/messages", h.ListMessages)
			}
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, httptest.NewRequest("GET", path, nil))
			var response struct {
				Data []struct{ ID, Content, RawContent string }
			}
			if recorder.Code != 200 {
				t.Fatalf("reload=%d %s", recorder.Code, recorder.Body.String())
			}
			if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, row := range response.Data {
				if row.ID != message.ID {
					continue
				}
				body := row.Content
				if row.RawContent != "" {
					body = row.RawContent
				}
				if body != want {
					t.Fatalf("reloaded body=%q want=%q", body, want)
				}
				found = true
			}
			if !found {
				t.Fatalf("published message missing from HTTP reload: %s", recorder.Body.String())
			}
		})
	}
}

type publicationEmbedding struct {
	db      *gorm.DB
	session string
	calls   atomic.Int32
}

func (e *publicationEmbedding) Embed(string) ([]float32, error) {
	e.calls.Add(1)
	// A separate transaction must acquire the session while preparing embeddings.
	// Holding a publication lock during this call would hit the lock timeout.
	err := e.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`SET LOCAL lock_timeout='150ms'`).Error; err != nil {
			return err
		}
		var id string
		return tx.Raw(`SELECT id FROM t_runtime_session WHERE id=? FOR UPDATE`, e.session).Scan(&id).Error
	})
	return []float32{1, 0, 0}, err
}
func (e *publicationEmbedding) EmbedWithModel(s, m string) ([]float32, error) { return e.Embed(s) }
func (e *publicationEmbedding) EmbedBatch([]string) ([][]float32, error) {
	return nil, errors.New("unused")
}
func (e *publicationEmbedding) EmbedBatchWithModel([]string, string) ([][]float32, error) {
	return nil, errors.New("unused")
}
func (e *publicationEmbedding) Dimension() int { return 3 }

func TestChatPublicationPreparesContentAndEmbeddingsBeforeTransaction(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, _ := testIdentity(t)
	f := newPublicationFixture(t, db, user, publicationID(t))
	input, err := f.chat.Admit(ctx, f.input)
	if err != nil {
		t.Fatal(err)
	}
	session := f.session(t)
	f.messages.SetContentProcessor(publicationContentProcessor{})
	embedding := &publicationEmbedding{db: db, session: session.ID}
	adapter := runtimeadapter.NewConversationMessages(f.messages, postgresrag.NewConversationMessageChunkSink(db, embedding))
	publisher := postgresruntime.NewChatPublisher(db, adapter)
	f.chat.SetPublication(publisher)
	callback := "publication/chunks-fault/" + f.id
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "t_runtime_journal" {
			field := tx.Statement.Schema.LookUpField("EventType")
			value, _ := field.ValueOf(tx.Statement.Context, tx.Statement.ReflectValue)
			if value == convruntime.EventCompleted {
				tx.AddError(errors.New("finish failed after chunks"))
			}
		}
	})
	_, err = f.chat.Chat(ctx, input, nil)
	db.Callback().Create().Remove(callback)
	if !convruntime.IsPublicationPending(err) {
		t.Fatal(err)
	}
	if f.count(t, "t_session_chunk", "conversation_id=?", f.id) != 0 {
		t.Fatal("chunks escaped failed publication")
	}
	message, _, err := publisher.Publish(ctx, session.ID)
	if err != nil {
		t.Fatal(err)
	}
	if embedding.calls.Load() != 4 || f.count(t, "t_session_chunk", "message_id=?", message.ID) != 2 {
		t.Fatal("content chunks missing or embedding repeated during commit")
	}
	var vectors int64
	if err := db.Raw(`SELECT COUNT(*) FROM t_session_chunk_embedding e JOIN t_session_chunk c ON c.id=e.chunk_id WHERE c.message_id=?`, message.ID).Scan(&vectors).Error; err != nil || vectors != 2 {
		t.Fatalf("chunk embeddings: %d %v", vectors, err)
	}
	persisted, err := postgresrag.NewConversationMessageRepository(db).GetByID(ctx, message.ID)
	if err != nil || !persisted.IsSummarized || persisted.RawContent == "" || persisted.ContentSummary != "summary" || len(persisted.Sources) != 1 {
		t.Fatalf("message processing lost: %+v %v", persisted, err)
	}
	if _, _, err := publisher.Publish(ctx, session.ID); err != nil || embedding.calls.Load() != 4 {
		t.Fatal("published message was prepared again", err)
	}
}

func TestChatReadyAnswerCommitAndPendingCancellation(t *testing.T) {
	db := testDB(t)
	user, _ := testIdentity(t)
	f := newPublicationFixture(t, db, user, publicationID(t))
	ctx := context.Background()
	input, err := f.chat.Admit(ctx, f.input)
	if err != nil {
		t.Fatal(err)
	}
	session := f.session(t)
	callback := "publication/ready-fault/" + f.id
	db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
		if tx.Statement.Table == "t_runtime_chat_publication" {
			tx.AddError(errors.New("ready commit failed"))
		}
	})
	_, err = f.chat.Chat(ctx, input, nil)
	db.Callback().Create().Remove(callback)
	if err == nil || f.session(t).Status != convruntime.StatusFailed || f.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type=?", session.ID, convruntime.EventAnswerFinal) != 0 || f.count(t, "t_runtime_chat_publication", "runtime_session_id=?", session.ID) != 0 || f.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type=?", session.ID, convruntime.EventFailed) != 1 {
		t.Fatal("failed ready transaction leaked an answer or lacked a terminal outcome")
	}
	// This execution has no committed final answer and is not eligible for publication.
	if _, _, err := f.publisher.Publish(ctx, session.ID); err == nil {
		t.Fatal("uncommitted answer was recovered")
	}

	f = newPublicationFixture(t, db, user, publicationID(t))
	f.chat.SetPublication(postgresruntime.NewChatPublisher(db, publicationPreparerFunc(func(context.Context, convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
		return postgresruntime.PreparedPublication{}, errors.New("pending fixture")
	})))
	_, err = f.chat.Chat(ctx, f.input, nil)
	if !convruntime.IsPublicationPending(err) {
		t.Fatal(err)
	}
	session = f.session(t)
	h := raghttp.NewHandler(f.conversations, f.messages, nil, nil, nil, stream.NewMemoryStreamManager())
	h.SetRuntimeChat(f.chat)
	router := gin.New()
	router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
	router.POST("/stop", h.StopChat)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("POST", "/stop?taskId="+f.id, nil))
	if w.Code != 200 {
		t.Fatalf("stop failed: %s", w.Body.String())
	}
	if f.state(t) != "abandoned" || f.count(t, "t_message", "conversation_id=? AND role='assistant'", f.id) != 0 {
		t.Fatal("cancelled publication was saved")
	}
	if _, _, err := f.publisher.Publish(ctx, session.ID); err == nil {
		t.Fatal("cancelled publication revived")
	}
	cache := stream.NewMemoryStreamManager()
	fresh := newPublicationFixture(t, db, user, f.id)
	if _, err := fresh.chat.Replay(ctx, user, f.id, raghttp.NewRuntimeEventSink(cache, f.id)); err != nil {
		t.Fatal(err)
	}
	events, _, _ := cache.GetEvents(ctx, f.id, 0)
	if len(events) == 0 || !events[len(events)-1].Done {
		t.Fatal("cancelled publication has no replayable fate")
	}
}

func TestWorkCancelledPendingAnswerCannotPublish(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, key := testIdentity(t)
	work, err := workbootstrap.NewRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := work.Store.CreateTopic(ctx, user, workdomain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "pending cancellation"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := work.Store.AcceptTurn(ctx, user, topic.ID, workdomain.ChatRequest{RequestID: key + "turn", Question: "question", Action: "discuss"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := work.Store.StartTurn(ctx, user, topic.ID, turn.ID); err != nil {
		t.Fatal(err)
	}
	lifecycle := convruntime.NewLifecycle(postgresruntime.NewStore(db), func() (string, error) { n, err := distributedid.NextID(); return strconv.FormatInt(n, 10), err })
	session, err := lifecycle.StartSession(ctx, convruntime.RunRequest{ConversationID: turn.ConversationID, UserID: user, UserMessageID: turn.UserMessageID, Question: turn.Question, TraceID: turn.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.ReadyAnswer(ctx, session, "ready answer"); err != nil {
		t.Fatal(err)
	}
	f := newPublicationFixture(t, db, user, turn.ID)
	if err := work.Store.CancelTurn(ctx, user, topic.ID, turn.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := f.publisher.Publish(ctx, session.ID); err == nil {
		t.Fatal("Work cancellation ignored")
	}
	if f.state(t) != "abandoned" || f.count(t, "t_message", "conversation_id=? AND role='assistant'", turn.ConversationID) != 0 {
		t.Fatal("cancelled Work answer persisted")
	}
	if got, err := work.Store.GetTurn(ctx, user, topic.ID, turn.ID); err != nil || got.Status != "cancelled" {
		t.Fatalf("turn overwritten: %+v %v", got, err)
	}
}

type publicationModel struct {
	calls  atomic.Int32
	prefix string
}

func (m *publicationModel) Stream(ctx context.Context, _ convruntime.ModelRequest, emit func(convruntime.ModelEvent) error) (convruntime.Turn, error) {
	if m.calls.Add(1) == 1 {
		if m.prefix != "" {
			if err := emit(convruntime.ModelEvent{Kind: convruntime.ModelEventContent, Text: m.prefix}); err != nil {
				return convruntime.Turn{}, err
			}
		}
		return convruntime.Turn{Content: m.prefix, ToolCalls: []convruntime.ToolCall{{ID: "source", CapabilityID: capability.WebSearchID, Arguments: capability.Value(`{"query":"evidence"}`)}}}, nil
	}
	if err := emit(convruntime.ModelEvent{Kind: convruntime.ModelEventContent, Text: publicationAnswer}); err != nil {
		return convruntime.Turn{}, err
	}
	return convruntime.Turn{Content: publicationAnswer}, nil
}

type publicationPreparerFunc func(context.Context, convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error)

func (f publicationPreparerFunc) PreparePublication(ctx context.Context, m convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
	return f(ctx, m)
}

type publicationSinkFunc func(context.Context, convruntime.JournalEntry) error

func (f publicationSinkFunc) Append(ctx context.Context, e convruntime.JournalEntry) error {
	return f(ctx, e)
}

type publicationFixture struct {
	db            *gorm.DB
	chat          *convruntime.ChatService
	publisher     *postgresruntime.ChatPublisher
	conversations *ragservice.ConversationService
	messages      *ragservice.ConversationMessageService
	adapter       runtimeadapter.ConversationMessages
	model         *publicationModel
	kernel        *convruntime.Runtime
	user, id      string
	input         convruntime.ChatInput
}

func publicationID(t *testing.T) string {
	t.Helper()
	id, err := distributedid.NextID()
	if err != nil {
		t.Fatal(err)
	}
	return strconv.FormatInt(id, 10)
}
func newPublicationFixture(t *testing.T, db *gorm.DB, user, id string) *publicationFixture {
	t.Helper()
	convRepo := postgresrag.NewConversationRepository(db)
	msgRepo := postgresrag.NewConversationMessageRepository(db)
	summary := postgresrag.NewConversationSummaryRepository(db)
	conv := ragservice.NewConversationService(convRepo, msgRepo, summary, nil, nil, 30, postgresrag.NewConversationDeleteTransaction(db))
	messages := ragservice.NewConversationMessageService(convRepo, msgRepo, summary, nil)
	adapter := runtimeadapter.NewConversationMessages(messages)
	store := postgresruntime.NewStore(db)
	model := &publicationModel{}
	tools := capability.NewRegistry()
	if err := tools.Register(capability.WebSearch(publicationSearch{})); err != nil {
		t.Fatal(err)
	}
	kernel := &convruntime.Runtime{Model: model, Tools: tools, Lifecycle: convruntime.NewLifecycle(store, func() (string, error) { n, err := distributedid.NextID(); return strconv.FormatInt(n, 10), err }), Episodes: store}
	chat := convruntime.NewChatService(runtimeadapter.NewConversations(conv), adapter, kernel)
	chat.SetConversationGuard(runtimeadapter.OrdinaryConversationGuard(db))
	chat.SetTaskAccessResolver(store.CanAccessChatTask)
	publisher := postgresruntime.NewChatPublisher(db, adapter)
	chat.SetPublication(publisher)
	return &publicationFixture{db: db, chat: chat, publisher: publisher, conversations: conv, messages: messages, adapter: adapter, model: model, user: user, id: id,
		kernel: kernel, input: convruntime.ChatInput{TaskID: id, TraceID: id, ConversationID: id, UserID: user, Question: "question", Policy: convruntime.Policy{AllowWebSearch: true}}}
}
func (f *publicationFixture) session(t *testing.T) persistence.Session {
	t.Helper()
	s, err := postgresruntime.NewStore(f.db).FindSessionByTraceID(context.Background(), f.user, f.id)
	if err != nil || s.ID == "" {
		t.Fatalf("session: %+v %v", s, err)
	}
	return s
}
func (f *publicationFixture) state(t *testing.T) string {
	t.Helper()
	var s string
	if err := f.db.Raw(`SELECT state FROM t_runtime_chat_publication WHERE runtime_session_id=?`, f.session(t).ID).Scan(&s).Error; err != nil {
		t.Fatal(err)
	}
	return s
}
func (f *publicationFixture) count(t *testing.T, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := f.db.Table(table).Where(where, args...).Count(&n).Error; err != nil {
		t.Fatal(err)
	}
	return n
}
func (f *publicationFixture) assertPublished(t *testing.T, session string) string {
	t.Helper()
	if f.state(t) != "published" {
		t.Fatal("publication not committed")
	}
	if n := f.count(t, "t_message", "conversation_id=? AND role='assistant' AND deleted=0", f.id); n != 1 {
		t.Fatalf("assistant count %d", n)
	}
	var id string
	f.db.Raw(`SELECT assistant_message_id FROM t_runtime_chat_publication WHERE runtime_session_id=?`, session).Scan(&id)
	if n := f.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type=?", session, convruntime.EventCompleted); n != 1 {
		t.Fatalf("finish count %d", n)
	}
	return id
}

func TestChatPublicationFailureWindowsAndConcurrentRecovery(t *testing.T) {
	for _, stage := range []string{"prepare", "message", "finish", "transport"} {
		t.Run(stage, func(t *testing.T) {
			db := testDB(t)
			user, _ := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			ctx := context.Background()
			input, err := f.chat.Admit(ctx, f.input)
			if err != nil {
				t.Fatal(err)
			}
			session := f.session(t)
			episodeID := publicationID(t)
			if err := postgresruntime.NewStore(db).CreateEpisode(ctx, persistence.Episode{ID: episodeID, RuntimeSessionID: session.ID, ConversationID: f.id, UserID: user, SourceUserMessageID: session.UserMessageID, Summary: "pending fact", Importance: "normal", Status: persistence.EpisodePending, CreatedAt: time.Now(), UpdatedAt: time.Now()}, []float32{1, 0, 0}); err != nil {
				t.Fatal(err)
			}
			callback := "publication/fault/" + f.id
			if stage == "prepare" {
				f.chat.SetPublication(postgresruntime.NewChatPublisher(db, publicationPreparerFunc(func(context.Context, convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
					return postgresruntime.PreparedPublication{}, errors.New("prepare failed")
				})))
			}
			if stage == "message" || stage == "finish" {
				db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
					if (stage == "message" && tx.Statement.Table == "t_message") || (stage == "finish" && tx.Statement.Table == "t_runtime_journal") {
						field := tx.Statement.Schema.LookUpField("Role")
						want := "assistant"
						if stage == "finish" {
							field = tx.Statement.Schema.LookUpField("EventType")
							want = convruntime.EventCompleted
						}
						if field != nil {
							value, _ := field.ValueOf(tx.Statement.Context, tx.Statement.ReflectValue)
							if value == want {
								tx.AddError(errors.New("injected " + stage + " failure"))
							}
						}
					}
				})
				t.Cleanup(func() { db.Callback().Create().Remove(callback) })
			}
			cache := stream.NewMemoryStreamManager()
			raghttp.SendRuntimeMeta(cache, f.id, f.id)
			sink := raghttp.NewRuntimeEventSink(cache, f.id)
			if stage == "transport" {
				sink = publicationSinkFunc(func(ctx context.Context, e convruntime.JournalEntry) error {
					if e.EventType == convruntime.EventCompleted {
						return errors.New("connection failed")
					}
					return raghttp.NewRuntimeEventSink(cache, f.id).Append(ctx, e)
				})
			}
			_, err = f.chat.Chat(ctx, input, sink)
			if !convruntime.IsPublicationPending(err) {
				t.Fatalf("missing pending result: %v", err)
			}
			if stage != "transport" {
				if f.state(t) != "pending" || f.count(t, "t_message", "conversation_id=? AND role='assistant'", f.id) != 0 {
					t.Fatal("failed publication leaked a message")
				}
				if f.count(t, "t_runtime_episode", "id=? AND status='pending'", episodeID) != 1 {
					t.Fatal("episode escaped rollback")
				}
			}
			db.Callback().Create().Remove(callback)
			// Recreate all service objects, as after restart, and race independent publishers.
			fresh := newPublicationFixture(t, db, user, f.id)
			var wg sync.WaitGroup
			errs := make(chan error, 8)
			ids := make(chan string, 8)
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					m, _, err := fresh.publisher.Publish(ctx, session.ID)
					errs <- err
					ids <- m.ID
				}()
			}
			wg.Wait()
			close(errs)
			close(ids)
			for err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			messageID := f.assertPublished(t, session.ID)
			for id := range ids {
				if id != messageID {
					t.Fatalf("unstable assistant id %s != %s", id, messageID)
				}
			}
			if f.count(t, "t_runtime_episode", "id=? AND status='ready' AND source_assistant_message_id=?", episodeID, messageID) != 1 {
				t.Fatal("episode not linked atomically")
			}
			// A prior transport error has already closed the hot cache. Reconnect must
			// pass that old done and recover finish/sources at the formal HTTP endpoint.
			raghttp.SendRuntimeFailure(cache, f.id, errors.New("old transport error"))
			h := raghttp.NewHandler(f.conversations, f.messages, nil, nil, nil, cache)
			h.SetRuntimeChat(fresh.chat)
			h.SetRuntimeReplay(fresh.chat)
			router := gin.New()
			router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
			router.GET("/continue", h.ContinueChat)
			for _, hot := range []bool{true, false} {
				if !hot {
					cache = stream.NewMemoryStreamManager()
					h = raghttp.NewHandler(f.conversations, f.messages, nil, nil, nil, cache)
					h.SetRuntimeChat(fresh.chat)
					h.SetRuntimeReplay(fresh.chat)
					router = gin.New()
					router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
					router.GET("/continue", h.ContinueChat)
				}
				w := httptest.NewRecorder()
				router.ServeHTTP(w, httptest.NewRequest("GET", "/continue?taskId="+f.id, nil))
				wire := w.Body.String()
				if !strings.Contains(wire, `"messageId":"`+messageID+`"`) || !strings.Contains(wire, "https://example.com/evidence") || strings.Index(wire, "event: done") < strings.Index(wire, "event: finish") {
					t.Fatalf("hot=%t incorrect replay: %s", hot, wire)
				}
			}
			if f.model.calls.Load() != 2 || fresh.model.calls.Load() != 0 {
				t.Fatal("recovery invoked answer model")
			}
		})
	}
}

func TestChatPublicationDeletedInputCannotBeResurrected(t *testing.T) {
	for _, deleteConversation := range []bool{true, false} {
		t.Run(fmt.Sprint(deleteConversation), func(t *testing.T) {
			db := testDB(t)
			user, _ := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			ctx := context.Background()
			f.chat.SetPublication(postgresruntime.NewChatPublisher(db, publicationPreparerFunc(func(context.Context, convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
				return postgresruntime.PreparedPublication{}, errors.New("fault")
			})))
			_, err := f.chat.Chat(ctx, f.input, nil)
			if !convruntime.IsPublicationPending(err) {
				t.Fatal(err)
			}
			session := f.session(t)
			if deleteConversation {
				if err := f.conversations.Delete(ctx, ragservice.DeleteConversationInput{ConversationID: f.id, UserID: user}); err != nil {
					t.Fatal(err)
				}
			} else {
				db.Exec(`UPDATE t_message SET deleted=1 WHERE id=?`, session.UserMessageID)
			}
			if _, _, err := f.publisher.Publish(ctx, session.ID); err == nil {
				t.Fatal("deleted identity was published")
			}
			if f.state(t) != "abandoned" || f.count(t, "t_message", "conversation_id=? AND role='assistant'", f.id) != 0 {
				t.Fatal("deleted input resurrected")
			}
		})
	}
}

// The subprocess exits abruptly inside publication; PostgreSQL must release its
// transaction and a new process must recover without a second model turn.
func TestChatPublicationCrashHelper(t *testing.T) {
	stage := os.Getenv("CODEX_PUBLICATION_CRASH_STAGE")
	if stage == "" {
		t.Skip("subprocess helper")
	}
	db := testDB(t)
	f := newPublicationFixture(t, db, os.Getenv("CODEX_PUBLICATION_USER"), os.Getenv("CODEX_PUBLICATION_ID"))
	if stage == "ready" {
		f.chat.SetPublication(postgresruntime.NewChatPublisher(db, publicationPreparerFunc(func(context.Context, convruntime.ConversationMessage) (postgresruntime.PreparedPublication, error) {
			os.Exit(73)
			return postgresruntime.PreparedPublication{}, nil
		})))
	} else {
		db.Callback().Create().Before("gorm:create").Register("publication/process-exit", func(tx *gorm.DB) {
			if tx.Statement.Table == "t_runtime_journal" {
				field := tx.Statement.Schema.LookUpField("EventType")
				value, _ := field.ValueOf(tx.Statement.Context, tx.Statement.ReflectValue)
				if value == convruntime.EventCompleted {
					os.Exit(73)
				}
			}
		})
	}
	f.chat.Chat(context.Background(), f.input, nil)
	t.Fatal("process did not exit")
}
func TestChatPublicationSurvivesProcessExit(t *testing.T) {
	for _, stage := range []string{"ready", "message"} {
		t.Run(stage, func(t *testing.T) {
			db := testDB(t)
			user, _ := testIdentity(t)
			id := publicationID(t)
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestChatPublicationCrashHelper$")
			cmd.Env = append(os.Environ(), "CODEX_PUBLICATION_CRASH_STAGE="+stage, "CODEX_PUBLICATION_USER="+user, "CODEX_PUBLICATION_ID="+id)
			out, err := cmd.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 73 {
				t.Fatalf("child: %s %v", out, err)
			}
			fresh := newPublicationFixture(t, db, user, id)
			session := fresh.session(t)
			if fresh.state(t) != "pending" || fresh.count(t, "t_message", "conversation_id=? AND role='assistant'", id) != 0 {
				t.Fatal("crash left partial publication")
			}
			cache := stream.NewMemoryStreamManager()
			_, err = fresh.chat.Replay(context.Background(), user, id, raghttp.NewRuntimeEventSink(cache, id))
			if err != nil {
				t.Fatal(err)
			}
			fresh.assertPublished(t, session.ID)
			if fresh.model.calls.Load() != 0 || fresh.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type=?", session.ID, convruntime.EventModelTurnStarted) != 2 {
				t.Fatal("restart executed the model again")
			}
		})
	}
}
