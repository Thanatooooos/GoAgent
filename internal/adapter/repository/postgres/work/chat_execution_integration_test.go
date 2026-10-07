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
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	raghttp "local/rag-project/internal/adapter/http/rag"
	workhttp "local/rag-project/internal/adapter/http/work"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
	workdomain "local/rag-project/internal/app/work/domain"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/framework/contextx"
	"local/rag-project/internal/framework/stream"
)

type executionModelFunc func(context.Context, convruntime.ModelRequest, func(convruntime.ModelEvent) error) (convruntime.Turn, error)

func (f executionModelFunc) Stream(ctx context.Context, req convruntime.ModelRequest, emit func(convruntime.ModelEvent) error) (convruntime.Turn, error) {
	return f(ctx, req, emit)
}

func expireExecution(t *testing.T, db *gorm.DB, task string) {
	t.Helper()
	if err := db.Exec(`UPDATE t_runtime_chat_execution SET lease_until=clock_timestamp()-INTERVAL '1 second' WHERE task_id=?`, task).Error; err != nil {
		t.Fatal(err)
	}
}

func recoverExecutionConcurrently(t *testing.T, db *gorm.DB, user, task string) convruntime.JournalEntry {
	t.Helper()
	var wg sync.WaitGroup
	results := make(chan convruntime.JournalEntry, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			event, err := postgresruntime.NewStore(db).RecoverChatExecution(context.Background(), user, task)
			results <- event
			errs <- err
		}()
	}
	wg.Wait()
	close(results)
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var terminal convruntime.JournalEntry
	for event := range results {
		if event.ID == "" || (terminal.ID != "" && event.ID != terminal.ID) {
			t.Fatalf("unstable terminal event: %+v != %+v", event, terminal)
		}
		terminal = event
	}
	return terminal
}

func TestChatExecutionClaimRecoveryAndFencing(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, _ := testIdentity(t)
	f := newPublicationFixture(t, db, user, publicationID(t))
	if _, err := f.chat.Admit(ctx, f.input); err != nil {
		t.Fatal(err)
	}
	session := f.session(t)
	store := postgresruntime.NewStore(db)
	var wg sync.WaitGroup
	claims := make(chan persistence.Execution, 8)
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			claim, err := store.ClaimChatExecution(ctx, session, fmt.Sprintf("worker-%d", i), time.Minute)
			claims <- claim
			errs <- err
		}(i)
	}
	wg.Wait()
	close(claims)
	close(errs)
	var claim persistence.Execution
	winners := 0
	for c := range claims {
		if c.Owner != "" {
			claim = c
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners: %d", winners)
	}
	for err := range errs {
		if err != nil && !errors.Is(err, persistence.ErrExecutionLeaseLost) {
			t.Fatal(err)
		}
	}
	wrong := claim
	wrong.Epoch++
	if err := store.RenewChatExecution(ctx, wrong, time.Minute); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
		t.Fatal("wrong owner renewed", err)
	}
	if err := store.RenewChatExecution(ctx, claim, time.Minute); err != nil {
		t.Fatal(err)
	}
	owned := persistence.WithExecution(ctx, claim)
	lifecycle := convruntime.NewLifecycle(store, func() (string, error) { return publicationID(t), nil })
	if _, err := lifecycle.BeginTool(owned, session, "unfinished", "web_search", "{}"); err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.StartTool(owned, session, "unfinished", "web_search"); err != nil {
		t.Fatal(err)
	}
	episode := persistence.Episode{ID: publicationID(t), RuntimeSessionID: session.ID, ConversationID: f.id, UserID: user, SourceUserMessageID: session.UserMessageID, Summary: "unpublished fact", Importance: "normal", Status: persistence.EpisodePending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	if err := store.CreateEpisode(owned, episode, []float32{1, 0, 0}); err != nil {
		t.Fatal(err)
	}
	expireExecution(t, db, f.id)
	if err := store.RenewChatExecution(ctx, claim, time.Minute); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
		t.Fatal("expired lease revived", err)
	}
	if event, err := store.RecoverChatExecution(ctx, "other-user", f.id); err != nil || event.ID != "" {
		t.Fatal("foreign user recovered execution", err)
	}
	event := recoverExecutionConcurrently(t, db, user, f.id)
	if event.EventType != convruntime.EventInterrupted || f.session(t).Status != convruntime.StatusInterrupted || f.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type='interrupted'", session.ID) != 1 || f.count(t, "t_runtime_episode", "id=? AND status='discarded'", episode.ID) != 1 {
		t.Fatal("interruption did not settle atomically")
	}
	if calls, err := store.ListUnsettledToolCalls(ctx, session.ID); err != nil || len(calls) != 0 {
		t.Fatal("tool left executing", err)
	}
	if _, err := lifecycle.SettleTool(owned, session, "unfinished", "web_search", convruntime.ToolStateCompleted, "late", nil); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
		t.Fatal("late tool settlement accepted", err)
	}
	if err := lifecycle.ReadyAnswer(owned, session, "late answer"); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
		t.Fatal("late answer accepted", err)
	}
	if _, err := store.FinishChatExecution(owned, session, convruntime.StatusFailed, "late failure"); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
		t.Fatal("late failure replaced interruption", err)
	}
	assertExecutionHTTPReplay(t, f, event)
}

