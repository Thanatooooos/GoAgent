package cachemetrics

import "testing"

func TestServiceSnapshotIncludesMaintenanceAndFailureCounters(t *testing.T) {
	metrics := NewService()
	metrics.Record("session_recall", "conversation", "hit")
	metrics.RecordMaintenanceRun(2, 3)
	metrics.RecordMaintenanceFailure()
	metrics.RecordEmbeddingGenerationFailure()
	metrics.RecordEmbeddingPersistFailure()
	metrics.RecordTouchLastUsedFailure()
	metrics.RecordScopeVersionLookupFailure()

	snapshot := metrics.Snapshot()
	if len(snapshot.Events) != 1 || snapshot.Events[0].CacheKind != "session_recall" {
		t.Fatalf("unexpected events snapshot: %+v", snapshot.Events)
	}
	if snapshot.MaintenanceRuns != 1 || snapshot.MaintenanceExpiredCount != 2 || snapshot.MaintenanceDeletedCount != 3 {
		t.Fatalf("unexpected maintenance counters: %+v", snapshot)
	}
	if snapshot.MaintenanceFailures != 1 {
		t.Fatalf("expected maintenance failure counter, got %+v", snapshot)
	}
	if snapshot.EmbeddingGenerationFailures != 1 || snapshot.EmbeddingPersistFailures != 1 {
		t.Fatalf("unexpected embedding failure counters: %+v", snapshot)
	}
	if snapshot.TouchLastUsedFailures != 1 || snapshot.ScopeVersionLookupFailures != 1 {
		t.Fatalf("unexpected fail-open counters: %+v", snapshot)
	}
}
