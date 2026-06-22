package chat

import (
	"strconv"
	"strings"

	agentapp "local/rag-project/internal/app/agent"
	agentstate "local/rag-project/internal/app/agent/state"
	ragtool "local/rag-project/internal/app/rag/tool/core"
)

type projectedAgentToolEvents struct {
	calls  []ragtool.CallSummary
	rounds []ragtool.RoundSummary
}

func projectAgentToolEvents(result agentapp.RunResponse) projectedAgentToolEvents {
	if len(result.Journal) == 0 {
		return projectedAgentToolEvents{}
	}

	calls := make([]ragtool.CallSummary, 0, 4)
	activeStarts := make(map[string]agentstate.RuntimeEvent, 4)
	sequence := 0

	for _, event := range result.Journal {
		switch strings.TrimSpace(event.EventType) {
		case agentstate.EventTypeCapabilityStart:
			activeStarts[strings.TrimSpace(event.Node)] = event
		case agentstate.EventTypeCapabilityResult, agentstate.EventTypeCapabilitySkipped:
			node := strings.TrimSpace(event.Node)
			start, ok := activeStarts[node]
			if !ok {
				start = event
			}
			delete(activeStarts, node)

			displayName, ok := agentToolDisplayName(node)
			if !ok {
				continue
			}
			sequence++
			call := ragtool.CallSummary{
				CallID:     agentToolCallID(node, sequence, event.Sequence),
				Round:      1,
				Sequence:   sequence,
				Name:       displayName,
				Status:     agentToolEventStatus(event),
				Summary:    firstNonEmptyString(strings.TrimSpace(event.PayloadText), strings.TrimSpace(start.PayloadText), agentToolDefaultSummary(node, event.EventType)),
				DurationMs: maxInt64(0, event.Timestamp.Sub(start.Timestamp).Milliseconds()),
			}
			calls = append(calls, call)
		}
	}

	if len(calls) == 0 {
		return projectedAgentToolEvents{}
	}

	round := ragtool.RoundSummary{
		Round:             1,
		Calls:             append([]ragtool.CallSummary(nil), calls...),
		ToolCallCount:     len(calls),
		NextHintCallCount: 0,
	}
	return projectedAgentToolEvents{
		calls:  calls,
		rounds: []ragtool.RoundSummary{round},
	}
}

func emitProjectedAgentToolEvents(sink RagChatEventSink, result agentapp.RunResponse) {
	if sink == nil {
		return
	}
	projected := projectAgentToolEvents(result)
	for _, call := range projected.calls {
		event := ragtool.ToolCallEvent{
			CallID:     call.CallID,
			Round:      call.Round,
			Sequence:   call.Sequence,
			Name:       call.Name,
			Status:     "running",
			Summary:    agentToolRunningSummary(call.Name),
			Arguments:  nil,
			Data:       nil,
			DurationMs: 0,
		}
		_ = sink.SendToolStart(event)
		_ = sink.SendToolResult(ragtool.ToolCallEvent{
			CallID:     call.CallID,
			Round:      call.Round,
			Sequence:   call.Sequence,
			Name:       call.Name,
			Status:     call.Status,
			Summary:    call.Summary,
			DurationMs: call.DurationMs,
		})
		_ = sink.SendTool(call.Name, call.Status, call.Summary)
	}
}

func agentToolDisplayName(node string) (string, bool) {
	switch strings.TrimSpace(node) {
	case "search":
		return "查询中", true
	case "fetch":
		return "拉取中", true
	case "external_evidence":
		return "资料整理中", true
	case "execute_step":
		return "执行中", true
	default:
		return "", false
	}
}

func agentToolDefaultSummary(node string, eventType string) string {
	switch strings.TrimSpace(node) {
	case "search":
		return "正在查询外部资料"
	case "fetch":
		if strings.TrimSpace(eventType) == agentstate.EventTypeCapabilitySkipped {
			return "未执行页面拉取"
		}
		return "已完成页面拉取"
	case "external_evidence":
		return "已完成外部资料整理"
	case "execute_step":
		return "已完成执行步骤"
	default:
		return ""
	}
}

func agentToolRunningSummary(name string) string {
	switch strings.TrimSpace(name) {
	case "查询中":
		return "正在查询外部资料"
	case "拉取中":
		return "正在拉取页面内容"
	case "资料整理中":
		return "正在整理外部资料"
	case "执行中":
		return "正在执行步骤"
	default:
		return "正在处理"
	}
}

func agentToolEventStatus(event agentstate.RuntimeEvent) string {
	if strings.TrimSpace(event.EventType) == agentstate.EventTypeCapabilitySkipped {
		return ragtool.CallStatusSkipped
	}
	return ragtool.CallStatusSuccess
}

func agentToolCallID(node string, sequence int, journalSequence int) string {
	base := strings.TrimSpace(node)
	if base == "" {
		base = "agent_tool"
	}
	if journalSequence > 0 {
		return base + "_" + strconv.Itoa(journalSequence)
	}
	return base + "_" + strconv.Itoa(sequence)
}
