package capability

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"local/rag-project/internal/app/runtime/persistence"
)

const (
	ArchiveConversationEpisodeID = "archive_conversation_episode"
	SearchConversationHistoryID  = "search_conversation_history"
)

type EpisodeEmbedding interface {
	Embed(string) ([]float32, error)
}
type EpisodeIDFactory func() (string, error)

type EpisodeService struct {
	store     persistence.EpisodeStore
	embedding EpisodeEmbedding
	ids       EpisodeIDFactory
	now       func() time.Time
}

func NewEpisodeService(store persistence.EpisodeStore, embedding EpisodeEmbedding, ids EpisodeIDFactory) *EpisodeService {
	return &EpisodeService{store: store, embedding: embedding, ids: ids, now: time.Now}
}

func (s *EpisodeService) Archive(ctx Context, summary string, topics []string, importance string) (persistence.Episode, error) {
	if s == nil || s.store == nil || s.embedding == nil || s.ids == nil {
		return persistence.Episode{}, fmt.Errorf("conversation episode service is not configured")
	}
	id, err := s.ids()
	if err != nil {
		return persistence.Episode{}, err
	}
	now := s.now().UTC()
	start, end := relativeTimeWindow(ctx.Question, now)
	episode := persistence.Episode{ID: id, RuntimeSessionID: ctx.RuntimeSessionID, ConversationID: ctx.ConversationID, UserID: ctx.UserID, SourceUserMessageID: ctx.UserMessageID, Summary: summary, Topics: topics, Importance: importance, MentionedStart: start, MentionedEnd: end, Status: persistence.EpisodePending, CreatedAt: now, UpdatedAt: now}
	vector, err := s.embedding.Embed(strings.Join(append([]string{summary}, topics...), "\n"))
	if err != nil {
		return persistence.Episode{}, fmt.Errorf("embed conversation episode: %w", err)
	}
	if err := s.store.CreateEpisode(ctx.Context, episode, vector); err != nil {
		return persistence.Episode{}, err
	}
	return episode, nil
}

func (s *EpisodeService) Search(ctx Context, query, conversationID string, topK int) ([]persistence.EpisodeHit, bool, error) {
	if s == nil || s.store == nil || s.embedding == nil {
		return nil, false, fmt.Errorf("conversation episode service is not configured")
	}
	vector, err := s.embedding.Embed(query)
	if err != nil {
		return nil, false, fmt.Errorf("embed conversation history query: %w", err)
	}
	now := s.now().UTC()
	start, end := relativeTimeWindow(query, now)
	hits, err := s.store.SearchEpisodes(ctx.Context, persistence.EpisodeSearch{UserID: ctx.UserID, ConversationID: conversationID, Start: start, End: end, Vector: vector, Limit: topK * 3})
	if err != nil {
		return nil, false, err
	}
	for i := range hits {
		hits[i].Score += importanceBoost(hits[i].Importance) + topicBoost(query, hits[i].Topics)
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	if len(hits) > topK {
		hits = hits[:topK]
	}
	ambiguous := start == nil && len(hits) > 1 && hits[0].Score-hits[1].Score < 0.05
	return hits, ambiguous, nil
}

func importanceBoost(value string) float32 {
	switch value {
	case "high":
		return .06
	case "normal":
		return .03
	default:
		return 0
	}
}
func topicBoost(query string, topics []string) float32 {
	query = strings.ToLower(query)
	var boost float32
	for _, topic := range topics {
		if topic != "" && strings.Contains(query, strings.ToLower(topic)) {
			boost += .04
		}
	}
	return boost
}

// relativeTimeWindow deliberately resolves only unambiguous Chinese relative
// forms. The stored interval is absolute, so repeated "上周" questions do not
// collapse into one semantic vector later.
func relativeTimeWindow(text string, now time.Time) (*time.Time, *time.Time) {
	text = strings.ReplaceAll(strings.TrimSpace(text), " ", "")
	if strings.Contains(text, "两周前") {
		return calendarWeek(now.AddDate(0, 0, -14))
	}
	if strings.Contains(text, "上周") {
		return calendarWeek(now.AddDate(0, 0, -7))
	}
	if strings.Contains(text, "本周") {
		return calendarWeek(now)
	}
	return nil, nil
}
func calendarWeek(now time.Time) (*time.Time, *time.Time) {
	offset := (int(now.Weekday()) + 6) % 7
	start := time.Date(now.Year(), now.Month(), now.Day()-offset, 0, 0, 0, 0, now.Location())
	end := start.AddDate(0, 0, 7)
	return &start, &end
}
