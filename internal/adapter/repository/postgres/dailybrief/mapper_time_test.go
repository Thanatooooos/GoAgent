package dailybrief

import (
	"testing"
	"time"

	"local/rag-project/internal/adapter/repository/postgres/dailybrief/models"
)

func TestGenerationRunFinishedAtUsesStoredLocalWallTime(t *testing.T) {
	location := time.FixedZone("test-local", 8*60*60)
	stored := time.Date(2026, 9, 29, 20, 29, 49, 0, time.UTC)
	finishedAt := timestampWallTime(stored, location)
	now := time.Date(2026, 9, 29, 21, 29, 49, 0, location)
	if now.Sub(finishedAt) != time.Hour {
		t.Fatalf("finished_at = %v, elapsed = %v", finishedAt, now.Sub(finishedAt))
	}
}

func TestIssuePublicationTimeUsesStoredLocalWallTime(t *testing.T) {
	original := time.Local
	time.Local = time.FixedZone("test-local", 8*60*60)
	t.Cleanup(func() { time.Local = original })
	stored := time.Date(2026, 10, 6, 19, 19, 3, 0, time.UTC)
	issue := toIssueDomain(models.IssueModel{GeneratedAt: &stored, PublishedAt: &stored})
	expected := time.Date(2026, 10, 6, 11, 19, 3, 0, time.UTC)
	if issue.GeneratedAt == nil || issue.PublishedAt == nil ||
		!issue.GeneratedAt.Equal(expected) || !issue.PublishedAt.Equal(expected) {
		t.Fatalf("wrong publication instant: generated=%v published=%v", issue.GeneratedAt, issue.PublishedAt)
	}
	if stored.Hour() != 19 || stored.Location() != time.UTC {
		t.Fatal("mapping mutated the persisted model timestamp")
	}
	empty := toIssueDomain(models.IssueModel{})
	if empty.GeneratedAt != nil || empty.PublishedAt != nil {
		t.Fatal("mapping fabricated publication timestamps")
	}
}
