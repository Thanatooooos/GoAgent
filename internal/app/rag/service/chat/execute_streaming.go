package chat

import (
	"strings"
	"sync"

	ragtool "local/rag-project/internal/app/rag/tool/core"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type ragChatWorkflowEventSink struct {
	sink RagChatEventSink
}

func (s ragChatWorkflowEventSink) OnAgentThink(message string) error {
	if s.sink == nil {
		return nil
	}
	return s.sink.SendAgentThink(message)
}

func (s ragChatWorkflowEventSink) OnToolStart(event ragtool.ToolCallEvent) error {
	if s.sink == nil {
		return nil
	}
	return s.sink.SendToolStart(event)
}

func (s ragChatWorkflowEventSink) OnToolResult(event ragtool.ToolCallEvent) error {
	if s.sink == nil {
		return nil
	}
	return s.sink.SendToolResult(event)
}

type ragChatStreamCallback struct {
	task *ragChatTask
	sink RagChatEventSink

	estimator            TokenEstimator
	promptTokensEstimate int

	mu       sync.Mutex
	content  strings.Builder
	thinking strings.Builder
}

func newRagChatStreamCallback(
	task *ragChatTask,
	sink RagChatEventSink,
	estimator TokenEstimator,
	promptTokensEstimate int,
) *ragChatStreamCallback {
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}
	callback := &ragChatStreamCallback{
		task:                 task,
		sink:                 sink,
		estimator:            estimator,
		promptTokensEstimate: promptTokensEstimate,
	}
	go callback.watchCancel()
	return callback
}

func (c *ragChatStreamCallback) OnContent(content string) {
	c.mu.Lock()
	c.content.WriteString(content)
	c.mu.Unlock()
	_ = c.sink.SendMessage(content)
}

func (c *ragChatStreamCallback) OnThinking(content string) {
	c.mu.Lock()
	c.thinking.WriteString(content)
	c.mu.Unlock()
	_ = c.sink.SendThinking(content)
}

func (c *ragChatStreamCallback) OnComplete() {
	c.task.doneCh <- c.buildTaskResult(nil)
}

func (c *ragChatStreamCallback) OnError(err error) {
	c.task.doneCh <- c.buildTaskResult(err)
}

func (c *ragChatStreamCallback) buildTaskResult(err error) ragChatTaskResult {
	content := c.currentContent()
	thinking := c.currentThinking()
	completionTokens := c.estimator.EstimateTokens(content) + c.estimator.EstimateTokens(thinking)
	return ragChatTaskResult{
		content:     content,
		thinking:    thinking,
		err:         err,
		tokenUsage:  aichat.EstimatedTokenUsage(c.promptTokensEstimate, completionTokens),
		usageSource: "estimated",
	}
}

func (c *ragChatStreamCallback) watchCancel() {
	<-c.task.cancelCh
	result := c.buildTaskResult(nil)
	result.cancelled = true
	c.task.doneCh <- result
}

func (c *ragChatStreamCallback) currentContent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.content.String()
}

func (c *ragChatStreamCallback) currentThinking() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.thinking.String()
}
