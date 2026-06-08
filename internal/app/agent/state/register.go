package state

import "github.com/cloudwego/eino/schema"

func init() {
	schema.RegisterName[StateSnapshot]("agent_state_StateSnapshot")
	schema.RegisterName[RequestState]("agent_state_RequestState")
	schema.RegisterName[RuntimeOptions]("agent_state_RuntimeOptions")
	schema.RegisterName[ContextState]("agent_state_ContextState")
	schema.RegisterName[SearchResultRef]("agent_state_SearchResultRef")
	schema.RegisterName[FetchResultRef]("agent_state_FetchResultRef")
	schema.RegisterName[MemoryRef]("agent_state_MemoryRef")
	schema.RegisterName[PlanState]("agent_state_PlanState")
	schema.RegisterName[PlanStep]("agent_state_PlanStep")
	schema.RegisterName[PlanStepResult]("agent_state_PlanStepResult")
	schema.RegisterName[PlanStepArtifact]("agent_state_PlanStepArtifact")
	schema.RegisterName[EvidenceState]("agent_state_EvidenceState")
	schema.RegisterName[ApprovalState]("agent_state_ApprovalState")
	schema.RegisterName[EvidenceItem]("agent_state_EvidenceItem")
	schema.RegisterName[ExecutionState]("agent_state_ExecutionState")
	schema.RegisterName[AnswerState]("agent_state_AnswerState")
	schema.RegisterName[RuntimeEvent]("agent_state_RuntimeEvent")
	schema.RegisterName[EvidenceRef]("agent_state_EvidenceRef")
	schema.RegisterName[DecisionRef]("agent_state_DecisionRef")
	schema.RegisterName[CheckpointRef]("agent_state_CheckpointRef")
	schema.RegisterName[StateDelta]("agent_state_StateDelta")
	schema.RegisterName[RequestDelta]("agent_state_RequestDelta")
	schema.RegisterName[ContextDelta]("agent_state_ContextDelta")
	schema.RegisterName[PlanDelta]("agent_state_PlanDelta")
	schema.RegisterName[EvidenceDelta]("agent_state_EvidenceDelta")
	schema.RegisterName[ApprovalDelta]("agent_state_ApprovalDelta")
	schema.RegisterName[ExecutionDelta]("agent_state_ExecutionDelta")
	schema.RegisterName[AnswerDelta]("agent_state_AnswerDelta")
}
