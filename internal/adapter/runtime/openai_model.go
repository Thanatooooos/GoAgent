package runtimeadapter

import (
	"context"
	"fmt"

	"local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/infra-ai/chat"
	"local/rag-project/internal/infra-ai/model"
)

const (
	SiliconFlowProviderID           = "siliconflow"
	SiliconFlowDeepSeekV4FlashModel = "deepseek-ai/DeepSeek-V4-Flash"
)

type OpenAIModel struct {
	Client   chat.NativeStreamClient
	Target   model.ModelTarget
	Thinking bool
}

// NewSiliconFlowDeepSeekV4Flash fixes runtime to its single approved model.
// Provider credentials and endpoint remain deployment configuration.
func NewSiliconFlowDeepSeekV4Flash(client chat.NativeStreamClient, provider config.ProviderConfig, thinking bool) OpenAIModel {
	return OpenAIModel{
		Client: client,
		Target: model.ModelTarget{
			Id:        "runtime-deepseek-v4-flash",
			Candidate: config.ModelCandidate{Id: "runtime-deepseek-v4-flash", Provider: SiliconFlowProviderID, Model: SiliconFlowDeepSeekV4FlashModel},
			Provider:  provider,
		},
		Thinking: thinking,
	}
}

func (m OpenAIModel) Stream(ctx context.Context, request runtime.ModelRequest, emit func(runtime.ModelEvent) error) (runtime.Turn, error) {
	if m.Client == nil {
		return runtime.Turn{}, fmt.Errorf("native stream client is required")
	}
	builder := &runtime.TurnBuilder{}
	callback := nativeCallback{builder: builder, emit: emit}
	thinking := m.Thinking
	if request.Thinking != nil {
		thinking = *request.Thinking
	}
	if err := m.Client.StreamNative(ctx, toNativeRequest(request, thinking), m.Target, callback); err != nil {
		return runtime.Turn{}, err
	}
	return builder.Build()
}
func toNativeRequest(request runtime.ModelRequest, thinking bool) chat.NativeRequest {
	result := chat.NativeRequest{System: append([]string(nil), request.System...), Thinking: thinking, JSONMode: request.JSONMode}
	for _, message := range request.Messages {
		item := chat.NativeMessage{Role: string(message.Role), Content: message.Content, ToolCallID: message.ToolCallID}
		for _, call := range message.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, chat.NativeToolCall{ID: call.ID, Name: call.CapabilityID, Arguments: call.Arguments})
		}
		result.Messages = append(result.Messages, item)
	}
	for _, tool := range request.Tools {
		result.Tools = append(result.Tools, chat.NativeTool{Name: tool.ID, Description: tool.Description, Parameters: tool.JSONSchema})
	}
	return result
}

type nativeCallback struct {
	builder *runtime.TurnBuilder
	emit    func(runtime.ModelEvent) error
}

func (c nativeCallback) OnContent(text string) error {
	return c.add(runtime.ModelEvent{Kind: runtime.ModelEventContent, Text: text})
}
func (c nativeCallback) OnThinking(text string) error {
	return c.add(runtime.ModelEvent{Kind: runtime.ModelEventThinking, Text: text})
}
func (c nativeCallback) OnToolCall(call chat.ToolCallDelta) error {
	return c.add(runtime.ModelEvent{Kind: runtime.ModelEventToolCall, ToolCallIndex: call.Index, ToolCallID: call.ID, ToolName: call.Name, Arguments: []byte(call.Arguments)})
}
func (c nativeCallback) OnComplete(reason string) error {
	return c.add(runtime.ModelEvent{Kind: runtime.ModelEventFinish, FinishReason: reason})
}
func (c nativeCallback) add(event runtime.ModelEvent) error {
	if err := c.builder.Add(event); err != nil {
		return err
	}
	if c.emit != nil {
		return c.emit(event)
	}
	return nil
}
