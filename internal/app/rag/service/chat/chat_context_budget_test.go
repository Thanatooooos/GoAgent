package chat

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	ragprompt "local/rag-project/internal/app/rag/core/prompt"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

type fixedTokenEstimator struct {
	factor int
}

func (f fixedTokenEstimator) EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if f.factor <= 0 {
		return len([]rune(text))
	}
	return len([]rune(text)) / f.factor
}

func TestApplyChatContextBudgetDisabledPreservesHistory(t *testing.T) {
	t.Parallel()
	history := []convention.ChatMessage{
		convention.UserMessage("older question"),
		convention.AssistantMessage("older answer"),
	}
	result, err := applyChatContextBudget(
		ChatContextBudgetOptions{Enabled: false, Estimator: fixedTokenEstimator{factor: 1}},
		ragprompt.NewService(nil),
		ragprompt.Context{
			Question: "latest question",
			History:  history,
		},
	)
	if err != nil {
		t.Fatalf("applyChatContextBudget() error = %v", err)
	}
	if len(result.History) != 2 {
		t.Fatalf("expected history preserved, got %d messages", len(result.History))
	}
	if result.Trimmed {
		t.Fatal("expected no trimming when budget disabled")
	}
	if result.EstimatedPromptTokens <= 0 {
		t.Fatalf("expected estimated prompt tokens, got %d", result.EstimatedPromptTokens)
	}
}

func TestTrimHistoryForTokenBudgetDropsOldestMessages(t *testing.T) {
	t.Parallel()
	history := []convention.ChatMessage{
		convention.UserMessage(strings.Repeat("a", 40)),
		convention.AssistantMessage(strings.Repeat("b", 40)),
		convention.UserMessage(strings.Repeat("c", 20)),
	}
	trimmed, dropped := trimHistoryForTokenBudget(history, 50, fixedTokenEstimator{factor: 1})
	if dropped != 2 {
		t.Fatalf("expected two dropped messages, got %d", dropped)
	}
	if len(trimmed) != 1 {
		t.Fatalf("expected one remaining message, got %d", len(trimmed))
	}
	if trimmed[0].Content != strings.Repeat("c", 20) {
		t.Fatalf("expected oldest messages dropped, got %#v", trimmed[0].Content)
	}
}

func TestTrimHistoryForTokenBudgetKeepsConversationSummary(t *testing.T) {
	t.Parallel()
	summary := convention.SystemMessage("对话摘要：earlier discussion about ingestion failures")
	history := []convention.ChatMessage{
		summary,
		convention.UserMessage(strings.Repeat("a", 40)),
		convention.AssistantMessage(strings.Repeat("b", 40)),
	}
	trimmed, dropped := trimHistoryForTokenBudget(history, 30, fixedTokenEstimator{factor: 1})
	if dropped != 2 {
		t.Fatalf("expected two dropped messages, got %d", dropped)
	}
	if len(trimmed) != 1 {
		t.Fatalf("expected summary only, got %d messages", len(trimmed))
	}
	if trimmed[0].Content != summary.Content {
		t.Fatal("expected conversation summary to remain pinned")
	}
}

func TestApplyChatContextBudgetTrimsBeforePromptBuild(t *testing.T) {
	t.Parallel()
	history := []convention.ChatMessage{
		convention.UserMessage(strings.Repeat("old", 200)),
		convention.AssistantMessage(strings.Repeat("old-reply", 200)),
		convention.UserMessage(strings.Repeat("recent", 40)),
	}
	result, err := applyChatContextBudget(
		ChatContextBudgetOptions{
			Enabled:         true,
			MaxPromptTokens: 120,
			Estimator:       fixedTokenEstimator{factor: 1},
		},
		ragprompt.NewService(nil),
		ragprompt.Context{
			Question:         "current",
			KnowledgeContext: strings.Repeat("k", 20),
			SystemPrompt:     "short system prompt",
			History:          history,
		},
	)
	if err != nil {
		t.Fatalf("applyChatContextBudget() error = %v", err)
	}
	if !result.Trimmed || result.DroppedHistoryMessages == 0 {
		t.Fatalf("expected history trimming, got %+v", result)
	}
	if result.EstimatedPromptTokens > 120 {
		t.Fatalf("expected estimated prompt tokens within budget, got %d", result.EstimatedPromptTokens)
	}
	if len(result.History) == len(history) {
		t.Fatal("expected some history to be trimmed")
	}
}

