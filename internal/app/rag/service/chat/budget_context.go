package chat

import (
	"strings"

	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	"local/rag-project/internal/framework/convention"
)

const (
	defaultChatContextMaxPromptTokens = 8000

	degradeStepTrimHistory          = "trim_history"
	degradeStepTrimToolContext      = "trim_tool_context"
	degradeStepTrimKnowledgeContext = "trim_knowledge_context"
	degradeStepTrimSessionContext   = "trim_session_context"
	degradeStepTrimMemoryContext    = "trim_memory_context"
)

// ChatContextBudgetOptions controls token-aware history trimming before prompt build.
type ChatContextBudgetOptions struct {
	Enabled         bool
	MaxPromptTokens int
	Estimator       TokenEstimator
}

// ChatContextBudgetResult captures trimming and estimation metadata for tracing.
type ChatContextBudgetResult struct {
	History                   []convention.ChatMessage
	MemoryContext             string
	SessionContext            string
	KnowledgeContext          string
	ToolContext               string
	EstimatedPromptTokens     int
	HistoryMessageCountBefore int
	HistoryMessageCountAfter  int
	DroppedHistoryMessages    int
	Trimmed                   bool
	Degraded                  bool
	DegradationSteps          []string
}

func (o ChatContextBudgetOptions) normalized() ChatContextBudgetOptions {
	if o.Estimator == nil {
		o.Estimator = RoughTokenEstimator{}
	}
	if o.MaxPromptTokens <= 0 {
		o.MaxPromptTokens = defaultChatContextMaxPromptTokens
	}
	return o
}

func applyChatContextBudget(
	options ChatContextBudgetOptions,
	promptService *ragprompt.Service,
	ctx ragprompt.Context,
) (ChatContextBudgetResult, error) {
	working := clonePromptContext(ctx)
	result := ChatContextBudgetResult{
		History:                   append([]convention.ChatMessage(nil), working.History...),
		MemoryContext:             working.MemoryContext,
		SessionContext:            working.SessionContext,
		KnowledgeContext:          working.KnowledgeContext,
		ToolContext:               working.ToolContext,
		HistoryMessageCountBefore: len(working.History),
		HistoryMessageCountAfter:  len(working.History),
	}
	if promptService == nil {
		return result, nil
	}

	options = options.normalized()
	estimated, err := estimatePromptContextTokens(promptService, working, options.Estimator)
	if err != nil {
		return ChatContextBudgetResult{}, err
	}
	result.EstimatedPromptTokens = estimated
	if !options.Enabled || estimated <= options.MaxPromptTokens {
		return result, nil
	}

	trimmedHistory, dropped := trimHistoryToPromptBudget(options, promptService, working, working.History)
	if dropped > 0 {
		working.History = trimmedHistory
		result.History = append([]convention.ChatMessage(nil), trimmedHistory...)
		result.HistoryMessageCountAfter = len(trimmedHistory)
		result.DroppedHistoryMessages = dropped
		result.Trimmed = true
		result.Degraded = true
		result.DegradationSteps = append(result.DegradationSteps, degradeStepTrimHistory)
	}

	degradableFields := []struct {
		step  string
		value *string
	}{
		{degradeStepTrimToolContext, &working.ToolContext},
		{degradeStepTrimKnowledgeContext, &working.KnowledgeContext},
		{degradeStepTrimSessionContext, &working.SessionContext},
		{degradeStepTrimMemoryContext, &working.MemoryContext},
	}
	for _, field := range degradableFields {
		stepRecorded := false
		for strings.TrimSpace(*field.value) != "" {
			estimated, err = estimatePromptContextTokens(promptService, working, options.Estimator)
			if err != nil {
				return ChatContextBudgetResult{}, err
			}
			if estimated <= options.MaxPromptTokens {
				break
			}
			trimmed, changed := shrinkContextText(*field.value, options.Estimator)
			if !changed {
				*field.value = ""
				break
			}
			*field.value = trimmed
			result.Degraded = true
			result.Trimmed = true
			if !stepRecorded {
				result.DegradationSteps = append(result.DegradationSteps, field.step)
				stepRecorded = true
			}
		}
		estimated, err = estimatePromptContextTokens(promptService, working, options.Estimator)
		if err != nil {
			return ChatContextBudgetResult{}, err
		}
		if estimated <= options.MaxPromptTokens {
			break
		}
	}

	result.MemoryContext = working.MemoryContext
	result.SessionContext = working.SessionContext
	result.KnowledgeContext = working.KnowledgeContext
	result.ToolContext = working.ToolContext

	finalEstimate, err := estimatePromptContextTokens(promptService, working, options.Estimator)
	if err != nil {
		return ChatContextBudgetResult{}, err
	}
	result.EstimatedPromptTokens = finalEstimate
	return result, nil
}

func clonePromptContext(ctx ragprompt.Context) ragprompt.Context {
	return ragprompt.Context{
		Question:         ctx.Question,
		MemoryContext:    ctx.MemoryContext,
		SessionContext:   ctx.SessionContext,
		KnowledgeContext: ctx.KnowledgeContext,
		ToolContext:      ctx.ToolContext,
		WorkflowPolicy:   ctx.WorkflowPolicy,
		AnswerGuidance:   ctx.AnswerGuidance,
		History:          append([]convention.ChatMessage(nil), ctx.History...),
		SystemPromptKey:  ctx.SystemPromptKey,
		SystemPrompt:     ctx.SystemPrompt,
	}
}

