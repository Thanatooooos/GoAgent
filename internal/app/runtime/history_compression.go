package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/compression"
	"local/rag-project/internal/app/runtime/persistence"
	fwlog "local/rag-project/internal/framework/log"
)

const summaryMaxChars = 6000

// compactHistory is deliberately best-effort. A failed summary must never
// turn an otherwise valid user request into an error; the original history is
// still sent to the model in that case.
func (r *Runtime) compactHistory(ctx context.Context, session persistence.Session, request RunRequest, policy Policy, system []string, tools []capability.ModelDefinition, entries []JournalEntry, current []ModelMessage) ([]ModelMessage, error) {
	budget := policy.ContextTokenBudget
	if budget <= 0 {
		budget = r.ContextTokenBudget
	}
	historyStore, ok := r.History.(CompactionHistory)
	if budget <= 0 || !ok {
		return current, nil
	}
	estimator := tokenbudget.NewDefaultEstimator()
	if estimateModelRequest(ModelRequest{System: system, Messages: current, Tools: tools}, estimator) <= budget {
		return current, nil
	}

	snapshot, err := historyStore.Snapshot(ctx, request)
	if err != nil {
		fwlog.FromContext(ctx).Warnw("runtime history compression skipped", "reason", "load_history", "error", err)
		return current, nil
	}
	raw := make([]compression.Message, 0, len(snapshot.Messages))
	for _, message := range snapshot.Messages {
		raw = append(raw, compression.Message{Role: string(message.Role), Content: message.Content})
	}
	// The available window must pay for current user input, runtime journal,
	// system sources and tool schemas. Reserve room for the new summary too.
	withoutHistory := ModelRequest{System: system, Tools: tools}
	withoutHistory.Messages = append(withoutHistory.Messages, ModelMessage{Role: ModelRoleUser, Content: request.Question})
	projected, projectErr := ProjectJournalHistory(withoutHistory.Messages, entries)
	if projectErr != nil {
		return current, nil
	}
	withoutHistory.Messages = projected
	available := budget - estimateModelRequest(withoutHistory, estimator) - estimator.EstimateTokens(strings.Repeat("x", summaryMaxChars))
	plan := compression.Split(raw, available, estimator)
	if !plan.NeedsSummary || len(plan.Past) == 0 {
		return current, nil
	}
	if err := r.recordHistoryCompression(ctx, session, EventHistoryCompressionStarted, "running", len(plan.Past)); err != nil {
		return current, err
	}

	turn, err := r.Model.Stream(ctx, ModelRequest{
		System:   []string{structuredSummaryPrompt(snapshot.Summary.StructuredJSON, plan.Past)},
		Messages: []ModelMessage{{Role: ModelRoleUser, Content: structuredSummaryRequest}},
	}, func(ModelEvent) error { return nil })
	if err != nil {
		_ = r.recordHistoryCompression(context.WithoutCancel(ctx), session, EventHistoryCompressionFinished, "failed", len(plan.Past))
		fwlog.FromContext(ctx).Warnw("runtime history compression skipped", "reason", "model", "error", err)
		return current, nil
	}
	structured, err := history.ParseStructuredSummary(strings.TrimSpace(turn.Content))
	if err != nil {
		_ = r.recordHistoryCompression(context.WithoutCancel(ctx), session, EventHistoryCompressionFinished, "failed", len(plan.Past))
		fwlog.FromContext(ctx).Warnw("runtime history compression skipped", "reason", "invalid_summary", "error", err)
		return current, nil
	}
	structured = history.RepairStructuredSummary(structured)
	structuredJSON, err := json.Marshal(structured)
	if err != nil {
		_ = r.recordHistoryCompression(context.WithoutCancel(ctx), session, EventHistoryCompressionFinished, "failed", len(plan.Past))
		return current, nil
	}
	coveredTo := snapshot.Messages[len(plan.Past)-1].ID
	coveredFrom := snapshot.Summary.CoveredFromMessageID
	if coveredFrom == "" {
		coveredFrom = snapshot.Messages[0].ID
	}
	accepted, err := historyStore.StoreSummary(ctx, request, HistorySummary{
		Content:              history.RenderStructuredSummary(structured, summaryMaxChars),
		StructuredJSON:       string(structuredJSON),
		CoveredFromMessageID: coveredFrom,
		CoveredToMessageID:   coveredTo,
		SourceMessageCount:   snapshot.Summary.SourceMessageCount + len(plan.Past),
	})
	if err != nil {
		fwlog.FromContext(ctx).Warnw("runtime history compression skipped", "reason", "persist", "error", err)
		return current, nil
	}
	if !accepted {
		fwlog.FromContext(ctx).Infow("runtime history compression superseded", "coveredToMessageId", coveredTo)
	}
	if err := r.recordHistoryCompression(ctx, session, EventHistoryCompressionFinished, "completed", len(plan.Past)); err != nil {
		return current, err
	}
	fresh, err := r.history(ctx, request)
	if err != nil {
		fwlog.FromContext(ctx).Warnw("runtime history compression skipped", "reason", "reload", "error", err)
		return current, nil
	}
	fresh = append(fresh, ModelMessage{Role: ModelRoleUser, Content: request.Question})
	fresh, err = ProjectJournalHistory(fresh, entries)
	if err != nil {
		return current, nil
	}
	fwlog.FromContext(ctx).Infow("runtime history compressed", "pastMessages", len(plan.Past), "recentMessages", len(plan.Recent), "coveredToMessageId", coveredTo)
	return fresh, nil
}

func (r *Runtime) recordHistoryCompression(ctx context.Context, session persistence.Session, kind, status string, messageCount int) error {
	detail, err := json.Marshal(struct {
		Status       string `json:"status"`
		MessageCount int    `json:"messageCount"`
	}{Status: status, MessageCount: messageCount})
	if err != nil {
		return err
	}
	_, err = r.Lifecycle.RecordEvent(ctx, session, kind, string(detail))
	return err
}

func estimateModelRequest(request ModelRequest, estimator tokenbudget.Estimator) int {
	total := 0
	for _, block := range request.System {
		total += estimator.EstimateTokens(block)
	}
	for _, message := range request.Messages {
		total += estimator.EstimateTokens(message.Content)
		for _, call := range message.ToolCalls {
			total += estimator.EstimateTokens(call.CapabilityID) + estimator.EstimateTokens(string(call.Arguments))
		}
	}
	for _, tool := range request.Tools {
		encoded, _ := json.Marshal(tool)
		total += estimator.EstimateTokens(string(encoded))
	}
	return total
}

func structuredSummaryPrompt(previousJSON string, messages []compression.Message) string {
	var source strings.Builder
	for _, message := range messages {
		fmt.Fprintf(&source, "%s: %s\n", message.Role, strings.TrimSpace(message.Content))
	}
	previousJSON = strings.TrimSpace(previousJSON)
	if previousJSON == "" {
		previousJSON = "{}"
	}
	return structuredSummarySystemPrompt + previousJSON + structuredSummaryRecordsPrefix + source.String()
}
