package tool

import (
	"context"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

const (
	CallStatusSuccess = ragcore.CallStatusSuccess
	CallStatusFailed  = ragcore.CallStatusFailed
	CallStatusSkipped = ragcore.CallStatusSkipped
)

type Workflow = ragcore.Workflow
type Planner = ragcore.Planner
type HintCall = ragcore.HintCall
type AgentState = ragcore.AgentState
type PlanInput = ragcore.PlanInput
type PlanResult = ragcore.PlanResult
type WorkflowInput = ragcore.WorkflowInput
type CallSummary = ragcore.CallSummary
type RoundSummary = ragcore.RoundSummary
type WorkflowResult = ragcore.WorkflowResult
type ToolCallEvent = ragcore.ToolCallEvent
type WorkflowEventSink = ragcore.WorkflowEventSink

// Ensure context import stays available.
var _ context.Context
