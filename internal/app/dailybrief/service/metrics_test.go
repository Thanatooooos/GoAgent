package service

import (
	"fmt"
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestMetricsServiceAggregatesRunAndSourceMetrics(t *testing.T) {
	t.Parallel()

	metrics := NewMetricsService()
	metrics.RecordSubscribedUsersScanned(3)
	metrics.RecordRetryAttempt()
	metrics.RecordSourceCollect(SourceCollectResult{
		Candidates: []domain.Candidate{
			{Source: domain.SourceKeyHackerNews},
			{Source: domain.SourceKeyOpenAIBlog},
		},
		Failures: map[string]error{
			domain.SourceKeyMetaAIBlog: fmt.Errorf("timeout"),
		},
	})
	metrics.RecordPipelineCounts(2, 1)
	metrics.RecordFinalItemCount(4)
	metrics.RecordRunStatus(domain.GenerationRunStatusDegraded)

	snapshot := metrics.Snapshot()
	if snapshot.SubscribedUsersScanned != 3 {
		t.Fatalf("unexpected scanned users: %d", snapshot.SubscribedUsersScanned)
	}
	if snapshot.RetryAttempts != 1 || snapshot.DegradedRuns != 1 {
		t.Fatalf("unexpected retry/degraded totals: %+v", snapshot)
	}
	if snapshot.CandidateCountBefore != 2 || snapshot.CandidateCountAfter != 1 || snapshot.FinalItemCount != 4 {
		t.Fatalf("unexpected pipeline totals: %+v", snapshot)
	}
	if len(snapshot.Sources) != 3 {
		t.Fatalf("expected 3 source rows, got %+v", snapshot.Sources)
	}
}

func TestMetricsServiceSnapshotIsSortedBySource(t *testing.T) {
	t.Parallel()

	metrics := NewMetricsService()
	metrics.RecordSourceCollect(SourceCollectResult{
		Candidates: []domain.Candidate{{Source: "zebra"}, {Source: "alpha"}},
	})
	snapshot := metrics.Snapshot()
	if len(snapshot.Sources) != 2 || snapshot.Sources[0].Source != "alpha" {
		t.Fatalf("expected sorted sources, got %+v", snapshot.Sources)
	}
}
