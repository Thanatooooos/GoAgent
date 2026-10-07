package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrNotFound      = errors.New("work resource not found")
	ErrArchived      = errors.New("work topic is archived")
	ErrRequestReused = errors.New("request id was reused with different input")
)

type Conflict struct{ CurrentRevision int }

func (e *Conflict) Error() string {
	return fmt.Sprintf("work version conflict: current revision is %d", e.CurrentRevision)
}

type Topic struct {
	ID          string    `json:"id"`
	UserID      string    `json:"-"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	Revision    int       `json:"revision"`
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}
type Item struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topicId"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}
type Conversation struct {
	ID           string    `json:"id"`
	TopicID      string    `json:"topicId"`
	ItemID       string    `json:"itemId,omitempty"`
	ContinueFrom string    `json:"continueFrom,omitempty"`
	Title        string    `json:"title"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
type Reference struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision int    `json:"revision,omitempty"`
}
type StateEntry struct {
	ID         string      `json:"id"`
	Kind       string      `json:"kind"`
	Text       string      `json:"text"`
	ItemID     string      `json:"itemId,omitempty"`
	References []Reference `json:"references,omitempty"`
}
type State struct {
	TopicID   string       `json:"topicId"`
	Revision  int          `json:"revision"`
	Entries   []StateEntry `json:"entries"`
	Author    string       `json:"author"`
	CreatedAt time.Time    `json:"createdAt"`
}
type Artifact struct {
	ID        string    `json:"id"`
	TopicID   string    `json:"topicId"`
	ItemID    string    `json:"itemId,omitempty"`
	Title     string    `json:"title"`
	Revision  int       `json:"revision"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
type ArtifactVersion struct {
	ArtifactID     string    `json:"artifactId"`
	Revision       int       `json:"revision"`
	Title          string    `json:"title"`
	Body           Document  `json:"body"`
	Author         string    `json:"author"`
	Summary        string    `json:"summary"`
	ConversationID string    `json:"conversationId,omitempty"`
	RestoredFrom   int       `json:"restoredFrom,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}
type ArtifactDetail struct {
	Artifact Artifact        `json:"artifact"`
	Version  ArtifactVersion `json:"version"`
}
type Page struct {
	Limit  int
	Offset int
}

func (p Page) Validate() error {
	if p.Limit < 1 || p.Limit > 100 || p.Offset < 0 {
		return fmt.Errorf("limit must be 1..100 and offset non-negative")
	}
	return nil
}

type Mutation struct {
	RequestID        string `json:"requestId"`
	ExpectedRevision int    `json:"expectedRevision"`
}

func (m Mutation) Validate() error {
	if strings.TrimSpace(m.RequestID) == "" || len(m.RequestID) > 128 || m.ExpectedRevision < 0 {
		return fmt.Errorf("requestId and non-negative expectedRevision are required")
	}
	return nil
}

type CreateTopic struct {
	Mutation
	Name        string `json:"name"`
	Description string `json:"description"`
}
type UpdateTopic struct {
	Mutation
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      string `json:"status"`
}
type CreateItem struct {
	Mutation
	Name string `json:"name"`
}
type CreateConversation struct {
	Mutation
	Title        string `json:"title"`
	ItemID       string `json:"itemId,omitempty"`
	ContinueFrom string `json:"continueFrom,omitempty"`
}
type SaveState struct {
	Mutation
	Entries []StateEntry `json:"entries"`
}
type SaveArtifact struct {
	Mutation
	Title   string   `json:"title"`
	ItemID  string   `json:"itemId,omitempty"`
	Body    Document `json:"body"`
	Summary string   `json:"summary"`
}
type RestoreArtifact struct {
	Mutation
	Revision int `json:"revision"`
}

func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > 128 {
		return fmt.Errorf("name must contain 1..128 characters")
	}
	return nil
}
func ValidateEntries(entries []StateEntry) error {
	if len(entries) > 1000 {
		return fmt.Errorf("too many progress entries")
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if strings.TrimSpace(e.ID) == "" || len(e.ID) > 128 || seen[e.ID] {
			return fmt.Errorf("progress entry ids must be unique and non-empty")
		}
		seen[e.ID] = true
		if strings.TrimSpace(e.Text) == "" || utf8.RuneCountInString(e.Text) > 8000 {
			return fmt.Errorf("invalid progress entry text")
		}
		switch e.Kind {
		case "goal", "constraint", "decision", "question", "next":
		default:
			return fmt.Errorf("unsupported progress entry kind")
		}
		if len(e.References) > 30 {
			return fmt.Errorf("too many progress references")
		}
	}
	return nil
}
