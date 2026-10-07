package work_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	raghttp "local/rag-project/internal/adapter/http/rag"
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
	"local/rag-project/internal/middleware"
)

type guardedChat struct {
	*conversationruntime.ChatService
	cancels atomic.Int32
}

func (s *guardedChat) CancelTask(id string) bool {
	s.cancels.Add(1)
	return s.ChatService.CancelTask(id)
}

type guardedStreams struct {
	stream.StreamManager
	reads, writes atomic.Int32
	checkMeta     func(context.Context, string) error
}

func (s *guardedStreams) GetEvents(ctx context.Context, id string, offset int) ([]stream.StreamEvent, int, error) {
	s.reads.Add(1)
	return s.StreamManager.GetEvents(ctx, id, offset)
}
func (s *guardedStreams) AppendEvent(ctx context.Context, id string, event stream.StreamEvent) error {
	s.writes.Add(1)
	if event.Name == "meta" && s.checkMeta != nil {
		if err := s.checkMeta(ctx, id); err != nil {
			return err
		}
	}
	return s.StreamManager.AppendEvent(ctx, id, event)
}

type streamAccessModel struct {
	started, release, ended chan struct{}
}

func (m *streamAccessModel) Stream(ctx context.Context, _ conversationruntime.ModelRequest, emit func(conversationruntime.ModelEvent) error) (conversationruntime.Turn, error) {
	close(m.started)
	defer close(m.ended)
	select {
	case <-ctx.Done():
		return conversationruntime.Turn{}, ctx.Err()
	case <-m.release:
	}
	if err := emit(conversationruntime.ModelEvent{Kind: conversationruntime.ModelEventContent, Text: "private sentinel"}); err != nil {
		return conversationruntime.Turn{}, err
	}
	return conversationruntime.Turn{Content: "private sentinel"}, nil
}

