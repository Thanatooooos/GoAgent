package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	ragtool "local/rag-project/internal/app/rag/tool/core"
)

type ChatTracer struct {
	traceRunRepo  port.RagTraceRunRepository
	traceNodeRepo port.RagTraceNodeRepository
	now           func() time.Time
}

func NewChatTracer(
	traceRunRepo port.RagTraceRunRepository,
	traceNodeRepo port.RagTraceNodeRepository,
) *ChatTracer {
	return &ChatTracer{
		traceRunRepo:  traceRunRepo,
		traceNodeRepo: traceNodeRepo,
		now:           time.Now,
	}
}

func (t *ChatTracer) startTraceRun(ctx context.Context, traceID, conversationID, taskID, userID string) error {
	return t.startTraceRunAt(ctx, traceID, conversationID, taskID, userID, t.now())
}

func (t *ChatTracer) startTraceRunAt(ctx context.Context, traceID, conversationID, taskID, userID string, startedAt time.Time) error {
	if t.traceRunRepo == nil {
		return nil
	}
	_, err := t.traceRunRepo.Create(ctx, domain.RagTraceRun{
		ID:             traceID,
		TraceID:        traceID,
		TraceName:      "rag_chat",
		EntryMethod:    "rag.v3.chat",
		ConversationID: conversationID,
		TaskID:         taskID,
		UserID:         userID,
		Status:         ragTraceStatusRunning,
		StartTime:      timePointerValue(startedAt),
		CreateTime:     startedAt,
		UpdateTime:     startedAt,
	})
	return err
}

func (t *ChatTracer) finishTraceRun(ctx context.Context, traceID string, status string, cause error) {
	if t.traceRunRepo == nil || strings.TrimSpace(traceID) == "" {
		return
	}
	now := t.now()
	duration := int64(0)
	run, err := t.traceRunRepo.GetByTraceID(ctx, traceID)
	if err == nil && run.StartTime != nil {
		duration = now.Sub(*run.StartTime).Milliseconds()
	}
	errorMessage := ""
	if cause != nil {
		errorMessage = cause.Error()
	}
	_ = t.traceRunRepo.UpdateByTraceID(ctx, traceID, domain.RagTraceRun{
		TraceID:      traceID,
		Status:       status,
		ErrorMessage: errorMessage,
		EndTime:      &now,
		DurationMs:   &duration,
		UpdateTime:   now,
	})
}

func (t *ChatTracer) appendTraceRunExtra(ctx context.Context, traceID string, patch map[string]any) {
	if t == nil || t.traceRunRepo == nil || strings.TrimSpace(traceID) == "" || len(patch) == 0 {
		return
	}
	run, err := t.traceRunRepo.GetByTraceID(ctx, traceID)
	if err != nil || strings.TrimSpace(run.ID) == "" {
		return
	}

	payload := map[string]any{}
	if strings.TrimSpace(run.ExtraData) != "" {
		_ = json.Unmarshal([]byte(run.ExtraData), &payload)
	}
	for key, value := range patch {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return
	}
	run.ExtraData = string(raw)
	run.UpdateTime = t.now()
	_ = t.traceRunRepo.UpdateByTraceID(ctx, traceID, run)
}

func (t *ChatTracer) recordTraceNode(ctx context.Context, traceID string, node ragChatTraceNode, status string, extra map[string]any) error {
	now := t.now()
	return t.recordTraceNodeAt(ctx, traceID, node, status, now, now, extra)
}

