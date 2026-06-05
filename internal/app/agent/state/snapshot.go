package state

import "time"

// StateSnapshot is the structured view of the current runtime state.
// Nodes read from it; later M1 reducer logic will be the only writer.
type StateSnapshot struct {
	Request   RequestState   `json:"request"`
	Context   ContextState   `json:"context"`
	Plan      PlanState      `json:"plan"`
	Evidence  EvidenceState  `json:"evidence"`
	Approval  ApprovalState  `json:"approval"`
	Execution ExecutionState `json:"execution"`
	Answer    AnswerState    `json:"answer"`
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
	CapabilityName   string         `json:"capability_name,omitempty"`
	CapabilityKind   string         `json:"capability_kind,omitempty"`
	CapabilityFamily string         `json:"capability_family,omitempty"`
	CapabilityRole   string         `json:"capability_role,omitempty"`
	CapabilityInput  map[string]any `json:"capability_input,omitempty"`
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
	StepID             string   `json:"step_id,omitempty"`
	CapabilityName     string   `json:"capability_name,omitempty"`
	Status             string   `json:"status,omitempty"`
	ErrorClass         string   `json:"error_class,omitempty"`
	Summary            string   `json:"summary,omitempty"`
	URLs               []string `json:"urls,omitempty"`
	ProducedEvidence   bool     `json:"produced_evidence,omitempty"`
	RequiresReapproval bool     `json:"requires_reapproval,omitempty"`
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
	ApprovalStatusPending  = "pending"
	ApprovalStatusApproved = "approved"
	ApprovalStatusRejected = "rejected"
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

// AnswerState stores answer-generation results separately from evidence and
// runtime control flow.
type AnswerState struct {
	Draft         string `json:"draft,omitempty"`
	DegradeReason string `json:"degrade_reason,omitempty"`
	Final         string `json:"final,omitempty"`
}
