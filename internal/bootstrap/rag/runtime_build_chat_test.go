package rag

import (
	"testing"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	infraai "local/rag-project/internal/infra-ai"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type bootstrapLLMServiceStub struct{}

func (bootstrapLLMServiceStub) Chat(string) (string, error) {
	return "", nil
}

func (bootstrapLLMServiceStub) ChatWithRequest(convention.ChatRequest) (string, error) {
	return "", nil
}

func (bootstrapLLMServiceStub) ChatWithModel(convention.ChatRequest, string) (string, error) {
	return "", nil
}

func (bootstrapLLMServiceStub) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

func (bootstrapLLMServiceStub) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, nil
}

var _ aichat.LLMService = (*bootstrapLLMServiceStub)(nil)

func TestBuildChatContextBudgetOptionsIncludesStageBudgets(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.ChatContext.MaxPromptTokens = 8000
	cfg.Rag.Memory.ChatContext.FixedReserveTokens = 800
	cfg.Rag.Memory.ChatContext.SafetyReserveTokens = 500
	cfg.Rag.Memory.ChatContext.StageBudget.MemoryTokens = 500
	cfg.Rag.Memory.ChatContext.StageBudget.SessionRecallTokens = 1500
	cfg.Rag.Memory.ChatContext.StageBudget.RetrieveTokens = 2000
	cfg.Rag.Memory.ChatContext.StageBudget.ToolTokens = 1500
	cfg.Rag.Memory.SummaryToken.MessageOverheadTokens = 4

	got := buildChatContextBudgetOptions(cfg)
	if got.FixedReserveTokens != 800 ||
		got.SafetyReserveTokens != 500 ||
		got.MemoryTokens != 500 ||
		got.SessionRecallTokens != 1500 ||
		got.RetrieveTokens != 2000 ||
		got.ToolTokens != 1500 ||
		got.MessageOverheadTokens != 4 {
		t.Fatalf("unexpected chat context budget options: %+v", got)
	}
}

func TestBuildLongTermMemoryWriteback(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		buildCtx *buildContext
		memory   memoryBundle
		wantNil  bool
	}{
		{
			name: "returns nil without build context",
			memory: memoryBundle{
				explicitMemoryService: &longtermmemory.MemoryService{},
			},
			wantNil: true,
		},
		{
			name:     "returns nil without ai runtime",
			buildCtx: &buildContext{},
			memory: memoryBundle{
				explicitMemoryService: &longtermmemory.MemoryService{},
			},
			wantNil: true,
		},
		{
			name: "returns nil without chat service",
			buildCtx: &buildContext{
				aiRuntime: &infraai.Runtime{},
			},
			memory: memoryBundle{
				explicitMemoryService: &longtermmemory.MemoryService{},
			},
			wantNil: true,
		},
		{
			name: "returns nil without explicit memory service",
			buildCtx: &buildContext{
				aiRuntime: &infraai.Runtime{Chat: bootstrapLLMServiceStub{}},
			},
			memory:  memoryBundle{},
			wantNil: true,
		},
		{
			name: "returns writeback service when dependencies are present",
			buildCtx: &buildContext{
				aiRuntime: &infraai.Runtime{Chat: bootstrapLLMServiceStub{}},
			},
			memory: memoryBundle{
				explicitMemoryService: &longtermmemory.MemoryService{},
				memoryCacheMetrics:    ragcachemetrics.NewService(),
			},
			wantNil: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := buildLongTermMemoryWriteback(tc.buildCtx, tc.memory)
			if (got == nil) != tc.wantNil {
				t.Fatalf("buildLongTermMemoryWriteback() nil = %v, wantNil %v", got == nil, tc.wantNil)
			}
		})
	}
}
