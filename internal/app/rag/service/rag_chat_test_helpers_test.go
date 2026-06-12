package service

import (
	"testing"

	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

func minimalRagChatDeps() RagChatDeps {
	return RagChatDeps{
		ConversationService: &ConversationService{},
		MessageService:      &ConversationMessageService{},
		HistoryService:      memoryServiceStub{},
		RetrieveService:     &retrieveServiceStub{},
		PromptService:       ragprompt.NewService(nil),
		ChatService:         &llmServiceStub{},
		Tracer:              NewChatTracer(nil, nil),
	}
}

func mustNewTestRagChatService(t *testing.T, deps RagChatDeps, opts RagChatOptions) *RagChatService {
	t.Helper()
	service, err := NewRagChatServiceWithDeps(deps, opts)
	if err != nil {
		t.Fatalf("NewRagChatServiceWithDeps() error = %v", err)
	}
	return service
}

func newTestRagChatServiceWithRetrieve(t *testing.T, retrieve ragretrieve.Service, tracer *ChatTracer, opts RagChatOptions) *RagChatService {
	t.Helper()
	deps := minimalRagChatDeps()
	deps.RetrieveService = retrieve
	if tracer != nil {
		deps.Tracer = tracer
	}
	return mustNewTestRagChatService(t, deps, opts)
}
