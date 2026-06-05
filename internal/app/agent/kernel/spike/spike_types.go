// Package spike contains the M1 spike code for validating Eino Graph's
// branch, checkpoint, and interrupt capabilities as the execution base
// for the new Agent Runtime.
//
// This package is a temporary spike — successful patterns will be promoted
// into the kernel proper; the rest will be discarded.
package spike

import "github.com/cloudwego/eino/schema"

func init() {
	// Register spike types with Eino's serialization system so checkpoint
	// persist/restore can marshal and unmarshal *SpikeState correctly.
	schema.RegisterName[*SpikeState]("spike_SpikeState")
	schema.RegisterName[EvidenceItem]("spike_EvidenceItem")
	schema.RegisterName[NodeEvent]("spike_NodeEvent")
}

// =============================================================================
// Spike Types — Minimal models mirroring the design doc's core abstractions
// =============================================================================

// EvidenceItem represents one piece of accumulated evidence during execution.
type EvidenceItem struct {
	Source string `json:"source"` // "search" | "fetch" | "diagnose" | "retrieve"
	Fact   string `json:"fact"`
	Level  string `json:"level"` // "high" | "medium" | "low"
}

// NodeEvent is a single runtime event emitted by a graph node.
// Mirrors the design doc's RuntimeEvent.
type NodeEvent struct {
	Node      string `json:"node"`
	EventType string `json:"event_type"` // "start" | "finish" | "decision" | "degrade"
	Data      string `json:"data"`
}

// SpikeState is the typed state that flows through the Eino graph.
// It is a minimal version of the design doc's StateSnapshot, containing
// enough structure to validate state propagation, branching, and journal
// accumulation without over-engineering.
type SpikeState struct {
	// Request domain
	Question string `json:"question"`

	// Context domain
	SearchQuery  string `json:"search_query"`
	SearchResult string `json:"search_result"`
	FetchResult  string `json:"fetch_result"`

	// Evidence domain
	Evidence []EvidenceItem `json:"evidence"`

	// Answer domain
	Answer        string `json:"answer"`
	DegradeReason string `json:"degrade_reason"`

	// Execution domain
	NodeJournal []NodeEvent `json:"node_journal"`
	Rounds      int         `json:"rounds"`
	MaxRounds   int         `json:"max_rounds"`
}

// AddEvent appends a node event to the journal.
func (s *SpikeState) AddEvent(node, eventType, data string) {
	s.NodeJournal = append(s.NodeJournal, NodeEvent{
		Node: node, EventType: eventType, Data: data,
	})
}

// AddEvidence appends an evidence item.
func (s *SpikeState) AddEvidence(source, fact, level string) {
	s.Evidence = append(s.Evidence, EvidenceItem{
		Source: source, Fact: fact, Level: level,
	})
}

// HasEvidence returns true if any evidence has been accumulated.
func (s *SpikeState) HasEvidence() bool {
	return len(s.Evidence) > 0
}

// Clone returns a deep copy of the state.
func (s *SpikeState) Clone() *SpikeState {
	c := *s
	c.Evidence = make([]EvidenceItem, len(s.Evidence))
	copy(c.Evidence, s.Evidence)
	c.NodeJournal = make([]NodeEvent, len(s.NodeJournal))
	copy(c.NodeJournal, s.NodeJournal)
	return &c
}

// EventCount returns the number of events for a given node name.
func (s *SpikeState) EventCount(node string) int {
	n := 0
	for _, e := range s.NodeJournal {
		if e.Node == node {
			n++
		}
	}
	return n
}

// EventsByType returns events matching the given EventType.
func (s *SpikeState) EventsByType(eventType string) []NodeEvent {
	var out []NodeEvent
	for _, e := range s.NodeJournal {
		if e.EventType == eventType {
			out = append(out, e)
		}
	}
	return out
}

// =============================================================================
// Spike-specific helpers used in graph node logic
// =============================================================================

// SimulateSearch returns a fake search result for testing.
func SimulateSearch(query string) string {
	if query == "" {
		return ""
	}
	return "search result for: " + query
}

// SimulateFetch returns a fake fetch result for testing.
func SimulateFetch(url string) string {
	if url == "" {
		return ""
	}
	return "fetched content from: " + url
}