func TestChatAndWorkStreamOwnershipAcrossHTTPEntrypoints(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, key := testIdentity(t)
	other := user + "other"
	store := postgresruntime.NewStore(db)
	kernel := &conversationruntime.Runtime{Tools: capability.NewRegistry(), Lifecycle: conversationruntime.NewLifecycle(store, func() (string, error) { id, err := distributedid.NextID(); return strconv.FormatInt(id, 10), err })}
	convRepo := postgresrag.NewConversationRepository(db)
	msgRepo := postgresrag.NewConversationMessageRepository(db)
	summaryRepo := postgresrag.NewConversationSummaryRepository(db)
	conv := ragservice.NewConversationService(convRepo, msgRepo, summaryRepo, nil, nil, 30, postgresrag.NewConversationDeleteTransaction(db))
	messages := ragservice.NewConversationMessageService(convRepo, msgRepo, summaryRepo, nil)
	chat := conversationruntime.NewChatService(runtimeadapter.NewConversations(conv), runtimeadapter.NewConversationMessages(messages), kernel)
	chat.SetConversationGuard(runtimeadapter.OrdinaryConversationGuard(db))
	chat.SetTaskAccessResolver(store.CanAccessChatTask)
	guard := &guardedChat{ChatService: chat}
	underlying := stream.NewMemoryStreamManager()
	streams := &guardedStreams{StreamManager: underlying}
	h := raghttp.NewHandler(conv, messages, nil, nil, nil, streams)
	h.SetRuntimeChat(guard)
	h.SetRuntimeReplay(guard)
	work, err := workbootstrap.NewRuntime(db)
	if err != nil {
		t.Fatal(err)
	}
	if err = work.ConfigureChat(kernel, runtimeadapter.NewConversations(conv), runtimeadapter.NewConversationMessages(messages), streams); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.Use(middleware.ErrorHandlerMiddleware(), func(c *gin.Context) {
		if id := c.GetHeader("X-Test-User"); id != "" {
			contextx.Set(c, &contextx.LoginUser{UserID: id, Role: "user"})
		}
	})
	router.GET("/continue", h.ContinueChat)
	router.POST("/stop", h.StopChat)
	router.GET("/chat", h.Chat)
	workhttp.RegisterChatRoutes(router, work)
	request := func(method, path, as string) *httptest.ResponseRecorder {
		t.Helper()
		req := httptest.NewRequest(method, path, nil)
		req.Header.Set("X-Test-User", as)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		return w
	}
	deny := func(id, as string) {
		t.Helper()
		for _, action := range []string{"continue", "stop"} {
			reads, writes, cancels := streams.reads.Load(), streams.writes.Load(), guard.cancels.Load()
			method := http.MethodGet
			if action == "stop" {
				method = http.MethodPost
			}
			w := request(method, "/"+action+"?taskId="+id, as)
			if w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "private sentinel") || reads != streams.reads.Load() || writes != streams.writes.Load() || cancels != guard.cancels.Load() {
				t.Fatalf("unauthorized %s as %s: %d %s; stream or cancellation accessed", action, as, w.Code, w.Body.String())
			}
		}
	}
	for _, hot := range []bool{false, true} {
		numericID, idErr := distributedid.NextID()
		if idErr != nil {
			t.Fatal(idErr)
		}
		id := strconv.FormatInt(numericID, 10)
		conversation := id
		_, err = chat.Admit(ctx, conversationruntime.ChatInput{TaskID: id, TraceID: id, ConversationID: conversation, UserID: user, Question: "question"})
		if err != nil {
			t.Fatal(err)
		}
		// A replacement service can authorize before Run or any journal exists.
		allowed, err := postgresruntime.NewStore(db).CanAccessChatTask(ctx, user, id)
		if err != nil || !allowed {
			t.Fatalf("admission identity is not durable: %t %v", allowed, err)
		}
		if events, _, err := underlying.GetEvents(ctx, id, 0); err != nil || len(events) != 0 {
			t.Fatal("admission exposed a stream before durable identity")
		}
		session, err := store.FindSessionByTraceID(ctx, user, id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = kernel.Lifecycle.RecordEvent(ctx, session, conversationruntime.EventAnswerDelta, "private sentinel"); err != nil {
			t.Fatal(err)
		}
		if _, err = kernel.Lifecycle.RecordEvent(ctx, session, conversationruntime.EventCompleted, ""); err != nil {
			t.Fatal(err)
		}
		if hot {
			_ = underlying.AppendEvent(ctx, id, stream.StreamEvent{Name: "message", Data: []byte(`{"delta":"private sentinel"}`)})
			_ = underlying.AppendEvent(ctx, id, stream.StreamEvent{Name: "done", Data: []byte(`{}`), Done: true})
		}
		deny(id, other)
		w := request(http.MethodGet, "/continue?taskId="+id, user)
		if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "private sentinel") {
			t.Fatalf("owner replay failed: %d %s", w.Code, w.Body.String())
		}
		w = request(http.MethodPost, "/stop?taskId="+id, user)
		if w.Code != http.StatusOK {
			t.Fatalf("owner stop rejected: %d %s", w.Code, w.Body.String())
		}
		if err = conv.Delete(ctx, ragservice.DeleteConversationInput{UserID: user, ConversationID: conversation}); err != nil {
			t.Fatal(err)
		}
		deny(id, user)
		deny(id, other)
		// Restoring the conversation alone cannot revive a deleted execution's input.
		if err = db.Exec(`UPDATE t_conversation SET deleted=0 WHERE conversation_id=? AND user_id=?`, conversation, user).Error; err != nil {
			t.Fatal(err)
		}
		deny(id, user)
	}
	topic, err := work.Store.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "流归属回归"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := work.Store.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: key + "turn", Question: "private Work question", Action: "discuss"})
	if err != nil {
		t.Fatal(err)
	}
	_ = underlying.AppendEvent(ctx, turn.ID, stream.StreamEvent{Name: "message", Data: []byte(`{"delta":"private sentinel"}`)})
	_ = underlying.AppendEvent(ctx, turn.ID, stream.StreamEvent{Name: "done", Data: []byte(`{}`), Done: true})
	// Accepted Work turn has no runtime session yet, but its warm cache is protected.
	deny(turn.ID, user)
	deny(turn.ID, other)
	if err = kernel.Admit(ctx, conversationruntime.RunRequest{ConversationID: turn.ConversationID, UserID: user, UserMessageID: turn.UserMessageID, Question: turn.Question, TraceID: turn.ID}); err != nil {
		t.Fatal(err)
	}
	deny(turn.ID, user)
	deny(turn.ID, other)
	session, err := store.FindSessionByTraceID(ctx, user, turn.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = kernel.Lifecycle.RecordEvent(ctx, session, conversationruntime.EventAnswerDelta, "private sentinel"); err != nil {
		t.Fatal(err)
	}
	if _, err = kernel.Lifecycle.RecordEvent(ctx, session, conversationruntime.EventCompleted, ""); err != nil {
		t.Fatal(err)
	}
	streams.StreamManager = stream.NewMemoryStreamManager()
	deny(turn.ID, user)
	deny(turn.ID, other)
	deny("nonexistent-task", user)
	path := "/work/topics/" + topic.ID + "/turns/" + turn.ID
	w := request(http.MethodGet, path+"/stream", user)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "private sentinel") {
		t.Fatalf("Work owner lost stream: %d %s", w.Code, w.Body.String())
	}
	for _, suffix := range []string{"/stream", "/stop"} {
		method := http.MethodGet
		if suffix == "/stop" {
			method = http.MethodPost
		}
		reads, writes := streams.reads.Load(), streams.writes.Load()
		w = request(method, path+suffix, other)
		if w.Code == http.StatusOK || reads != streams.reads.Load() || writes != streams.writes.Load() {
			t.Fatalf("cross-user Work access: %d %s", w.Code, w.Body.String())
		}
	}
	if err = conv.Delete(ctx, ragservice.DeleteConversationInput{UserID: user, ConversationID: turn.ConversationID}); err != nil {
		t.Fatal(err)
	}
	deny(turn.ID, user)
	w = request(http.MethodGet, path+"/stream", user)
	if w.Code == http.StatusOK || strings.Contains(w.Body.String(), "private sentinel") {
		t.Fatalf("deleted Work stream remained accessible: %d %s", w.Code, w.Body.String())
	}

	// Exercise formal admission and cancellation with a running model.
	metadataAccess := make(chan error, 2)
	streams.checkMeta = func(ctx context.Context, id string) error {
		allowed, err := store.CanAccessChatTask(ctx, user, id)
		if err == nil && !allowed {
			err = fmt.Errorf("stream ID was exposed before durable admission")
		}
		metadataAccess <- err
		return err
	}
	model := &streamAccessModel{started: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{})}
	kernel.Model = model
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() { response <- request(http.MethodGet, "/chat?question=ownership-live", user) }()
	select {
	case <-model.started:
	case <-time.After(3 * time.Second):
		close(model.release)
		t.Fatal("formal chat did not start")
	}
	if err = <-metadataAccess; err != nil {
		close(model.release)
		t.Fatal(err)
	}
	var liveID string
	if err = db.Raw(`SELECT trace_id FROM t_runtime_session WHERE user_id=? ORDER BY create_time DESC LIMIT 1`, user).Scan(&liveID).Error; err != nil {
		close(model.release)
		t.Fatal(err)
	}
	allowed, err := store.CanAccessChatTask(ctx, user, liveID)
	if err != nil || !allowed {
		close(model.release)
		t.Fatalf("exposed running execution has no owner: %t %v", allowed, err)
	}
	for _, action := range []string{"continue", "stop"} {
		method := http.MethodGet
		if action == "stop" {
			method = http.MethodPost
		}
		w = request(method, "/"+action+"?taskId="+liveID, other)
		if w.Code != http.StatusNotFound {
			close(model.release)
			t.Fatalf("live cross-user access: %d %s", w.Code, w.Body.String())
		}
	}
	select {
	case <-model.ended:
		t.Fatal("unauthorized request stopped model")
	default:
	}
	w = request(http.MethodPost, "/stop?taskId="+liveID, user)
	if w.Code != http.StatusOK {
		close(model.release)
		t.Fatalf("owner cannot stop live chat: %d %s", w.Code, w.Body.String())
	}
	select {
	case w = <-response:
		if !strings.Contains(w.Body.String(), "event: meta") || !strings.Contains(w.Body.String(), "event: cancel") {
			t.Fatalf("chat did not expose admission/cancellation: %s", w.Body.String())
		}
	case <-time.After(3 * time.Second):
		close(model.release)
		t.Fatal("owner cancellation did not finish chat")
	}
	// Successful admission reuses its user message and runtime session exactly once.
	model = &streamAccessModel{started: make(chan struct{}), release: make(chan struct{}), ended: make(chan struct{})}
	close(model.release)
	kernel.Model = model
	w = request(http.MethodGet, "/chat?question=ownership-success", user)
	if err = <-metadataAccess; err != nil {
		t.Fatal(err)
	}
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "private sentinel") || !strings.Contains(w.Body.String(), "event: finish") {
		t.Fatalf("formal chat success failed: %d %s", w.Code, w.Body.String())
	}
	var count int64
	if err = db.Raw(`SELECT COUNT(*) FROM t_message WHERE user_id=? AND content='ownership-success' AND deleted=0`, user).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("admission duplicated user message: %d %v", count, err)
	}
	if err = db.Raw(`SELECT COUNT(*) FROM t_runtime_session r JOIN t_message m ON m.id=r.user_message_id WHERE r.user_id=? AND m.content='ownership-success'`, user).Scan(&count).Error; err != nil || count != 1 {
		t.Fatalf("admission duplicated runtime session: %d %v", count, err)
	}
}