func assertExecutionHTTPReplay(t *testing.T, f *publicationFixture, terminal convruntime.JournalEntry) {
	t.Helper()
	for _, hot := range []bool{true, false} {
		cache := stream.NewMemoryStreamManager()
		if hot {
			raghttp.SendRuntimeMeta(cache, f.id, f.id)
			raghttp.SendRuntimeFailure(cache, f.id, errors.New("old transport failure"))
		}
		fresh := newPublicationFixture(t, f.db, f.user, f.id)
		h := raghttp.NewHandler(f.conversations, f.messages, nil, nil, nil, cache)
		h.SetRuntimeChat(fresh.chat)
		h.SetRuntimeReplay(fresh.chat)
		router := gin.New()
		router.Use(func(c *gin.Context) {
			user := f.user
			if c.GetHeader("X-Test-User") != "" {
				user = c.GetHeader("X-Test-User")
			}
			contextx.Set(c, &contextx.LoginUser{UserID: user})
			c.Next()
		})
		router.GET("/continue", h.ContinueChat)
		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/continue?taskId="+f.id+"&offset=999999", nil))
			wire := w.Body.String()
			if !strings.Contains(wire, `"runtimeEventId":"`+terminal.ID+`"`) || !strings.Contains(wire, `"status":"interrupted"`) || strings.Count(wire, "event: error") != 1 || strings.Count(wire, "event: done") != 1 {
				t.Fatalf("hot=%t replay=%d: %s", hot, i, wire)
			}
		}
		denied := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/continue?taskId="+f.id, nil)
		req.Header.Set("X-Test-User", "other-user")
		router.ServeHTTP(denied, req)
		if strings.Contains(denied.Body.String(), terminal.ID) || fresh.model.calls.Load() != 0 {
			t.Fatal("replay leaked terminal event or ran model")
		}
	}
}

func TestChatReadyAnswerRollbackBeforeTerminalRecovery(t *testing.T) {
	for _, expiry := range []bool{false, true} {
		t.Run(fmt.Sprint(expiry), func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			user, _ := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			if _, err := f.chat.Admit(ctx, f.input); err != nil {
				t.Fatal(err)
			}
			session := f.session(t)
			store := postgresruntime.NewStore(db)
			ttl := time.Minute
			if expiry {
				ttl = 500 * time.Millisecond
			}
			claim, err := store.ClaimChatExecution(ctx, session, "commit-worker", ttl)
			if err != nil {
				t.Fatal(err)
			}
			owned := persistence.WithExecution(ctx, claim)
			callback := "execution/ready-fault/" + f.id
			if err := db.Callback().Create().Before("gorm:create").Register(callback, func(tx *gorm.DB) {
				if tx.Statement.Table != "t_runtime_chat_publication" {
					return
				}
				if expiry {
					tx.Exec(`SELECT pg_sleep(0.6)`)
				} else {
					tx.AddError(errors.New("injected ready failure"))
				}
			}); err != nil {
				t.Fatal(err)
			}
			lifecycle := convruntime.NewLifecycle(store, func() (string, error) { return publicationID(t), nil })
			err = lifecycle.ReadyAnswer(owned, session, "uncommitted answer")
			db.Callback().Create().Remove(callback)
			if err == nil || f.session(t).Status != persistence.StatusRunning || f.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type=?", session.ID, convruntime.EventAnswerFinal) != 0 || f.count(t, "t_runtime_chat_publication", "runtime_session_id=?", session.ID) != 0 {
				t.Fatal("ready transaction did not roll back", err)
			}
			// A later real expiry closes the rolled-back execution, never publishes it.
			expireExecution(t, db, f.id)
			recoverExecutionConcurrently(t, db, user, f.id)
			if _, _, err := f.publisher.Publish(ctx, session.ID); err == nil {
				t.Fatal("uncommitted answer published")
			}
		})
	}
}

