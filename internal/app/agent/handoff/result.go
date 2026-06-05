package handoff

import (
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
)

const (
	ActionHandoffToRAG = "handoff_to_rag"
	ActionFinalAnswer  = "final_answer"
	ActionDegrade      = "degrade"
)

type Result struct {
	Used            bool                    `json:"used"`
	ToolContext     string                  `json:"tool_context,omitempty"`
	AnswerGuidance  string                  `json:"answer_guidance,omitempty"`
	WorkflowPolicy  string                  `json:"workflow_policy,omitempty"`
	EvidenceBundle  EvidenceBundle          `json:"evidence_bundle"`
	DecisionSummary DecisionSummary         `json:"decision_summary"`
	Replay          agentruntime.ReplayView `json:"replay"`
	Degraded        bool                    `json:"degraded"`
	DegradeReason   string                  `json:"degrade_reason,omitempty"`
}

type EvidenceBundle struct {
	Question          string                         `json:"question,omitempty"`
	SearchQuery       string                         `json:"search_query,omitempty"`
	Provider          string                         `json:"provider,omitempty"`
	SearchResults     []agentsearch.SearchResultItem `json:"search_results,omitempty"`
	Pages             []agentfetch.PageResult        `json:"pages,omitempty"`
	AcceptedEvidence  []AcceptedEvidenceItem         `json:"accepted_evidence,omitempty"`
	Sufficient        bool                           `json:"sufficient"`
	SufficiencyReason string                         `json:"sufficiency_reason,omitempty"`
	OpenQuestions     []string                       `json:"open_questions,omitempty"`
	NewItemsThisRound int                            `json:"new_items_this_round,omitempty"`
}

type AcceptedEvidenceItem struct {
	ID        string `json:"id,omitempty"`
	Source    string `json:"source,omitempty"`
	Content   string `json:"content,omitempty"`
	Level     string `json:"level,omitempty"`
	SourceRef string `json:"source_ref,omitempty"`
}

type DecisionSummary struct {
	FinalAction                 string  `json:"final_action,omitempty"`
	Reason                      string  `json:"reason,omitempty"`
	Confidence                  float64 `json:"confidence,omitempty"`
	Iteration                   int     `json:"iteration,omitempty"`
	MaxIterations               int     `json:"max_iterations,omitempty"`
	ContinueCount               int     `json:"continue_count,omitempty"`
	LastProgressKind            string  `json:"last_progress_kind,omitempty"`
	LastNewURLCount             int     `json:"last_new_url_count,omitempty"`
	LastNewEvidenceCount        int     `json:"last_new_evidence_count,omitempty"`
	ConsecutiveNoProgressRounds int     `json:"consecutive_no_progress_rounds,omitempty"`
}

type CapabilityProfile struct {
	Node               string `json:"node,omitempty"`
	Name               string `json:"name,omitempty"`
	WorkflowCapability string `json:"workflow_capability,omitempty"`
	Kind               string `json:"kind,omitempty"`
	RiskLevel          string `json:"risk_level,omitempty"`
	RequiresApproval   bool   `json:"requires_approval,omitempty"`
	SupportsParallel   bool   `json:"supports_parallel,omitempty"`
	SupportsResume     bool   `json:"supports_resume,omitempty"`
}

type Builder struct {
	profiles map[string]CapabilityProfile
}