func estimatePromptContextTokens(
	promptService *ragprompt.Service,
	ctx ragprompt.Context,
	estimator TokenEstimator,
) (int, error) {
	messages, err := promptService.BuildMessages(ctx)
	if err != nil {
		return 0, err
	}
	return estimateChatMessagesTokens(messages, estimator), nil
}

func trimHistoryToPromptBudget(
	options ChatContextBudgetOptions,
	promptService *ragprompt.Service,
	ctx ragprompt.Context,
	history []convention.ChatMessage,
) ([]convention.ChatMessage, int) {
	options = options.normalized()
	current := append([]convention.ChatMessage(nil), history...)
	dropped := 0

	for len(current) > 0 {
		probe := clonePromptContext(ctx)
		probe.History = current
		estimated, err := estimatePromptContextTokens(promptService, probe, options.Estimator)
		if err != nil || estimated <= options.MaxPromptTokens {
			return current, dropped
		}

		pinned, trimmable := splitPinnedConversationHistory(current)
		if len(trimmable) == 0 {
			return pinned, dropped
		}
		trimmable = trimmable[1:]
		dropped++
		if len(pinned) == 0 {
			current = trimmable
			continue
		}
		current = append(append([]convention.ChatMessage(nil), pinned...), trimmable...)
	}

	return current, dropped
}

func trimHistoryForTokenBudget(
	history []convention.ChatMessage,
	tokenBudget int,
	estimator TokenEstimator,
) ([]convention.ChatMessage, int) {
	if len(history) == 0 || tokenBudget <= 0 {
		return nil, len(history)
	}
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}

	pinned, trimmable := splitPinnedConversationHistory(history)
	dropped := 0
	for len(trimmable) > 0 && estimateHistoryTokens(pinned, trimmable, estimator) > tokenBudget {
		trimmable = trimmable[1:]
		dropped++
	}

	if len(pinned) == 0 {
		return append([]convention.ChatMessage(nil), trimmable...), dropped
	}
	result := make([]convention.ChatMessage, 0, len(pinned)+len(trimmable))
	result = append(result, pinned...)
	result = append(result, trimmable...)
	return result, dropped
}

func shrinkContextText(text string, estimator TokenEstimator) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}

	originalTokens := estimator.EstimateTokens(text)
	if originalTokens <= 1 {
		return "", true
	}

	base := strings.TrimSuffix(text, "\n...[truncated]")
	baseRunes := []rune(strings.TrimSpace(base))
	for len(baseRunes) > 0 {
		nextLen := len(baseRunes) * 3 / 4
		if nextLen <= 0 {
			return "", true
		}
		candidate := strings.TrimSpace(string(baseRunes[:nextLen]))
		if candidate == "" {
			return "", true
		}
		withMarker := candidate + "\n...[truncated]"
		if estimator.EstimateTokens(withMarker) < originalTokens {
			return withMarker, true
		}
		baseRunes = baseRunes[:nextLen]
	}
	return "", true
}

func splitPinnedConversationHistory(history []convention.ChatMessage) (pinned, trimmable []convention.ChatMessage) {
	if len(history) == 0 {
		return nil, nil
	}
	if isConversationSummaryMessage(history[0]) {
		return append([]convention.ChatMessage(nil), history[0]), append([]convention.ChatMessage(nil), history[1:]...)
	}
	return nil, append([]convention.ChatMessage(nil), history...)
}

func isConversationSummaryMessage(message convention.ChatMessage) bool {
	return strings.HasPrefix(strings.TrimSpace(message.Content), "对话摘要：")
}

func estimateHistoryTokens(
	pinned []convention.ChatMessage,
	trimmable []convention.ChatMessage,
	estimator TokenEstimator,
) int {
	total := 0
	for _, message := range pinned {
		total += estimator.EstimateTokens(message.Content)
	}
	for _, message := range trimmable {
		total += estimator.EstimateTokens(message.Content)
	}
	return total
}

func estimateChatMessagesTokens(messages []convention.ChatMessage, estimator TokenEstimator) int {
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}
	total := 0
	for _, message := range messages {
		total += estimator.EstimateTokens(message.Content)
	}
	return total
}

func chatContextBudgetTraceExtra(result ChatContextBudgetResult) map[string]any {
	if result.HistoryMessageCountBefore == 0 && !result.Trimmed && result.EstimatedPromptTokens == 0 {
		return nil
	}
	extra := map[string]any{
		"estimatedPromptTokens": result.EstimatedPromptTokens,
		"historyMessageCount":   result.HistoryMessageCountAfter,
	}
	if result.Trimmed {
		extra["historyTrimmed"] = true
		extra["droppedHistoryMessages"] = result.DroppedHistoryMessages
		extra["historyMessageCountBefore"] = result.HistoryMessageCountBefore
	}
	if result.Degraded {
		extra["contextDegraded"] = true
		extra["degradationSteps"] = append([]string(nil), result.DegradationSteps...)
	}
	return extra
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
