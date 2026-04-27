package test

import (
	"testing"
	"time"

	"local/rag-project/internal/app/knowledge/domain"
)

func TestNewKnowledgeDocumentScheduleDefaultsEnabled(t *testing.T) {
	t.Parallel()

	schedule := domain.NewKnowledgeDocumentSchedule("s-1", "d-1", "kb-1", "0 */5 * * * *")
	if !schedule.Enabled {
		t.Fatal("new schedule should be enabled by default")
	}
	if schedule.ID != "s-1" || schedule.DocumentID != "d-1" || schedule.KnowledgeBaseID != "kb-1" {
		t.Fatalf("unexpected schedule identity: %+v", schedule)
	}
}

func TestKnowledgeDocumentScheduleExecDefaultsToRunning(t *testing.T) {
	t.Parallel()

	start := time.Now()
	exec := domain.NewKnowledgeDocumentScheduleExec("e-1", "s-1", "d-1", "kb-1", start)
	if exec.Status != domain.KnowledgeDocumentScheduleRunStatusRunning {
		t.Fatalf("unexpected default exec status: %q", exec.Status)
	}
	if exec.StartTime == nil || !exec.StartTime.Equal(start) {
		t.Fatalf("unexpected start time: %+v", exec.StartTime)
	}
	if exec.IsFinished() {
		t.Fatal("running exec should not be finished")
	}
}

func TestKnowledgeDocumentScheduleExecFinishedStates(t *testing.T) {
	t.Parallel()

	states := []string{
		domain.KnowledgeDocumentScheduleRunStatusSuccess,
		domain.KnowledgeDocumentScheduleRunStatusFailed,
		domain.KnowledgeDocumentScheduleRunStatusSkipped,
	}
	for _, status := range states {
		exec := domain.KnowledgeDocumentScheduleExec{Status: status}
		if !exec.IsFinished() {
			t.Fatalf("status %q should be treated as finished", status)
		}
	}
}
