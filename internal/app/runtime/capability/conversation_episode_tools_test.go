package capability

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/runtime/persistence"
)

func TestArchiveConversationEpisodeCreatesPendingEpisodeWithAbsoluteWeek(t *testing.T) {
	store := &episodeStoreStub{}
	service := NewEpisodeService(store, embeddingStub{}, func() (string, error) { return "episode-1", nil })
	service.now = func() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.Local) }
	def := ArchiveConversationEpisode(service)
	_, err := def.Execute(Value(`{"summary":"Tika timeout root cause","topics":["Tika","import"],"importance":"high"}`), Context{Context: context.Background(), RuntimeSessionID: "session-1", ConversationID: "conversation-1", UserID: "user-1", UserMessageID: "message-1", Question: "上周的 Tika 导入问题", AllowEpisodeArchive: true})
	if err != nil {
		t.Fatal(err)
	}
	if store.created.Status != persistence.EpisodePending || store.created.MentionedStart == nil || store.created.MentionedEnd == nil {
		t.Fatalf("episode = %#v", store.created)
	}
	if store.created.MentionedStart.Format("2006-01-02") != "2026-08-31" || store.created.MentionedEnd.Format("2006-01-02") != "2026-09-07" {
		t.Fatalf("week = %v..%v", store.created.MentionedStart, store.created.MentionedEnd)
	}
}

func TestSearchConversationHistoryRanksTopicAndMarksCloseResultsAmbiguous(t *testing.T) {
	store := &episodeStoreStub{hits: []persistence.EpisodeHit{{Episode: persistence.Episode{ID: "old", Topics: []string{"Tika"}, Importance: "low"}, Score: .8}, {Episode: persistence.Episode{ID: "new", Topics: []string{"Tika"}, Importance: "normal"}, Score: .79}}}
	service := NewEpisodeService(store, embeddingStub{}, func() (string, error) { return "", nil })
	hits, ambiguous, err := service.Search(Context{Context: context.Background(), UserID: "user-1"}, "之前的 Tika 问题", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !ambiguous || len(hits) != 2 || hits[0].ID != "new" {
		t.Fatalf("hits=%#v ambiguous=%v", hits, ambiguous)
	}
}

func TestSearchConversationHistoryUsesAbsoluteWeekFilter(t *testing.T) {
	store := &episodeStoreStub{}
	service := NewEpisodeService(store, embeddingStub{}, func() (string, error) { return "", nil })
	service.now = func() time.Time { return time.Date(2026, 9, 8, 12, 0, 0, 0, time.Local) }
	_, _, err := service.Search(Context{Context: context.Background(), UserID: "user-1"}, "上周的问题", "", 3)
	if err != nil {
		t.Fatal(err)
	}
	if store.search.Start == nil || store.search.Start.Format("2006-01-02") != "2026-08-31" {
		t.Fatalf("search=%#v", store.search)
	}
}

type embeddingStub struct{}

func (embeddingStub) Embed(string) ([]float32, error) { return []float32{.1, .2}, nil }

type episodeStoreStub struct {
	created persistence.Episode
	search  persistence.EpisodeSearch
	hits    []persistence.EpisodeHit
}

func (s *episodeStoreStub) CreateEpisode(_ context.Context, episode persistence.Episode, _ []float32) error {
	s.created = episode
	return nil
}
func (*episodeStoreStub) CompleteEpisodes(context.Context, string, string) error { return nil }
func (*episodeStoreStub) DiscardEpisodes(context.Context, string) error          { return nil }
func (s *episodeStoreStub) SearchEpisodes(_ context.Context, search persistence.EpisodeSearch) ([]persistence.EpisodeHit, error) {
	s.search = search
	return append([]persistence.EpisodeHit(nil), s.hits...), nil
}
