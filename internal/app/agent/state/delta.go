package state

import "time"

// StateDelta is a node's requested state change. Nil sub-deltas mean the
// corresponding state domain is unchanged.
type StateDelta struct {
	Request   *RequestDelta   `json:"request,omitempty"`
	Context   *ContextDelta   `json:"context,omitempty"`
	Plan      *PlanDelta      `json:"plan,omitempty"`
	Evidence  *EvidenceDelta  `json:"evidence,omitempty"`
	Approval  *ApprovalDelta  `json:"approval,omitempty"`
	Execution *ExecutionDelta `json:"execution,omitempty"`
	Answer    *AnswerDelta    `json:"answer,omitempty"`
}

// RequestDelta is intentionally small because request-scoped state should stay
// stable across the run.
type RequestDelta struct {
	ConversationID   *string         `json:"conversation_id,omitempty"`
	KnowledgeBaseIDs []string        `json:"knowledge_base_ids,omitempty"`
	RuntimeOptions   *RuntimeOptions `json:"runtime_options,omitempty"`
}

// ContextDelta carries updates to intermediate working context.
type ContextDelta struct {
	RewrittenQuery       *string           `json:"rewritten_query,omitempty"`
	SearchQuery          *string           `json:"search_query,omitempty"`
	SearchProvider       *string           `json:"search_provider,omitempty"`
	SearchProviderActual *string           `json:"search_provider_actual,omitempty"`
	SearchErrorClass     *string           `json:"search_error_class,omitempty"`
	FetchErrorClass      *string           `json:"fetch_error_class,omitempty"`
	ResetSearchResults   bool              `json:"reset_search_results,omitempty"`
	ResetFetchResults    bool              `json:"reset_fetch_results,omitempty"`
	SearchResults        []SearchResultRef `json:"search_results,omitempty"`
	FetchResults         []FetchResultRef  `json:"fetch_results,omitempty"`
	PreferredURLs        *[]string         `json:"preferred_urls,omitempty"`
	AvoidURLs            *[]string         `json:"avoid_urls,omitempty"`
	SeenURLs             []string          `json:"seen_urls,omitempty"`
	MemoryRefs           []MemoryRef       `json:"memory_refs,omitempty"`
	Notes                []string          `json:"notes,omitempty"`
}

// PlanDelta currently keeps plan mutation coarse-grained and replay-friendly:
// the entire plan state is replaced atomically. Step patch and step-result
// append semantics must still be expressed by building the next PlanState and
// supplying it through Replace.
type PlanDelta struct {
	Replace *PlanState `json:"replace,omitempty"`
}

// EvidenceDelta carries additive evidence changes with shared reducer rules:
// evidence items are appended by stable identity, sufficiency is last-writer
// wins when explicitly set, and open questions are accumulated uniquely.
type EvidenceDelta struct {
	AddItems          []EvidenceItem `json:"add_items,omitempty"`
	Sufficient        *bool          `json:"sufficient,omitempty"`
	SufficiencyReason *string        `json:"sufficiency_reason,omitempty"`
	NewItemsThisRound *int           `json:"new_items_this_round,omitempty"`
	OpenQuestions     []string       `json:"open_questions,omitempty"`
}

// ApprovalDelta carries approval gate changes for pending/resolved execution.
type ApprovalDelta struct {
	Status       *string    `json:"status,omitempty"`
	Reason       *string    `json:"reason,omitempty"`
	Node         *string    `json:"node,omitempty"`
	Capability   *string    `json:"capability,omitempty"`
	CheckpointID *string    `json:"checkpoint_id,omitempty"`
	RerunNode    *string    `json:"rerun_node,omitempty"`
	RequestedAt  *time.Time `json:"requested_at,omitempty"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	DecisionNote *string    `json:"decision_note,omitempty"`
}

// ExecutionDelta carries control-flow progress updates.
type ExecutionDelta struct {
	Status                      *string `json:"status,omitempty"`
	CurrentNode                 *string  `json:"current_node,omitempty"`
	IterationIncrement          int      `json:"iteration_increment,omitempty"`
	ContinueCountIncrement      int      `json:"continue_count_increment,omitempty"`
	LastBranchTarget            *string  `json:"last_branch_target,omitempty"`
	LastBranchReason            *string  `json:"last_branch_reason,omitempty"`
	LastProgressKind            *string  `json:"last_progress_kind,omitempty"`
	LastNewURLCount             *int     `json:"last_new_url_count,omitempty"`
	LastNewEvidenceCount        *int     `json:"last_new_evidence_count,omitempty"`
	ConsecutiveNoProgressRounds *int     `json:"consecutive_no_progress_rounds,omitempty"`
	ScheduledActions            []string `json:"scheduled_actions,omitempty"`
	CompletedActions            []string `json:"completed_actions,omitempty"`
	FailedActions               []string `json:"failed_actions,omitempty"`
	Interrupted                 *bool    `json:"interrupted,omitempty"`
	InterruptReason             *string  `json:"interrupt_reason,omitempty"`
}

// AnswerDelta carries answer-phase updates such as draft text, degrade reason,
// and the final answer.
type AnswerDelta struct {
	Draft         *string `json:"draft,omitempty"`
	DegradeReason *string `json:"degrade_reason,omitempty"`
	Final         *string `json:"final,omitempty"`
}
