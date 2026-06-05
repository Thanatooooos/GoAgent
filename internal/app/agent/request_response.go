package agent

import (
	"time"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

// Request is the public service request contract for one agent run.
type Request struct {
	Question  string
	UserID    string
	TraceID   string
	Options   RequestOptions
	ToolStage *ToolStageContext
}

// RequestOptions carries caller-controlled runtime behavior.
type RequestOptions struct {
	MaxIterations   int
	RequireApproval bool
	OutputMode      string
}

// ToolStageContext seeds the runtime with upstream RAG/tool-stage context.
//
// This context is optional. When present, it should be treated as caller-owned
// input projection rather than mutable runtime state.
type ToolStageContext struct {
	ConversationID    string
	KnowledgeBaseIDs  []string
	RewrittenQuestion string
	SubQuestions      []string
	NeedRetrieval     bool
	KnowledgeContext  string
	SearchChannels    []string
	HistorySummary    string
	SessionContext    string
	MemoryContext     string
}

// Response is the public final-answer projection for a completed or degraded run.
type Response struct {
	// Query is the effective query the runtime used for the current run.
	Query string
	// Results is the outward projection of collected search results.
	Results []agentsearch.SearchResultItem
	// Pages is the outward projection of fetched page content.
	Pages []agentfetch.PageResult
	// CombinedText concatenates fetched page text for callers that prefer a
	// single textual evidence blob over page-by-page traversal.
	CombinedText string
	// Summary is the best available outward summary. It may come from the final
	// answer, a final note, or a degrade reason path.
	Summary string
	// Provider reports the effective search provider when search was used.
	Provider string
	// Degraded reports whether the run ended through a degrade path.
	Degraded bool
	// DegradeReason is only meaningful when Degraded is true.
	DegradeReason string
}

const (
	// RunStatusCompleted means the runtime finished normally without approval
	// pending and without a degrade reason.
	RunStatusCompleted = "completed"
	// RunStatusDegraded means the runtime terminated through a degrade path.
	RunStatusDegraded = "degraded"
	// RunStatusAwaitingApproval means execution paused and requires an explicit
	// caller approval decision before it can continue.
	RunStatusAwaitingApproval = "awaiting_approval"

	// ApprovalDecisionApproved is the canonical outward decision value for
	// resuming an approval-pending run.
	ApprovalDecisionApproved = "approved"
	// ApprovalDecisionRejected is the canonical outward decision value for
	// rejecting an approval-pending run.
	ApprovalDecisionRejected = "rejected"
)

// ApprovalPending is the public approval-facing projection for a run paused at
// an approval boundary.
//
// Compatibility fields:
// - Reason is kept as a compatibility alias of ReasonCode.
// - Capability is kept as a compatibility alias of CapabilityName.
type ApprovalPending struct {
	// Required is always true when ApprovalPending is present.
	Required bool
	// Status is the outward approval lifecycle status, typically pending.
	Status string
	// Reason is a compatibility alias of ReasonCode.
	Reason string
	// ReasonCode is the canonical machine-readable approval reason.
	ReasonCode string
	// ReasonMessage is the human-readable explanation for the approval request.
	ReasonMessage string
	// Trigger describes what surfaced the approval boundary.
	Trigger string
	// Node is the runtime node currently blocked on approval.
	Node string
	// RerunNode is the node expected to rerun after approval resume.
	RerunNode string
	// Capability is a compatibility alias of CapabilityName.
	Capability string
	// CapabilityName is the canonical runtime capability identity.
	CapabilityName        string
	CapabilityKind        string
	CapabilityFamily      string
	CapabilityDescription string
	RiskLevel             string
	SupportsResume        bool
	Idempotency           string
	CheckpointID          string
	SessionID             string
	RequestedAt           time.Time
	// ResumeCount reports how many successful resume attempts have already been
	// recorded on this runtime session.
	ResumeCount      int
	Question         string
	SearchQuery      string
	CurrentStepID    string
	CurrentStepTitle string
	CandidateURLs    []string
	CanApprove       bool
	CanReject        bool
	RejectOutcome    string
}

// RunOutcome is the public terminal-or-paused status projection of a run.
//
// Contract:
// - completed: CheckpointID empty, Approval nil
// - degraded: CheckpointID empty, Approval nil
// - awaiting_approval: CheckpointID non-empty, Approval non-nil
type RunOutcome struct {
	Status string
	// Interrupted mirrors runtime interruption state. It may be true only when
	// execution paused before completion, most notably while awaiting approval.
	Interrupted     bool
	InterruptReason string
	CheckpointID    string
	Approval        *ApprovalPending
}

// RunResponse is the detailed final-answer service response.
type RunResponse struct {
	Response Response
	Outcome  RunOutcome
	Journal  []agentstate.RuntimeEvent
}

// HandoffRunResponse is the detailed handoff-mode service response.
type HandoffRunResponse struct {
	Handoff HandoffResult
	Outcome RunOutcome
}

// ResumeApprovalRequest is the public resume contract for an approval-pending run.
//
// Decision is the canonical outward field. Approved is a compatibility fallback
// accepted only when Decision is empty.
type ResumeApprovalRequest struct {
	CheckpointID string
	Decision     string
	Approved     bool
	DecisionNote string
}

type HandoffResult = agenthandoff.Result
type HandoffEvidenceBundle = agenthandoff.EvidenceBundle
type AcceptedEvidenceItem = agenthandoff.AcceptedEvidenceItem
type HandoffDecisionSummary = agenthandoff.DecisionSummary
