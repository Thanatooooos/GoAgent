package state

import (
	"fmt"
	"strings"
	"time"
)

// StateSnapshot is the structured view of the current runtime state.
// Nodes read from it; later M1 reducer logic will be the only writer.
type StateSnapshot struct {
	SchemaVersion int            `json:"schema_version,omitempty"`
	Request       RequestState   `json:"request"`
	Context       ContextState   `json:"context"`
	Plan          PlanState      `json:"plan"`
	Evidence      EvidenceState  `json:"evidence"`
	Approval      ApprovalState  `json:"approval"`
	Execution     ExecutionState `json:"execution"`
	Answer        AnswerState    `json:"answer"`
	Pattern       PatternState   `json:"pattern,omitempty"`
}

const CurrentSnapshotVersion = 1
const LegacySnapshotVersion = 0

// PatternState reserves an explicit extension area for pattern-private state so
// shared projections do not need to overload generic runtime domains.
type PatternState struct {
	Name string         `json:"name,omitempty"`
	Data map[string]any `json:"data,omitempty"`
}

type StateOwner string

const (
	StateOwnerRuntime            StateOwner = "runtime"
	StateOwnerPattern            StateOwner = "pattern"
	StateOwnerCapability         StateOwner = "capability"
	StateOwnerAnswerSynthesizer  StateOwner = "answer_synthesizer"
)

type StateDomainContract struct {
	Domain string     `json:"domain"`
	Owner  StateOwner `json:"owner"`
	Shared bool       `json:"shared"`
	Notes  string     `json:"notes,omitempty"`
}

var snapshotDomainContracts = []StateDomainContract{
	{Domain: "request", Owner: StateOwnerRuntime, Shared: true, Notes: "request-scoped runtime input and envelope projection"},
	{Domain: "context", Owner: StateOwnerPattern, Shared: true, Notes: "pattern-curated working context assembled from capability outputs"},
	{Domain: "plan", Owner: StateOwnerPattern, Shared: true, Notes: "explicit multi-step strategy state owned by plan-capable patterns"},
	{Domain: "evidence", Owner: StateOwnerCapability, Shared: true, Notes: "accepted evidence set normalized from capability outputs"},
	{Domain: "approval", Owner: StateOwnerRuntime, Shared: true, Notes: "runtime-owned approval lifecycle and resume metadata"},
	{Domain: "execution", Owner: StateOwnerRuntime, Shared: true, Notes: "runtime-owned lifecycle, progress, and interruption state"},
	{Domain: "answer", Owner: StateOwnerAnswerSynthesizer, Shared: true, Notes: "answer synthesis output contract shared across patterns"},
	{Domain: "pattern", Owner: StateOwnerPattern, Shared: false, Notes: "pattern-private extension area excluded from shared projections"},
}

func SnapshotDomainContracts() []StateDomainContract {
	cloned := make([]StateDomainContract, len(snapshotDomainContracts))
	copy(cloned, snapshotDomainContracts)
	return cloned
}

// RequestState captures stable request-scoped inputs and runtime boundaries.
type RequestState struct {
	Question         string         `json:"question"`
	UserID           string         `json:"user_id,omitempty"`
	TraceID          string         `json:"trace_id,omitempty"`
	ConversationID   string         `json:"conversation_id,omitempty"`
	KnowledgeBaseIDs []string       `json:"knowledge_base_ids,omitempty"`
	RuntimeOptions   RuntimeOptions `json:"runtime_options"`
}

// RuntimeOptions keeps the main runtime knobs close to the request boundary.
type RuntimeOptions struct {
	MaxIterations   int    `json:"max_iterations,omitempty"`
	AllowWebSearch  bool   `json:"allow_web_search,omitempty"`
	AllowHighRisk   bool   `json:"allow_high_risk,omitempty"`
	RequireApproval bool   `json:"require_approval,omitempty"`
	OutputMode      string `json:"output_mode,omitempty"`
}

