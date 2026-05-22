package runtime

import (
	"context"
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/log"
	aichat "local/rag-project/internal/infra-ai/chat"
)

// LLMObserver lets the LLM decide whether the agent loop should continue.
// RuleObserver remains as a guardrail fallback when the model output is missing or invalid.
type LLMObserver struct {
	chatService aichat.LLMService
	fallback    Observer
}

func NewLLMObserver(chatService aichat.LLMService) *LLMObserver {
	return &LLMObserver{
		chatService: chatService,
		fallback:    NewRuleObserver(),
	}
}

func (o *LLMObserver) SetFallback(observer Observer) {
	if o == nil || observer == nil {
		return
	}
	o.fallback = observer
}

func (o *LLMObserver) Observe(ctx context.Context, input ObserveInput) (ObserveResult, error) {
	if o == nil || o.chatService == nil {
		return o.observeWithFallback(ctx, input)
	}
	if len(input.RoundResults) == 0 || input.ReachedMaxLoop {
		return o.observeWithFallback(ctx, input)
	}

	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(o.buildSystemPrompt(input)),
			convention.UserMessage(o.BuildUserPrompt(input)),
		},
	}
	jsonMode := true
	request.JSONMode = &jsonMode

	response, err := o.chatService.ChatWithRequest(request)
	if err != nil {
		log.Warnf("llm observer call failed, falling back to rule observer: %v", err)
		fallback, fbErr := o.observeWithFallback(ctx, input)
		if fbErr != nil {
			return ObserveResult{}, fmt.Errorf("llm observer call: %w; fallback: %v", err, fbErr)
		}
		return fallback, nil
	}

	decision, ok := o.parseResponse(response, input)
	if !ok {
		log.Warnf("llm observer parse failed, falling back to rule observer: response=%s", truncateForLog(response))
		return o.observeWithFallback(ctx, input)
	}
	return decision, nil
}

func (o *LLMObserver) observeWithFallback(ctx context.Context, input ObserveInput) (ObserveResult, error) {
	if o != nil && o.fallback != nil {
		return o.fallback.Observe(ctx, input)
	}
	return NewRuleObserver().Observe(ctx, input)
}

func truncateForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= 300 {
		return raw
	}
	return raw[:300] + "..."
}
