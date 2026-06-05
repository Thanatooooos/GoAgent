package capability

import (
	"context"

	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	StatusSucceeded = "succeeded"
	StatusDegraded  = "degraded"
	StatusSkipped   = "skipped"

	ErrorClassValidation = "validation_error"
	ErrorClassDependency = "dependency_error"
	ErrorClassExternal   = "external_error"
	ErrorClassPermission = "permission_error"
)

// InvocationRequest is the generic runtime-facing input contract for a capability invocation.
type InvocationRequest struct {
	SessionID string                   `json:"session_id,omitempty"`
	Input     any                      `json:"input,omitempty"`
	Snapshot  agentstate.StateSnapshot `json:"snapshot"`
	Metadata  map[string]any           `json:"metadata,omitempty"`
}

// ActionRecord summarizes what the capability intended to do.
type ActionRecord struct {
	Name    string `json:"name,omitempty"`
	Summary string `json:"summary,omitempty"`
}

// ObservationRecord summarizes what the capability observed after invocation.
type ObservationRecord struct {
	Summary    string `json:"summary,omitempty"`
	Degraded   bool   `json:"degraded,omitempty"`
	ErrorClass string `json:"error_class,omitempty"`
}

// InvocationResult is the generic capability return contract consumed by runtime patterns.
type InvocationResult struct {
	Output       any                      `json:"output,omitempty"`
	Action       ActionRecord             `json:"action,omitempty"`
	Observation  ObservationRecord        `json:"observation,omitempty"`
	Delta        agentstate.StateDelta    `json:"delta"`
	EvidenceRefs []agentstate.EvidenceRef `json:"evidence_refs,omitempty"`
	Status       string                   `json:"status,omitempty"`
	ErrorClass   string                   `json:"error_class,omitempty"`
}

// Handle is the common registry-facing capability contract.
type Handle interface {
	Spec() Spec
	Invoke(ctx context.Context, req InvocationRequest) (InvocationResult, error)
}
