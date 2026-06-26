package runtime

// Outcome is the normalized runtime-facing lifecycle result produced by the
// engine facade after one run or resume attempt.
type Outcome struct {
	Decision      Decision `json:"decision,omitempty"`
	CheckpointID  string   `json:"checkpoint_id,omitempty"`
	Interrupted   bool     `json:"interrupted,omitempty"`
	Reason        string   `json:"reason,omitempty"`
	ErrorClass    string   `json:"error_class,omitempty"`
	DegradeReason string   `json:"degrade_reason,omitempty"`
}

// RunResult keeps the final runtime session together with the normalized
// runtime-owned outcome that service code can project outward.
type RunResult struct {
	Session *RuntimeSession `json:"session,omitempty"`
	Outcome Outcome         `json:"outcome,omitempty"`
}
