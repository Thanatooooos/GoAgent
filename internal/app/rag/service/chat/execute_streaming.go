package chat

import (
	"strings"
	"sync"

	ragcitation "local/rag-project/internal/app/rag/core/citation"
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
	expander             *ragcitation.StreamExpander

	mu       sync.Mutex
	content  strings.Builder
	thinking strings.Builder
}

func newRagChatStreamCallback(
	task *ragChatTask,
	sink RagChatEventSink,
	estimator TokenEstimator,
	promptTokensEstimate int,
	expander *ragcitation.StreamExpander,
) *ragChatStreamCallback {
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}
	callback := &ragChatStreamCallback{
		task:                 task,
		sink:                 sink,
		estimator:            estimator,
		promptTokensEstimate: promptTokensEstimate,
		expander:             expander,
	}
	go callback.watchCancel()
	return callback
}

func (c *ragChatStreamCallback) OnContent(content string) {
	c.mu.Lock()
	expanded := content
	if c.expander != nil {
		expanded = c.expander.Feed(content)
	}
	c.content.WriteString(expanded)
	c.mu.Unlock()
	_ = c.sink.SendMessage(expanded)
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
	content := c.finalizeContent()
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

// finalizeContent flushes any expander tail and returns the fully expanded
// content for persistence.
func (c *ragChatStreamCallback) finalizeContent() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.expander != nil {
		rest := c.expander.Flush()
		if rest != "" {
			c.content.WriteString(rest)
		}
	}
	return c.content.String()
}

func (c *ragChatStreamCallback) currentThinking() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.thinking.String()
}