func TestChatHeartbeatAndLateModelCannotReviveExecution(t *testing.T) {
	for _, lateFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(lateFailure), func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			user, _ := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			input, err := f.chat.Admit(ctx, f.input)
			if err != nil {
				t.Fatal(err)
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			var calls atomic.Int32
			model := executionModelFunc(func(context.Context, convruntime.ModelRequest, func(convruntime.ModelEvent) error) (convruntime.Turn, error) {
				calls.Add(1)
				close(entered)
				<-release // Deliberately ignore cancellation, as a late provider can.
				if lateFailure {
					return convruntime.Turn{}, errors.New("late provider failure")
				}
				return convruntime.Turn{Content: "late provider answer"}, nil
			})
			f.kernel.Model = model
			f.kernel.ExecutionLeaseDuration = 900 * time.Millisecond
			finished := make(chan error, 1)
			go func() { _, err := f.chat.Chat(ctx, input, nil); finished <- err }()
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("model did not start")
			}
			started := time.Now()
			for time.Since(started) < 3*f.kernel.ExecutionLeaseDuration {
				if event, err := postgresruntime.NewStore(db).RecoverChatExecution(ctx, user, f.id); err != nil || event.ID != "" {
					t.Fatal("healthy heartbeat interrupted", err)
				}
				time.Sleep(50 * time.Millisecond)
			}
			// An independently constructed service cannot start another model.
			fresh := newPublicationFixture(t, db, user, f.id)
			fresh.kernel.Model = model
			if _, err := fresh.chat.Chat(ctx, input, nil); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
				t.Fatal("duplicate execution started", err)
			}
			expireExecution(t, db, f.id)
			event := recoverExecutionConcurrently(t, db, user, f.id)
			once.Do(func() { close(release) })
			select {
			case err := <-finished:
				if err == nil {
					t.Fatal("late result succeeded")
				}
			case <-time.After(3 * time.Second):
				t.Fatal("late model did not exit")
			}
			if calls.Load() != 1 || f.session(t).Status != convruntime.StatusInterrupted || f.count(t, "t_runtime_chat_publication", "runtime_session_id=?", f.session(t).ID) != 0 || f.count(t, "t_message", "conversation_id=? AND role='assistant'", f.id) != 0 {
				t.Fatal("late result revived execution")
			}
			assertExecutionHTTPReplay(t, f, event)
		})
	}
}

