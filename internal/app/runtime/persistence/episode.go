package persistence

import (
	"context"
	"time"
)

const (
	EpisodePending   = "pending"
	EpisodeReady     = "ready"
	EpisodeDiscarded = "discarded"
)

// Episode is a selectively archived, user-owned conversation fact. It is not
// a copy of every chat message and becomes searchable only after its final
// assistant answer has been linked.
type Episode struct {
	ID                       string     `json:"id"`
	RuntimeSessionID         string     `json:"runtimeSessionId"`
	ConversationID           string     `json:"conversationId"`
	UserID                   string     `json:"userId"`
	SourceUserMessageID      string     `json:"sourceUserMessageId"`
	SourceAssistantMessageID string     `json:"sourceAssistantMessageId"`
	Summary                  string     `json:"summary"`
	Topics                   []string   `json:"topics"`
	Importance               string     `json:"importance"`
	MentionedStart           *time.Time `json:"mentionedStart,omitempty"`
	MentionedEnd             *time.Time `json:"mentionedEnd,omitempty"`
	Status                   string     `json:"status"`
	CreatedAt                time.Time  `json:"createdAt"`
	UpdatedAt                time.Time  `json:"updatedAt"`
}

type EpisodeSearch struct {
	UserID         string
	ConversationID string
	Start          *time.Time
	End            *time.Time
	Vector         []float32
	Limit          int
}

type EpisodeHit struct {
	Episode
	Score float32 `json:"score"`
}

type EpisodeStore interface {
	CreateEpisode(context.Context, Episode, []float32) error
	CompleteEpisodes(context.Context, string, string) error
	DiscardEpisodes(context.Context, string) error
	SearchEpisodes(context.Context, EpisodeSearch) ([]EpisodeHit, error)
}
