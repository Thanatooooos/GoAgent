package runtime

import (
	"context"
	"fmt"
	"testing"
)

func TestConversationGuardRunsBeforeAnyMessageOrModel(t *testing.T) {
	messages := &messageStoreStub{}
	model := &recordingRuntime{}
	s := NewChatService(conversationStub{}, messages, model)
	s.SetConversationGuard(func(context.Context, string, string) error { return fmt.Errorf("Work endpoint required") })
	_, err := s.Chat(context.Background(), ChatInput{TaskID: "task", ConversationID: "work", UserID: "owner", Question: "question", TraceID: "trace"}, nil)
	if err == nil || len(messages.created) != 0 || model.request.Question != "" {
		t.Fatalf("guard leaked writes or execution: %v %+v", err, messages.created)
	}
}