func TestWorkAcceptedRecoveryAndSavedWriteFencing(t *testing.T) {
	for _, acceptedOnly := range []bool{true, false} {
		t.Run(fmt.Sprint(acceptedOnly), func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			user, key := testIdentity(t)
			work, err := workbootstrap.NewRuntime(db)
			if err != nil {
				t.Fatal(err)
			}
			topic, err := work.Store.CreateTopic(ctx, user, workdomain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "recover saved work"})
			if err != nil {
				t.Fatal(err)
			}
			turn, err := work.Store.AcceptTurn(ctx, user, topic.ID, workdomain.ChatRequest{RequestID: key + "turn", Question: "create document", Action: "create_document"})
			if err != nil {
				t.Fatal(err)
			}
			store := postgresruntime.NewStore(db)
			var owned context.Context
			change := workdomain.AIDocumentChange{Title: "saved", Body: documentPointer(workdomain.EmptyDocument()), Summary: "saved before crash"}
			var saved workdomain.ArtifactDetail
			if !acceptedOnly {
				if ok, err := work.Store.StartTurn(ctx, user, topic.ID, turn.ID); err != nil || !ok {
					t.Fatal("turn not started", err)
				}
				lifecycle := convruntime.NewLifecycle(store, func() (string, error) { return publicationID(t), nil })
				session, err := lifecycle.StartSession(ctx, convruntime.RunRequest{TraceID: turn.ID, ConversationID: turn.ConversationID, UserMessageID: turn.UserMessageID, UserID: user, Question: turn.Question})
				if err != nil {
					t.Fatal(err)
				}
				claim, err := store.ClaimChatExecution(ctx, session, "work-worker", time.Minute)
				if err != nil {
					t.Fatal(err)
				}
				owned = persistence.WithExecution(ctx, claim)
				saved, err = work.Store.WriteDocument(capability.Context{Context: owned, UserID: user, ToolCallID: "write", Work: &capability.WorkScope{TopicID: topic.ID, TurnID: turn.ID}}, change)
				if err != nil {
					t.Fatal(err)
				}
			}
			expireExecution(t, db, turn.ID)
			event := recoverExecutionConcurrently(t, db, user, turn.ID)
			result, err := work.Store.GetTurn(ctx, user, topic.ID, turn.ID)
			if err != nil || result.Status != "interrupted" || event.EventType != convruntime.EventInterrupted {
				t.Fatal("work did not interrupt", err)
			}
			if !acceptedOnly {
				tool := capability.Context{Context: owned, UserID: user, ToolCallID: "write", Work: &capability.WorkScope{TopicID: topic.ID, TurnID: turn.ID}}
				if replay, err := work.Store.WriteDocument(tool, change); err != nil || replay.Artifact.ID != saved.Artifact.ID {
					t.Fatal("saved result unavailable", err)
				}
				if _, err := work.Store.SuggestProgress(tool, []workdomain.StateChange{{Kind: "add", Entry: workdomain.StateEntry{ID: "late", Kind: "next", Text: "late"}}}); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
					t.Fatal("old worker wrote progress", err)
				}
				if len(result.Outputs) != 1 || result.Outputs[0].ArtifactID != saved.Artifact.ID {
					t.Fatal("saved output disappeared")
				}
			}
			// Cold formal Work replay needs no model and includes the saved outputs.
			fixture := newPublicationFixture(t, db, user, publicationID(t))
			if err := work.ConfigureChat(fixture.kernel, nil, fixture.adapter, stream.NewMemoryStreamManager()); err != nil {
				t.Fatal(err)
			}
			router := gin.New()
			router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
			workhttp.RegisterChatRoutes(router, work)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest("GET", "/work/topics/"+topic.ID+"/turns/"+turn.ID+"/stream?offset=999999", nil))
			if !strings.Contains(w.Body.String(), `"status":"interrupted"`) || !strings.Contains(w.Body.String(), "event: done") || fixture.model.calls.Load() != 0 {
				t.Fatalf("work replay: %s", w.Body.String())
			}
			if _, err := work.Store.AcceptTurn(ctx, user, topic.ID, workdomain.ChatRequest{RequestID: key + "next", ConversationID: turn.ConversationID, Question: "continue"}); err != nil {
				t.Fatal("interrupted turn blocked next request", err)
			}
		})
	}
}

func documentPointer(d workdomain.Document) *workdomain.Document { return &d }

func TestChatExecutionCancellationDeletionAndReadyBoundary(t *testing.T) {
	for _, reason := range []string{"cancel", "delete", "ready"} {
		t.Run(reason, func(t *testing.T) {
			db := testDB(t)
			ctx := context.Background()
			user, _ := testIdentity(t)
			f := newPublicationFixture(t, db, user, publicationID(t))
			if _, err := f.chat.Admit(ctx, f.input); err != nil {
				t.Fatal(err)
			}
			session := f.session(t)
			store := postgresruntime.NewStore(db)
			claim, err := store.ClaimChatExecution(ctx, session, "boundary-worker", time.Minute)
			if err != nil {
				t.Fatal(err)
			}
			owned := persistence.WithExecution(ctx, claim)
			lifecycle := convruntime.NewLifecycle(store, func() (string, error) { return publicationID(t), nil })
			switch reason {
			case "cancel":
				if err := f.chat.CancelPublication(ctx, user, f.id); err != nil {
					t.Fatal(err)
				}
			case "delete":
				if err := db.Exec(`UPDATE t_message SET deleted=1 WHERE id=?`, session.UserMessageID).Error; err != nil {
					t.Fatal(err)
				}
			case "ready":
				if err := lifecycle.ReadyAnswer(owned, session, "durable ready answer"); err != nil {
					t.Fatal(err)
				}
				expireExecution(t, db, f.id)
			}
			if reason != "ready" {
				event := recoverExecutionConcurrently(t, db, user, f.id)
				if event.EventType != convruntime.EventCancelled || f.session(t).Status != convruntime.StatusCancelled {
					t.Fatal("revocation became interruption")
				}
				if err := lifecycle.ReadyAnswer(owned, session, "revoked answer"); !errors.Is(err, persistence.ErrExecutionLeaseLost) {
					t.Fatal("revoked worker published", err)
				}
			} else {
				if err := store.RecoverChatExecutions(ctx, 100, nil); err != nil {
					t.Fatal(err)
				}
				if event, err := store.RecoverChatExecution(ctx, user, f.id); err != nil || event.ID != "" {
					t.Fatal("ready answer interrupted", err)
				}
				if _, _, err := f.publisher.Publish(ctx, session.ID); err != nil {
					t.Fatal("ready answer not recoverable", err)
				}
				f.assertPublished(t, session.ID)
			}
		})
	}
}

