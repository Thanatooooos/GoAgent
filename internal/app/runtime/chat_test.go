package runtime

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestChatServicePersistsOnlyUserAndFinalAssistant(t *testing.T) {
	t.Parallel()
	messages := &messageStoreStub{}
	service := NewChatService(conversationStub{}, messages, runtimeStub{result: RunResult{Status: StatusCompleted, AssistantContent: "answer"}})
	result, err := service.Chat(context.Background(), ChatInput{TaskID: "task", ConversationID: "c", UserID: "u", Question: "question", TraceID: "trace"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.created) != 2 || messages.created[0].Role != ModelRoleUser || messages.created[1].Role != ModelRoleAssistant || result.Runtime.AssistantContent != "answer" {
		t.Fatalf("messages=%#v result=%#v", messages.created, result)
	}
}

func TestChatServiceCancelTaskCancelsOnlyItsRuntimeContext(t *testing.T) {
	t.Parallel()
	started := make(chan struct{})
	service := NewChatService(conversationStub{}, &messageStoreStub{}, blockingRuntimeStub{started: started})
	done := make(chan error, 1)
	go func() {
		_, err := service.Chat(context.Background(), ChatInput{TaskID: "task", ConversationID: "c", UserID: "u", Question: "question", TraceID: "trace"}, nil)
		done <- err
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runtime did not start")
	}
	if !service.CancelTask("task") {
		t.Fatal("cancel task = false")
	}
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("chat did not stop")
	}
}

func TestChatServiceDoesNotPersistAssistantAfterRuntimeFailure(t *testing.T) {
	t.Parallel()
	messages := &messageStoreStub{}
	service := NewChatService(conversationStub{}, messages, runtimeStub{err: assertRuntimeError{}})
	_, err := service.Chat(context.Background(), ChatInput{TaskID: "task", ConversationID: "c", UserID: "u", Question: "question", TraceID: "trace"}, nil)
	if err == nil || len(messages.created) != 1 {
		t.Fatalf("err=%v messages=%#v", err, messages.created)
	}
}

func TestChatServiceCompletesPendingEpisodesAfterAssistantPersistence(t *testing.T) {
	finalizer := &episodeRuntimeStub{result: RunResult{Status: StatusCompleted, AssistantContent: "answer", RuntimeSessionID: "session-1"}}
	service := NewChatService(conversationStub{}, &messageStoreStub{}, finalizer)
	if _, err := service.Chat(context.Background(), ChatInput{TaskID: "task", ConversationID: "c", UserID: "u", Question: "question", TraceID: "trace"}, nil); err != nil {
		t.Fatal(err)
	}
	if finalizer.completedSession != "session-1" || finalizer.completedMessage == "" {
		t.Fatalf("finalizer=%#v", finalizer)
	}
}

func TestSelectKnowledgeBasesUsesAllAllowedWhenNothingWasSelected(t *testing.T) {
	got := selectKnowledgeBases([]string{" kb-1 ", "kb-2", "kb-1"}, nil)
	if len(got) != 2 || got[0] != "kb-1" || got[1] != "kb-2" {
		t.Fatalf("all allowed selection = %#v", got)
	}
}

func TestSelectKnowledgeBasesCannotExpandAllowedScope(t *testing.T) {
	got := selectKnowledgeBases([]string{"kb-1"}, []string{"kb-2", "kb-1"})
	if len(got) != 1 || got[0] != "kb-1" {
		t.Fatalf("restricted selection = %#v", got)
	}
}

func TestChatServiceResolvesKnowledgeBaseScopeBeforeRuntime(t *testing.T) {
	runtime := &recordingRuntime{result: RunResult{Status: StatusCompleted, AssistantContent: "answer"}}
	service := NewChatService(conversationStub{}, &messageStoreStub{}, runtime)
	service.SetKnowledgeBaseAccessResolver(accessResolverStub{ids: []string{"kb-1", "kb-2"}})
	_, err := service.Chat(context.Background(), ChatInput{TaskID: "task", ConversationID: "c", UserID: "u", Question: "question", KnowledgeBaseIDs: []string{"kb-2", "not-allowed"}, TraceID: "trace"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(runtime.request.KnowledgeBaseIDs) != 1 || runtime.request.KnowledgeBaseIDs[0] != "kb-2" {
		t.Fatalf("runtime knowledge base scope = %#v", runtime.request.KnowledgeBaseIDs)
	}
}

type messageStoreStub struct{ created []ConversationMessage }

type conversationStub struct{}

func (conversationStub) Ensure(context.Context, string, string, string) error { return nil }

func (s *messageStoreStub) Create(_ context.Context, message ConversationMessage) (ConversationMessage, error) {
	message.ID = "message-" + string(rune('1'+len(s.created)))
	s.created = append(s.created, message)
	return message, nil
}

type runtimeStub struct {
	result RunResult
	err    error
}

type accessResolverStub struct{ ids []string }

func (s accessResolverStub) AccessibleIDs(context.Context, string) ([]string, error) {
	return s.ids, nil
}

type recordingRuntime struct {
	request RunRequest
	result  RunResult
}

func (s *recordingRuntime) Run(_ context.Context, request RunRequest, _ EventSink) (RunResult, error) {
	s.request = request
	return s.result, nil
}

type episodeRuntimeStub struct {
	runtimeStub
	result                             RunResult
	completedSession, completedMessage string
}

func (s *episodeRuntimeStub) Run(ctx context.Context, request RunRequest, sink EventSink) (RunResult, error) {
	return s.result, nil
}
func (s *episodeRuntimeStub) CompleteEpisodes(_ context.Context, session, message string) error {
	s.completedSession, s.completedMessage = session, message
	return nil
}
func (*episodeRuntimeStub) DiscardEpisodes(context.Context, string) error { return nil }

func (s runtimeStub) Run(context.Context, RunRequest, EventSink) (RunResult, error) {
	return s.result, s.err
}

type assertRuntimeError struct{}

func (assertRuntimeError) Error() string { return "runtime failed" }

type blockingRuntimeStub struct{ started chan<- struct{} }

func (s blockingRuntimeStub) Run(ctx context.Context, _ RunRequest, _ EventSink) (RunResult, error) {
	close(s.started)
	<-ctx.Done()
	return RunResult{Status: StatusCancelled}, ctx.Err()
}