const (
	OutputModeFinalAnswer = "final_answer"
	OutputModeHandoff     = "handoff"
)

// ContextState stores intermediate working context gathered during execution.
type ContextState struct {
	RewrittenQuery       string            `json:"rewritten_query,omitempty"`
	SearchQuery          string            `json:"search_query,omitempty"`
	SearchProvider       string            `json:"search_provider,omitempty"`
	SearchProviderActual string            `json:"search_provider_actual,omitempty"`
	SearchErrorClass     string            `json:"search_error_class,omitempty"`
	FetchErrorClass      string            `json:"fetch_error_class,omitempty"`
	SearchResults        []SearchResultRef `json:"search_results,omitempty"`
	FetchResults         []FetchResultRef  `json:"fetch_results,omitempty"`
	PreferredURLs        []string          `json:"preferred_urls,omitempty"`
	AvoidURLs            []string          `json:"avoid_urls,omitempty"`
	SeenURLs             []string          `json:"seen_urls,omitempty"`
	MemoryRefs           []MemoryRef       `json:"memory_refs,omitempty"`
	Notes                []string          `json:"notes,omitempty"`
}

// SearchResultRef is a lightweight reference to a search result rather than
// the full provider payload.
type SearchResultRef struct {
	ID         string   `json:"id,omitempty"`
	Title      string   `json:"title,omitempty"`
	URL        string   `json:"url,omitempty"`
	Snippet    string   `json:"snippet,omitempty"`
	Source     string   `json:"source,omitempty"`
	Domain     string   `json:"domain,omitempty"`
	SourceType string   `json:"source_type,omitempty"`
	Policy     string   `json:"policy,omitempty"`
	RiskFlags  []string `json:"risk_flags,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
}

// FetchResultRef is a lightweight reference to fetched page content.
type FetchResultRef struct {
	ID             string `json:"id,omitempty"`
	URL            string `json:"url,omitempty"`
	Title          string `json:"title,omitempty"`
	Summary        string `json:"summary,omitempty"`
	Text           string `json:"text,omitempty"`
	ContentRef     string `json:"content_ref,omitempty"`
	OriginalLength int    `json:"original_length,omitempty"`
	WasTruncated   bool   `json:"was_truncated,omitempty"`
	Degraded       bool   `json:"degraded,omitempty"`
	ErrorReason    string `json:"error_reason,omitempty"`
}

// MemoryRef points at recalled memory items without embedding full storage DTOs
// into runtime state.
type MemoryRef struct {
	ID           string `json:"id,omitempty"`
	Category     string `json:"category,omitempty"`
	Summary      string `json:"summary,omitempty"`
	CanonicalKey string `json:"canonical_key,omitempty"`
}

const (
	PlanStatusActive    = "active"
	PlanStatusCompleted = "completed"
	PlanStatusDegraded  = "degraded"

	PlanStepStatusPending   = "pending"
	PlanStepStatusRunning   = "running"
	PlanStepStatusCompleted = "completed"
	PlanStepStatusFailed    = "failed"
	PlanStepStatusSkipped   = "skipped"
)

// PlanState stores the explicit plan-execute workflow state used by
// non-reactive runtime patterns.
type PlanState struct {
	Goal               string         `json:"goal,omitempty"`
	PlanID             string         `json:"plan_id,omitempty"`
	Status             string         `json:"status,omitempty"`
	Steps              []PlanStep     `json:"steps,omitempty"`
	CurrentStepIndex   int            `json:"current_step_index,omitempty"`
	ReplanCount        int            `json:"replan_count,omitempty"`
	CompletionCriteria []string       `json:"completion_criteria,omitempty"`
	Confidence         string         `json:"confidence,omitempty"`
	LastPlanReason     string         `json:"last_plan_reason,omitempty"`
	LastAssessment     string         `json:"last_assessment,omitempty"`
	LastStepResult     PlanStepResult `json:"last_step_result,omitempty"`
}

// PlanStep is the checkpoint-safe representation of one explicit plan step.
type PlanStep struct {
	StepID           string         `json:"step_id,omitempty"`
	Title            string         `json:"title,omitempty"`
	Goal             string         `json:"goal,omitempty"`
	CapabilityName   string         `json:"capability_name,omitempty"`
	CapabilityKind   string         `json:"capability_kind,omitempty"`
	CapabilityFamily string         `json:"capability_family,omitempty"`
	CapabilityRole   string         `json:"capability_role,omitempty"`
	CapabilityInput  map[string]any `json:"capability_input,omitempty"`
	Consumes         []string       `json:"consumes,omitempty"`
	Produces         []string       `json:"produces,omitempty"`
	CompletionPolicy string         `json:"completion_policy,omitempty"`
	FailurePolicy    string         `json:"failure_policy,omitempty"`
	Optional         bool           `json:"optional,omitempty"`
	MaxAttempts      int            `json:"max_attempts,omitempty"`
	AttemptCount     int            `json:"attempt_count,omitempty"`
	Query            string         `json:"query,omitempty"`
	URLs             []string       `json:"urls,omitempty"`
	DependsOn        []string       `json:"depends_on,omitempty"`
	Status           string         `json:"status,omitempty"`
	RequiresApproval bool           `json:"requires_approval,omitempty"`
	ExpectedEvidence []string       `json:"expected_evidence,omitempty"`
	LastSummary      string         `json:"last_summary,omitempty"`
	LastError        string         `json:"last_error,omitempty"`
	LastErrorClass   string         `json:"last_error_class,omitempty"`
}

// PlanStepResult records the most recent step execution in a replay-friendly
// shape that later nodes can assess deterministically.
type PlanStepResult struct {
	StepID             string             `json:"step_id,omitempty"`
	CapabilityName     string             `json:"capability_name,omitempty"`
	Status             string             `json:"status,omitempty"`
	ErrorClass         string             `json:"error_class,omitempty"`
	Summary            string             `json:"summary,omitempty"`
	Observation        string             `json:"observation,omitempty"`
	Artifacts          []PlanStepArtifact `json:"artifacts,omitempty"`
	URLs               []string           `json:"urls,omitempty"`
	ProducedEvidence   bool               `json:"produced_evidence,omitempty"`
	RequiresReapproval bool               `json:"requires_reapproval,omitempty"`
	Attempt            int                `json:"attempt,omitempty"`
	StartedAt          time.Time          `json:"started_at"`
	CompletedAt        time.Time          `json:"completed_at"`
	DurationMs         int64              `json:"duration_ms,omitempty"`
}

// PlanStepArtifact is a checkpoint-safe, replay-friendly intermediate output
// emitted by one plan step for later steps or assessment policies to consume.
type PlanStepArtifact struct {
	Name         string            `json:"name,omitempty"`
	Kind         string            `json:"kind,omitempty"`
	SourceStepID string            `json:"source_step_id,omitempty"`
	Summary      string            `json:"summary,omitempty"`
	StringValues []string          `json:"string_values,omitempty"`
	Refs         []string          `json:"refs,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// EvidenceState stores the currently accepted evidence set and sufficiency
// judgment used by branch and answer decisions.
type EvidenceState struct {
	Items             []EvidenceItem `json:"items,omitempty"`
	Sufficient        bool           `json:"sufficient,omitempty"`
	SufficiencyReason string         `json:"sufficiency_reason,omitempty"`
	NewItemsThisRound int            `json:"new_items_this_round,omitempty"`
	OpenQuestions     []string       `json:"open_questions,omitempty"`
}

const (
	ApprovalStatusNone      = "none"
	ApprovalStatusPending   = "pending"
	ApprovalStatusApproved  = "approved"
	ApprovalStatusRejected  = "rejected"
	ApprovalStatusExpired   = "expired"
	ApprovalStatusCancelled = "cancelled"
)

// ApprovalState captures whether execution is blocked on an approval gate.
type ApprovalState struct {
	Status       string    `json:"status,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	Node         string    `json:"node,omitempty"`
	Capability   string    `json:"capability,omitempty"`
	CheckpointID string    `json:"checkpoint_id,omitempty"`
	RerunNode    string    `json:"rerun_node,omitempty"`
	RequestedAt  time.Time `json:"requested_at"`
	ReviewedAt   time.Time `json:"reviewed_at"`
	DecisionNote string    `json:"decision_note,omitempty"`
}

// EvidenceItem is one accepted piece of evidence plus a reference to its source.
type EvidenceItem struct {
	ID        string `json:"id,omitempty"`
	Source    string `json:"source,omitempty"`
	Content   string `json:"content,omitempty"`
	Level     string `json:"level,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}

// ExecutionState captures progress through the runtime control flow.
type ExecutionState struct {
	Status                      string   `json:"status,omitempty"`
	CurrentNode                 string   `json:"current_node,omitempty"`
	Iteration                   int      `json:"iteration,omitempty"`
	MaxIterations               int      `json:"max_iterations,omitempty"`
	ContinueCount               int      `json:"continue_count,omitempty"`
	LastBranchTarget            string   `json:"last_branch_target,omitempty"`
	LastBranchReason            string   `json:"last_branch_reason,omitempty"`
	LastProgressKind            string   `json:"last_progress_kind,omitempty"`
	LastNewURLCount             int      `json:"last_new_url_count,omitempty"`
	LastNewEvidenceCount        int      `json:"last_new_evidence_count,omitempty"`
	ConsecutiveNoProgressRounds int      `json:"consecutive_no_progress_rounds,omitempty"`
	ScheduledActions            []string `json:"scheduled_actions,omitempty"`
	CompletedActions            []string `json:"completed_actions,omitempty"`
	FailedActions               []string `json:"failed_actions,omitempty"`
	Interrupted                 bool     `json:"interrupted,omitempty"`
	InterruptReason             string   `json:"interrupt_reason,omitempty"`
}

const (
	ExecutionStatusRunning     = "running"
	ExecutionStatusInterrupted = "interrupted"
	ExecutionStatusResuming    = "resuming"
	ExecutionStatusCompleted   = "completed"
	ExecutionStatusDegraded    = "degraded"
	ExecutionStatusFailed      = "failed"
)

// AnswerState stores answer-generation results separately from evidence and
// runtime control flow.
type AnswerState struct {
	Draft         string `json:"draft,omitempty"`
	DegradeReason string `json:"degrade_reason,omitempty"`
	Final         string `json:"final,omitempty"`
}

// HasContent reports whether the snapshot contains any meaningful state beyond
// zero values. It is used by both the runtime projection layer and the kernel
// runner to decide whether InitialSnapshot should be captured.
func HasContent(snapshot StateSnapshot) bool {
	return snapshot.Request.Question != "" ||
		snapshot.Request.UserID != "" ||
		snapshot.Request.TraceID != "" ||
		snapshot.Request.ConversationID != "" ||
		len(snapshot.Request.KnowledgeBaseIDs) > 0 ||
		snapshot.Request.RuntimeOptions != (RuntimeOptions{}) ||
		snapshot.Context.RewrittenQuery != "" ||
		snapshot.Context.SearchQuery != "" ||
		snapshot.Context.SearchProvider != "" ||
		snapshot.Context.SearchProviderActual != "" ||
		snapshot.Context.SearchErrorClass != "" ||
		snapshot.Context.FetchErrorClass != "" ||
		len(snapshot.Context.SearchResults) > 0 ||
		len(snapshot.Context.FetchResults) > 0 ||
		len(snapshot.Context.PreferredURLs) > 0 ||
		len(snapshot.Context.AvoidURLs) > 0 ||
		len(snapshot.Context.SeenURLs) > 0 ||
		len(snapshot.Context.MemoryRefs) > 0 ||
		len(snapshot.Context.Notes) > 0 ||
		len(snapshot.Evidence.Items) > 0 ||
		snapshot.Evidence.Sufficient ||
		snapshot.Evidence.SufficiencyReason != "" ||
		snapshot.Evidence.NewItemsThisRound != 0 ||
		len(snapshot.Evidence.OpenQuestions) > 0 ||
		snapshot.Approval.Status != "" ||
		snapshot.Approval.Reason != "" ||
		snapshot.Approval.Node != "" ||
		snapshot.Approval.Capability != "" ||
		snapshot.Approval.CheckpointID != "" ||
		snapshot.Approval.RerunNode != "" ||
		!snapshot.Approval.RequestedAt.IsZero() ||
		!snapshot.Approval.ReviewedAt.IsZero() ||
		snapshot.Approval.DecisionNote != "" ||
		snapshot.Execution.Status != "" ||
		snapshot.Execution.CurrentNode != "" ||
		snapshot.Execution.Iteration != 0 ||
		snapshot.Execution.MaxIterations != 0 ||
		snapshot.Execution.ContinueCount != 0 ||
		snapshot.Execution.LastBranchTarget != "" ||
		snapshot.Execution.LastBranchReason != "" ||
		snapshot.Execution.LastProgressKind != "" ||
		snapshot.Execution.LastNewURLCount != 0 ||
		snapshot.Execution.LastNewEvidenceCount != 0 ||
		snapshot.Execution.ConsecutiveNoProgressRounds != 0 ||
		len(snapshot.Execution.ScheduledActions) > 0 ||
		len(snapshot.Execution.CompletedActions) > 0 ||
		len(snapshot.Execution.FailedActions) > 0 ||
		snapshot.Execution.Interrupted ||
		snapshot.Execution.InterruptReason != "" ||
		snapshot.Answer.Draft != "" ||
		snapshot.Answer.DegradeReason != "" ||
		snapshot.Answer.Final != "" ||
		snapshot.Pattern.Name != "" ||
		len(snapshot.Pattern.Data) > 0
}

// NormalizeSnapshot applies compatibility defaults so older persisted sessions
// can still be projected and reduced under the current schema contract.
func NormalizeSnapshot(snapshot StateSnapshot) StateSnapshot {
	normalized := snapshot
	if normalized.SchemaVersion == LegacySnapshotVersion {
		normalized.SchemaVersion = CurrentSnapshotVersion
	}
	normalized.Execution.Status = deriveExecutionStatus(normalized)
	return normalized
}

func ValidateSnapshotCompatibility(snapshot StateSnapshot) error {
	switch {
	case snapshot.SchemaVersion < LegacySnapshotVersion:
		return fmt.Errorf("unsupported snapshot schema version %d", snapshot.SchemaVersion)
	case snapshot.SchemaVersion > CurrentSnapshotVersion:
		return fmt.Errorf("unsupported snapshot schema version %d", snapshot.SchemaVersion)
	default:
		return nil
	}
}

func deriveExecutionStatus(snapshot StateSnapshot) string {
	status := strings.TrimSpace(snapshot.Execution.Status)
	if status == ExecutionStatusResuming {
		return status
	}
	if snapshot.Execution.Interrupted || strings.TrimSpace(snapshot.Approval.Status) == ApprovalStatusPending {
		return ExecutionStatusInterrupted
	}
	if strings.TrimSpace(snapshot.Answer.DegradeReason) != "" {
		return ExecutionStatusDegraded
	}
	if strings.TrimSpace(snapshot.Answer.Final) != "" {
		return ExecutionStatusCompleted
	}
	if status == ExecutionStatusFailed {
		return status
	}
	return ExecutionStatusRunning
}