func TestRunPromptStageRecordsChatContextBudgetTrace(t *testing.T) {
	repo := &traceNodeRepoRecorder{}
	tracer := NewChatTracer(nil, repo)
	service := mustNewTestRagChatService(t, minimalRagChatDeps(), RagChatOptions{
		ChatContextBudget: ChatContextBudgetOptions{
			Enabled:         true,
			MaxPromptTokens: 80,
			Estimator:       fixedTokenEstimator{factor: 1},
		},
	})
	service.tracer = tracer

	_, err := service.runPromptStage(
		context.Background(),
		"current question",
		[]convention.ChatMessage{
			convention.UserMessage(strings.Repeat("old", 80)),
			convention.AssistantMessage(strings.Repeat("old-reply", 80)),
		},
		"",
		"",
		ragretrieve.Result{KnowledgeContext: strings.Repeat("k", 20)},
		"",
		"",
		"",
		"",
		"trace-budget-1",
	)
	if err != nil {
		t.Fatalf("runPromptStage() error = %v", err)
	}
	if len(repo.created) != 1 {
		t.Fatalf("expected one prompt trace node, got %d", len(repo.created))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(repo.created[0].ExtraData), &payload); err != nil {
		t.Fatalf("decode trace payload: %v", err)
	}
	if payload["estimatedPromptTokens"] == nil {
		t.Fatalf("expected estimatedPromptTokens in trace, got %+v", payload)
	}
	if payload["historyTrimmed"] != true {
		t.Fatalf("expected historyTrimmed=true, got %+v", payload)
	}
}

func TestChatContextBudgetTraceExtraIncludesTrimMetadata(t *testing.T) {
	t.Parallel()
	extra := chatContextBudgetTraceExtra(ChatContextBudgetResult{
		EstimatedPromptTokens:     900,
		HistoryMessageCountBefore: 5,
		HistoryMessageCountAfter:  2,
		DroppedHistoryMessages:    3,
		Trimmed:                   true,
		Degraded:                  true,
		DegradationSteps:          []string{degradeStepTrimHistory},
	})
	if extra["estimatedPromptTokens"] != 900 {
		t.Fatalf("unexpected trace extra: %+v", extra)
	}
	if extra["historyTrimmed"] != true || extra["droppedHistoryMessages"] != 3 {
		t.Fatalf("expected trim metadata, got %+v", extra)
	}
	if extra["contextDegraded"] != true {
		t.Fatalf("expected contextDegraded=true, got %+v", extra)
	}
}

func TestApplyChatContextBudgetDegradesKnowledgeContextWhenHistoryTrimInsufficient(t *testing.T) {
	t.Parallel()
	history := []convention.ChatMessage{
		convention.UserMessage(strings.Repeat("recent", 20)),
	}
	result, err := applyChatContextBudget(
		ChatContextBudgetOptions{
			Enabled:         true,
			MaxPromptTokens: 80,
			Estimator:       fixedTokenEstimator{factor: 1},
		},
		ragprompt.NewService(nil),
		ragprompt.Context{
			Question:         "current",
			KnowledgeContext: strings.Repeat("knowledge", 80),
			SystemPrompt:     "short system prompt",
			History:          history,
		},
	)
	if err != nil {
		t.Fatalf("applyChatContextBudget() error = %v", err)
	}
	if !result.Degraded {
		t.Fatalf("expected degraded result, got %+v", result)
	}
	if !containsString(result.DegradationSteps, degradeStepTrimKnowledgeContext) {
		t.Fatalf("expected knowledge context degradation, got %+v", result.DegradationSteps)
	}
	if strings.Contains(result.KnowledgeContext, strings.Repeat("knowledge", 80)) {
		t.Fatal("expected knowledge context to be truncated")
	}
	if result.EstimatedPromptTokens > 80 {
		t.Fatalf("expected estimated prompt tokens within budget, got %d", result.EstimatedPromptTokens)
	}
}

func TestShrinkContextTextKeepsPrefix(t *testing.T) {
	t.Parallel()
	trimmed, changed := shrinkContextText(strings.Repeat("x", 200), fixedTokenEstimator{factor: 1})
	if !changed {
		t.Fatal("expected context text to shrink")
	}
	if !strings.HasPrefix(trimmed, "x") {
		t.Fatalf("expected prefix preserved, got %q", trimmed)
	}
	if !strings.Contains(trimmed, "...[truncated]") {
		t.Fatalf("expected truncation marker, got %q", trimmed)
	}
}