func (t *ChatTracer) recordTraceNodeAt(ctx context.Context, traceID string, node ragChatTraceNode, status string, startedAt time.Time, endedAt time.Time, extra map[string]any) error {
	if t.traceNodeRepo == nil || strings.TrimSpace(traceID) == "" {
		return nil
	}
	nodeRecordID, err := nextRagTraceNodeID()
	if err != nil {
		return err
	}
	duration := endedAt.Sub(startedAt).Milliseconds()
	if duration < 0 {
		duration = 0
	}
	extraData := ""
	errorMessage := ""
	if value, ok := extra["error"]; ok && value != nil {
		errorMessage = strings.TrimSpace(fmt.Sprintf("%v", value))
	}
	if len(extra) > 0 {
		raw, err := json.Marshal(extra)
		if err == nil {
			extraData = string(raw)
		}
	}
	_, err = t.traceNodeRepo.Create(ctx, domain.RagTraceNode{
		ID:           nodeRecordID,
		TraceID:      traceID,
		NodeID:       node.NodeID,
		ParentNodeID: strings.TrimSpace(node.ParentNodeID),
		Depth:        maxInt(node.Depth, 1),
		NodeType:     node.NodeType,
		NodeName:     node.NodeName,
		Status:       status,
		ErrorMessage: errorMessage,
		StartTime:    &startedAt,
		EndTime:      &endedAt,
		DurationMs:   &duration,
		ExtraData:    extraData,
		CreateTime:   endedAt,
		UpdateTime:   endedAt,
	})
	return err
}

func (t *ChatTracer) recordToolCallTraceNodes(ctx context.Context, traceID string, calls []ragtool.CallSummary) {
	if t == nil || t.traceNodeRepo == nil || strings.TrimSpace(traceID) == "" || len(calls) == 0 {
		return
	}
	baseTime := t.now()
	offsetMs := int64(0)
	for idx, call := range calls {
		durationMs := call.DurationMs
		if durationMs < 0 {
			durationMs = 0
		}
		startedAt := baseTime.Add(time.Duration(offsetMs) * time.Millisecond)
		endedAt := startedAt.Add(time.Duration(durationMs) * time.Millisecond)
		offsetMs += maxInt64(durationMs, 1)
		extra := map[string]any{
			"toolName":   strings.TrimSpace(call.Name),
			"summary":    strings.TrimSpace(call.Summary),
			"durationMs": durationMs,
			"parentNode": "tool_workflow",
			"sequence":   idx + 1,
			"toolStatus": strings.TrimSpace(call.Status),
		}
		if strings.TrimSpace(call.Status) == ragtool.CallStatusFailed {
			extra["error"] = strings.TrimSpace(call.Summary)
		}
		_ = t.recordTraceNodeAt(ctx, traceID, ragChatTraceNode{
			NodeID:       fmt.Sprintf("tool_%02d", idx+1),
			ParentNodeID: "tool_workflow",
			Depth:        2,
			NodeType:     "tool_call",
			NodeName:     strings.TrimSpace(call.Name),
		}, strings.TrimSpace(call.Status), startedAt, endedAt, extra)
	}
}

