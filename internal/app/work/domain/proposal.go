package domain

import (
	"fmt"
	"time"
)

type StateChange struct {
	Kind  string     `json:"kind"`
	Entry StateEntry `json:"entry"`
}
type Proposal struct {
	ID              string            `json:"id"`
	TopicID         string            `json:"topicId"`
	TurnID          string            `json:"turnId"`
	BaseRevision    int               `json:"baseRevision"`
	Changes         []StateChange     `json:"changes"`
	Document        *DocumentProposal `json:"document,omitempty"`
	Status          string            `json:"status"`
	AppliedRevision int               `json:"appliedRevision,omitempty"`
	CreatedAt       time.Time         `json:"createdAt"`
}

// A proposed document is a preview. Only a human resolve request can save it.
type DocumentProposal struct {
	Kind       string   `json:"kind"`
	ArtifactID string   `json:"artifactId,omitempty"`
	ItemID     string   `json:"itemId,omitempty"`
	Title      string   `json:"title"`
	Summary    string   `json:"summary"`
	Body       Document `json:"body"`
}

type SuggestDocument struct {
	ArtifactID string           `json:"artifactId,omitempty"`
	Change     AIDocumentChange `json:"change"`
}
type ResolveProposal struct {
	Mutation
	Ignore  bool          `json:"ignore"`
	Changes []StateChange `json:"changes,omitempty"`
}

func ApplyStateChanges(entries []StateEntry, changes []StateChange) ([]StateEntry, error) {
	if len(changes) == 0 || len(changes) > 50 {
		return nil, fmt.Errorf("a proposal needs 1..50 changes")
	}
	out := append([]StateEntry{}, entries...)
	for _, c := range changes {
		at := -1
		for i, e := range out {
			if e.ID == c.Entry.ID {
				at = i
				break
			}
		}
		switch c.Kind {
		case "add":
			if at >= 0 {
				return nil, fmt.Errorf("progress entry already exists")
			}
			out = append(out, c.Entry)
		case "update":
			if at < 0 {
				return nil, fmt.Errorf("progress entry no longer exists")
			}
			out[at] = c.Entry
		case "remove":
			if at < 0 {
				return nil, fmt.Errorf("progress entry no longer exists")
			}
			out = append(out[:at], out[at+1:]...)
		default:
			return nil, fmt.Errorf("unsupported progress change")
		}
	}
	return out, ValidateEntries(out)
}

type AIDocumentChange struct {
	Title   string        `json:"title"`
	Summary string        `json:"summary"`
	Body    *Document     `json:"body,omitempty"`
	Changes []BlockChange `json:"changes,omitempty"`
}
