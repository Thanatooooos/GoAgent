package runtime

import (
	"context"
	"testing"
)

type admissionRuntimeStub struct {
	recordingRuntime
	admissions []RunRequest
}

func (r *admissionRuntimeStub) Admit(_ context.Context, input RunRequest) error {
	r.admissions = append(r.admissions, input)
	return input.Validate()
}

func TestChatAdmissionPersistsIdentityWithoutModelAndReusesUserMessage(t *testing.T) {
	r := &admissionRuntimeStub{recordingRuntime: recordingRuntime{result: RunResult{Status: StatusCompleted, AssistantContent: "answer"}}}
	messages := &messageStoreStub{}
	s := NewChatService(conversationStub{}, messages, r)
	s.SetTaskAccessResolver(func(context.Context, string, string) (bool, error) { return true, nil })
	input, err := s.Admit(context.Background(), ChatInput{TaskID: "task", TraceID: "task", UserID: "owner", ConversationID: "conversation", Question: "question", DeepThinking: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages.created) != 1 || len(r.admissions) != 1 || r.request.Question != "" || r.admissions[0].UserMessageID != messages.created[0].ID {
		t.Fatalf("admission did not establish identity: %+v %+v", messages.created, r)
	}
	if _, err = s.Chat(context.Background(), input, nil); err != nil {
		t.Fatal(err)
	}
	if len(messages.created) != 2 || r.request.UserMessageID != r.admissions[0].UserMessageID || !r.admissions[0].DeepThinking || !r.request.DeepThinking {
		t.Fatal("admitted user/session was not reused")
	}
}

func TestAdmittedChatCannotChangeThinkingMode(t *testing.T) {
	r := &admissionRuntimeStub{}
	s := NewChatService(conversationStub{}, &messageStoreStub{}, r)
	s.SetTaskAccessResolver(func(context.Context, string, string) (bool, error) { return true, nil })
	input, err := s.Admit(context.Background(), ChatInput{TaskID: "task", TraceID: "task", UserID: "owner", ConversationID: "conversation", Question: "question", DeepThinking: true})
	if err != nil {
		t.Fatal(err)
	}
	input.DeepThinking = false
	if _, err := s.Chat(context.Background(), input, nil); err == nil || r.request.Question != "" {
		t.Fatalf("changed mode executed: err=%v request=%+v", err, r.request)
	}
}

func TestDeletedAdmittedChatCannotStartModelOrCreateMoreMessages(t *testing.T) {
	r := &admissionRuntimeStub{}
	messages := &messageStoreStub{}
	s := NewChatService(conversationStub{}, messages, r)
	s.SetTaskAccessResolver(func(context.Context, string, string) (bool, error) { return false, nil })
	input, err := s.Admit(context.Background(), ChatInput{TaskID: "task", TraceID: "task", UserID: "owner", ConversationID: "conversation", Question: "question"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Chat(context.Background(), input, nil); err == nil || len(messages.created) != 1 || r.request.Question != "" {
		t.Fatalf("deleted admitted chat ran: %v %+v", err, r)
	}
}
