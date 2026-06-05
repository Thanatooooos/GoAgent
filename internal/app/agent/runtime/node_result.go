package runtime

import agentstate "local/rag-project/internal/app/agent/state"

// NodeResult is the standard contract returned by a runtime graph node.
type NodeResult struct {
	Events   []agentstate.RuntimeEvent `json:"events,omitempty"`
	Delta    agentstate.StateDelta     `json:"delta"`
	Decision *DecisionArtifact         `json:"decision,omitempty"`
}

// DecisionArtifact is the runtime's normalized decision object used by nodes
// and policy layers to describe the next runtime action in a compact form.
type DecisionArtifact struct {
	Kind       string  `json:"kind,omitempty"`
	Target     string  `json:"target,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
	Reasoning  string  `json:"reasoning,omitempty"`
}
