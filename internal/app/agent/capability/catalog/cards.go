package catalog

// Card is the LLM-facing summary of one registered capability.
type Card struct {
	Name             string   `json:"name"`
	Kind             string   `json:"kind,omitempty"`
	Family           string   `json:"family,omitempty"`
	Roles            []string `json:"roles,omitempty"`
	Summary          string   `json:"summary,omitempty"`
	InputHints       []string `json:"input_hints,omitempty"`
	RequiresApproval bool     `json:"requires_approval,omitempty"`
	SupportsResume   bool     `json:"supports_resume,omitempty"`
	ProducesEvidence bool     `json:"produces_evidence,omitempty"`
}