func TestChatExecutionPanicLeavesDurableFailure(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, _ := testIdentity(t)
	f := newPublicationFixture(t, db, user, publicationID(t))
	input, err := f.chat.Admit(ctx, f.input)
	if err != nil {
		t.Fatal(err)
	}
	f.kernel.Model = executionModelFunc(func(context.Context, convruntime.ModelRequest, func(convruntime.ModelEvent) error) (convruntime.Turn, error) {
		panic("provider panic fixture")
	})
	result, err := f.chat.Chat(ctx, input, nil)
	if err == nil || result.Runtime.Status != convruntime.StatusFailed || f.session(t).Status != convruntime.StatusFailed || f.count(t, "t_runtime_journal", "runtime_session_id=? AND event_type=?", f.session(t).ID, convruntime.EventFailed) != 1 || f.count(t, "t_runtime_chat_execution", "task_id=? AND state='failed'", f.id) != 1 {
		t.Fatalf("panic left execution open: %+v %v", result.Runtime, err)
	}
}

func TestWorkDeadlineRecoveryPreventsExpiredStart(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, key := testIdentity(t)
	work, err := workbootstrap.NewRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := work.Store.CreateTopic(ctx, user, workdomain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "deadline recovery"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := work.Store.AcceptTurn(ctx, user, topic.ID, workdomain.ChatRequest{RequestID: key + "turn", Question: "accepted then expired"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE t_work_turn SET deadline_at=clock_timestamp()-INTERVAL '1 second' WHERE id=?`, turn.ID).Error; err != nil {
		t.Fatal(err)
	}
	if ok, err := work.Store.StartTurn(ctx, user, topic.ID, turn.ID); err != nil || ok {
		t.Fatal("expired acceptance started", err)
	}
	result, err := work.Store.GetTurn(ctx, user, topic.ID, turn.ID)
	if err != nil || result.Status != "interrupted" {
		t.Fatalf("deadline outcome: %+v %v", result, err)
	}
	// Failures before the kernel starts retain their failure outcome during scan.
	failed, err := work.Store.AcceptTurn(ctx, user, topic.ID, workdomain.ChatRequest{RequestID: key + "failed", Question: "pre-model failure"})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := work.Store.StartTurn(ctx, user, topic.ID, failed.ID); err != nil || !ok {
		t.Fatal("failure fixture not started", err)
	}
	if err := work.Store.FinishTurn(ctx, user, topic.ID, failed.ID, "failed", ""); err != nil {
		t.Fatal(err)
	}
	terminal, err := postgresruntime.NewStore(db).RecoverChatExecution(ctx, user, failed.ID)
	if err != nil || terminal.EventType != convruntime.EventFailed {
		t.Fatalf("pre-model failure became cancellation: %+v %v", terminal, err)
	}
}

func TestWorkHTTPExpiredLateModelPreservesSavedDocument(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, key := testIdentity(t)
	work, err := workbootstrap.NewRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	topic, err := work.Store.CreateTopic(ctx, user, workdomain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "late work provider"})
	if err != nil {
		t.Fatal(err)
	}
	f := newPublicationFixture(t, db, user, publicationID(t))
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	f.kernel.Model = executionModelFunc(func(context.Context, convruntime.ModelRequest, func(convruntime.ModelEvent) error) (convruntime.Turn, error) {
		if calls.Add(1) == 1 {
			return convruntime.Turn{ToolCalls: []convruntime.ToolCall{{ID: "saved-document", CapabilityID: "work_write_document", Arguments: capability.Value(`{"title":"已保存","body":{"schemaVersion":1,"root":{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"已保存正文"}]}]}},"summary":"保存正文"}`)}}}, nil
		}
		close(entered)
		<-release
		return convruntime.Turn{Content: "late answer"}, nil
	})
	cache := stream.NewMemoryStreamManager()
	if err := work.ConfigureChat(f.kernel, runtimeadapter.NewConversations(f.conversations), f.adapter, cache); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(func(c *gin.Context) { contextx.Set(c, &contextx.LoginUser{UserID: user}); c.Next() })
	workhttp.RegisterChatRoutes(router, work)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	var once sync.Once
	t.Cleanup(func() { once.Do(func() { close(release) }) })
	type response struct {
		wire []byte
		err  error
	}
	finished := make(chan response, 1)
	go func() {
		body, _ := json.Marshal(workdomain.ChatRequest{RequestID: key + "turn", Question: "create saved document", Action: "create_document"})
		r, err := http.Post(server.URL+"/work/topics/"+topic.ID+"/chat", "application/json", bytes.NewReader(body))
		if err != nil {
			finished <- response{err: err}
			return
		}
		defer r.Body.Close()
		wire, err := io.ReadAll(r.Body)
		finished <- response{wire: wire, err: err}
	}()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("Work write/model did not start")
	}
	var task string
	if err := db.Raw(`SELECT id FROM t_work_turn WHERE user_id=? AND topic_id=?`, user, topic.ID).Scan(&task).Error; err != nil || task == "" {
		t.Fatal("turn missing", err)
	}
	expireExecution(t, db, task)
	// Let the late worker return before any explicit scan. The HTTP finalizer must
	// recover interruption rather than overwriting the turn as failed.
	once.Do(func() { close(release) })
	select {
	case result := <-finished:
		if result.err != nil || !bytes.Contains(result.wire, []byte(`"status":"interrupted"`)) || !bytes.Contains(result.wire, []byte("event: done")) {
			t.Fatalf("late HTTP result: %s %v", result.wire, result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("late Work stream hung")
	}
	turn, err := work.Store.GetTurn(ctx, user, topic.ID, task)
	if err != nil || turn.Status != "interrupted" || len(turn.Outputs) != 1 {
		t.Fatalf("late turn result: %+v %v", turn, err)
	}
	doc, err := work.Store.GetArtifact(ctx, user, topic.ID, turn.Outputs[0].ArtifactID)
	if err != nil || doc.Version.Revision != 1 || doc.Version.Body.Text() != "已保存正文" {
		t.Fatal("saved Work document lost", err)
	}
	if calls.Load() != 2 {
		t.Fatal("recovery repeated model")
	}
}

func TestChatExecutionCrashHelper(t *testing.T) {
	stage := os.Getenv("CODEX_EXECUTION_CRASH_STAGE")
	if stage == "" {
		t.Skip("subprocess helper")
	}
	db := testDB(t)
	f := newPublicationFixture(t, db, os.Getenv("CODEX_EXECUTION_USER"), os.Getenv("CODEX_EXECUTION_ID"))
	input, err := f.chat.Admit(context.Background(), f.input)
	if err != nil {
		t.Fatal(err)
	}
	if stage == "accepted" {
		os.Exit(74)
	}
	f.kernel.Model = executionModelFunc(func(context.Context, convruntime.ModelRequest, func(convruntime.ModelEvent) error) (convruntime.Turn, error) {
		os.Exit(74)
		return convruntime.Turn{}, nil
	})
	f.chat.Chat(context.Background(), input, nil)
	t.Fatal("child did not crash")
}

func TestChatExecutionSurvivesProcessExit(t *testing.T) {
	for _, stage := range []string{"accepted", "running"} {
		t.Run(stage, func(t *testing.T) {
			db := testDB(t)
			user, _ := testIdentity(t)
			id := publicationID(t)
			exe, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestChatExecutionCrashHelper$")
			cmd.Env = append(os.Environ(), "CODEX_EXECUTION_CRASH_STAGE="+stage, "CODEX_EXECUTION_USER="+user, "CODEX_EXECUTION_ID="+id)
			out, err := cmd.CombinedOutput()
			var exited *exec.ExitError
			if !errors.As(err, &exited) || exited.ExitCode() != 74 {
				t.Fatalf("child: %s %v", out, err)
			}
			fresh := newPublicationFixture(t, db, user, id)
			expireExecution(t, db, id)
			event := recoverExecutionConcurrently(t, db, user, id)
			assertExecutionHTTPReplay(t, fresh, event)
			if fresh.session(t).Status != convruntime.StatusInterrupted || fresh.model.calls.Load() != 0 {
				t.Fatal("restart reran model")
			}
		})
	}
}