func (t *ChatTracer) recordAgentWorkflowTraceNodes(ctx context.Context, traceID string, result ragtool.WorkflowResult) {
	if t == nil || t.traceNodeRepo == nil || strings.TrimSpace(traceID) == "" {
		return
	}
	if len(result.Rounds) == 0 {
		t.recordToolCallTraceNodes(ctx, traceID, result.Calls)
		return
	}

	baseTime := t.now()
	offsetMs := int64(0)
	for _, round := range result.Rounds {
		roundNodeID := fmt.Sprintf("agt_round_%02d", round.Round)
		roundDuration := int64(0)
		for _, call := range round.Calls {
			roundDuration += maxInt64(call.DurationMs, 1)
		}
		startedAt := baseTime.Add(time.Duration(offsetMs) * time.Millisecond)
		endedAt := startedAt.Add(time.Duration(roundDuration) * time.Millisecond)
		offsetMs += maxInt64(roundDuration, 1)
		_ = t.recordTraceNodeAt(ctx, traceID, ragChatTraceNode{
			NodeID:       roundNodeID,
			ParentNodeID: "tool_workflow",
			Depth:        2,
			NodeType:     "agt_round",
			NodeName:     fmt.Sprintf("agent_round_%d", round.Round),
		}, ragTraceStatusSuccess, startedAt, endedAt, map[string]any{
			"round":               round.Round,
			"done":                round.Done,
			"nextHint":            strings.TrimSpace(round.NextHint),
			"nextHintCalls":       round.NextHintCalls,
			"nextHintCallCount":   round.NextHintCallCount,
			"executionMode":       strings.TrimSpace(round.ExecutionMode),
			"planningSource":      strings.TrimSpace(round.PlanningSource),
			"llmPlannerSkipped":   round.LLMPlannerSkipped,
			"toolCallCount":       round.ToolCallCount,
			"wallClockDurationMs": round.WallClockDurationMs,
			"totalToolDurationMs": round.TotalToolDurationMs,
			"capability":          strings.TrimSpace(result.TraceMeta.Capability),
			"workflowMode":        strings.TrimSpace(result.TraceMeta.ExecutionMode),
			"riskLevel":           strings.TrimSpace(result.TraceMeta.RiskLevel),
			"approvalRequirement": strings.TrimSpace(result.TraceMeta.ApprovalRequirement),
			"evidenceSources":     append([]string(nil), result.TraceMeta.EvidenceSources...),
		})

		callOffset := int64(0)
		for _, call := range round.Calls {
			durationMs := maxInt64(call.DurationMs, 0)
			callStartedAt := startedAt.Add(time.Duration(callOffset) * time.Millisecond)
			callEndedAt := callStartedAt.Add(time.Duration(durationMs) * time.Millisecond)
			callOffset += maxInt64(durationMs, 1)
			extra := map[string]any{
				"callId":     strings.TrimSpace(call.CallID),
				"toolName":   strings.TrimSpace(call.Name),
				"summary":    strings.TrimSpace(call.Summary),
				"durationMs": durationMs,
				"round":      call.Round,
				"sequence":   call.Sequence,
				"toolStatus": strings.TrimSpace(call.Status),
				"arguments":  call.Arguments,
			}
			if len(call.Data) > 0 {
				extra["data"] = call.Data
			}
			if strings.TrimSpace(call.Status) == ragtool.CallStatusFailed {
				extra["error"] = strings.TrimSpace(call.Summary)
			}
			_ = t.recordTraceNodeAt(ctx, traceID, ragChatTraceNode{
				NodeID:       firstNonEmptyString(strings.TrimSpace(call.CallID), fmt.Sprintf("tool_%02d_%02d", round.Round, call.Sequence)),
				ParentNodeID: roundNodeID,
				Depth:        3,
				NodeType:     "tool_call",
				NodeName:     strings.TrimSpace(call.Name),
			}, strings.TrimSpace(call.Status), callStartedAt, callEndedAt, extra)
		}

		observationStatus := ragTraceStatusSuccess
		if round.Done {
			observationStatus = ragTraceStatusSuccess
		}
		observationStartedAt := endedAt
		observationEndedAt := observationStartedAt.Add(time.Millisecond)
		_ = t.recordTraceNodeAt(ctx, traceID, ragChatTraceNode{
			NodeID:       fmt.Sprintf("agt_obs_%02d", round.Round),
			ParentNodeID: roundNodeID,
			Depth:        3,
			NodeType:     "agt_obs",
			NodeName:     "agent_observation",
		}, observationStatus, observationStartedAt, observationEndedAt, map[string]any{
			"round":               round.Round,
			"done":                round.Done,
			"reasoning":           strings.TrimSpace(round.Reasoning),
			"nextHint":            strings.TrimSpace(round.NextHint),
			"nextHintCalls":       round.NextHintCalls,
			"nextHintCallCount":   round.NextHintCallCount,
			"planningSource":      strings.TrimSpace(round.PlanningSource),
			"llmPlannerSkipped":   round.LLMPlannerSkipped,
			"capability":          strings.TrimSpace(result.TraceMeta.Capability),
			"workflowMode":        strings.TrimSpace(result.TraceMeta.ExecutionMode),
			"riskLevel":           strings.TrimSpace(result.TraceMeta.RiskLevel),
			"approvalRequirement": strings.TrimSpace(result.TraceMeta.ApprovalRequirement),
		})
		offsetMs += 1
	}
}

func (t *ChatTracer) recordChatTraceNode(ctx context.Context, traceID string, status string, result ragChatTaskResult) {
	extra := map[string]any{
		"contentLength":  len(strings.TrimSpace(result.content)),
		"thinkingLength": len(strings.TrimSpace(result.thinking)),
	}
	if result.err != nil {
		extra["error"] = result.err.Error()
	}
	_ = t.recordTraceNode(ctx, traceID, ragChatTraceNode{
		NodeID:   "chat",
		NodeType: "chat",
		NodeName: "stream_chat",
	}, status, extra)
}
