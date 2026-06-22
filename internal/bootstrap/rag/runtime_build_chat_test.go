package rag

import (
	"testing"

	ragcachemetrics "local/rag-project/internal/app/rag/cachemetrics"
	"local/rag-project/internal/app/rag/service/longtermmemory"
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
