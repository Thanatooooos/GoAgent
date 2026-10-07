package runtimeadapter

import (
	"context"
	"sync"
	"testing"

	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/model"
)

func TestOpenAIModelMapsRequestAndClassifiedStream(t *testing.T) {
	t.Parallel()
	client := &nativeClientStub{}
	adapter := OpenAIModel{Client: client, Target: model.ModelTarget{}, Thinking: true}
	request := runtime.ModelRequest{JSONMode: true, System: []string{"core"}, Messages: []runtime.ModelMessage{{Role: runtime.ModelRoleUser, Content: "hello"}}, Tools: []capability.ModelDefinition{{ID: "lookup", Description: "lookup", JSONSchema: []byte(`{"type":"object"}`)}}}
	turn, err := adapter.Stream(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !client.request.JSONMode || len(client.request.System) != 1 || len(client.request.Messages) != 1 || len(client.request.Tools) != 1 || !client.request.Thinking {
		t.Fatalf("native request = %#v", client.request)
	}
	if turn.Thinking != "think" || turn.Content != "answer" || len(turn.ToolCalls) != 1 || string(turn.ToolCalls[0].Arguments) != `{"q":"x"}` {
		t.Fatalf("turn = %#v", turn)
	}
}

type concurrentThinkingClient struct{ requests chan chat.NativeRequest }

func (s concurrentThinkingClient) StreamNative(_ context.Context, request chat.NativeRequest, _ model.ModelTarget, callback chat.NativeStreamCallback) error {
	s.requests <- request
	if request.Thinking {
		if err := callback.OnThinking("reasoning"); err != nil {
			return err
		}
	}
	if err := callback.OnContent("answer"); err != nil {
		return err
	}
	return callback.OnComplete("stop")
}

func TestOpenAIModelThinkingOverrideIsIsolatedAcrossConcurrentRequests(t *testing.T) {
	client := concurrentThinkingClient{requests: make(chan chat.NativeRequest, 20)}
	// A false per-request value must override even a true adapter default.
	adapter := OpenAIModel{Client: client, Thinking: true}
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(thinking bool) {
			defer wg.Done()
			turn, err := adapter.Stream(context.Background(), runtime.ModelRequest{Thinking: &thinking}, nil)
			if err != nil || (turn.Thinking != "") != thinking || turn.Content != "answer" {
				t.Errorf("thinking=%t turn=%+v err=%v", thinking, turn, err)
			}
		}(i%2 == 0)
	}
	wg.Wait()
	close(client.requests)
	enabled := 0
	for request := range client.requests {
		if request.Thinking {
			enabled++
		}
	}
	if enabled != 10 || !adapter.Thinking {
		t.Fatalf("enabled=%d adapter default=%t", enabled, adapter.Thinking)
	}
}

func TestNewSiliconFlowDeepSeekV4FlashFixesProviderAndModel(t *testing.T) {
	t.Parallel()
	client := &nativeClientStub{}
	adapter := NewSiliconFlowDeepSeekV4Flash(client, config.ProviderConfig{Url: "https://api.siliconflow.cn"}, true)
	if adapter.Client != client || adapter.Target.Candidate.Provider != SiliconFlowProviderID || adapter.Target.Candidate.Model != SiliconFlowDeepSeekV4FlashModel || !adapter.Thinking {
		t.Fatalf("adapter = %#v", adapter)
	}
}

type nativeClientStub struct{ request chat.NativeRequest }

func (s *nativeClientStub) StreamNative(_ context.Context, request chat.NativeRequest, _ model.ModelTarget, callback chat.NativeStreamCallback) error {
	s.request = request
	if err := callback.OnThinking("think"); err != nil {
		return err
	}
	if err := callback.OnContent("answer"); err != nil {
		return err
	}
	if err := callback.OnToolCall(chat.ToolCallDelta{Index: 0, ID: "call-1", Name: "lookup", Arguments: `{"q":`}); err != nil {
		return err
	}
	if err := callback.OnToolCall(chat.ToolCallDelta{Index: 0, Arguments: `"x"}`}); err != nil {
		return err
	}
	return callback.OnComplete("tool_calls")
}
