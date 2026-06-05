package agent

import (
	"time"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agenthandoff "local/rag-project/internal/app/agent/handoff"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

type Request struct {
	Question  string
	UserID    string
	TraceID   string
	Options   RequestOptions
	ToolStage *ToolStageContext
}

type RequestOptions struct {
	MaxIterations   int
	RequireApproval bool
	OutputMode      string
}

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

type Response struct {
	Query         string
	Results       []agentsearch.SearchResultItem
	Pages         []agentfetch.PageResult
	CombinedText  string
	Summary       string
	Provider      string
	Degraded      bool
	DegradeReason string
}

const (
	RunStatusCompleted        = "completed"
	RunStatusDegraded         = "degraded"
	RunStatusAwaitingApproval = "awaiting_approval"

	ApprovalDecisionApproved = "approved"
	ApprovalDecisionRejected = "rejected"
)

type ApprovalPending struct {
	Required              bool
	Status                string
	Reason                string
	ReasonCode            string
	ReasonMessage         string
	Trigger               string
	Node                  string
	RerunNode             string
	Capability            string
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
	ResumeCount           int
	Question              string
	SearchQuery           string
	CurrentStepID         string
	CurrentStepTitle      string
	CandidateURLs         []string
	CanApprove            bool
	CanReject             bool
	RejectOutcome         string
}

type RunOutcome struct {
	Status          string
	Interrupted     bool
	InterruptReason string
	CheckpointID    string
	Approval        *ApprovalPending
}

type RunResponse struct {
	Response Response
	Outcome  RunOutcome
	Journal  []agentstate.RuntimeEvent
}

type HandoffRunResponse struct {
	Handoff HandoffResult
	Outcome RunOutcome
}

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
