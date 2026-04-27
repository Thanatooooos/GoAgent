package postgres

import (
	"testing"
	"time"

	"local/rag-project/internal/adapter/repository/postgres/models"
	"local/rag-project/internal/app/knowledge/domain"
)

func TestKnowledgeDocumentScheduleMappingRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now()
	schedule := domain.KnowledgeDocumentSchedule{
		ID:              "s-1",
		DocumentID:      "d-1",
		KnowledgeBaseID: "kb-1",
		CronExpr:        "0 */5 * * * *",
		Enabled:         true,
		NextRunTime:     &now,
		LastRunTime:     &now,
		LastSuccessTime: &now,
		LastStatus:      domain.KnowledgeDocumentScheduleRunStatusSuccess,
		LastError:       "none",
		LastETag:        "etag-1",
		LastModified:    "Wed, 21 Oct 2015 07:28:00 GMT",
		LastContentHash: "hash-1",
		LockOwner:       "owner-1",
		LockUntil:       &now,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	model := toKnowledgeDocumentScheduleModel(schedule)
	roundTrip := toKnowledgeDocumentScheduleDomain(model)
	if roundTrip.ID != schedule.ID || roundTrip.DocumentID != schedule.DocumentID || !roundTrip.Enabled {
		t.Fatalf("unexpected schedule round trip result: %+v", roundTrip)
	}
	if roundTrip.LockOwner != "owner-1" || roundTrip.LastStatus != domain.KnowledgeDocumentScheduleRunStatusSuccess {
		t.Fatalf("unexpected schedule metadata after round trip: %+v", roundTrip)
	}
}

func TestKnowledgeDocumentScheduleExecMappingRoundTrip(t *testing.T) {
	t.Parallel()

	now := time.Now()
	size := int64(42)
	exec := domain.KnowledgeDocumentScheduleExec{
		ID:              "e-1",
		ScheduleID:      "s-1",
		DocumentID:      "d-1",
		KnowledgeBaseID: "kb-1",
		Status:          domain.KnowledgeDocumentScheduleRunStatusSkipped,
		Message:         "unchanged",
		StartTime:       &now,
		EndTime:         &now,
		FileName:        "demo.md",
		FileSize:        &size,
		ContentHash:     "hash-1",
		ETag:            "etag-1",
		LastModified:    "Wed, 21 Oct 2015 07:28:00 GMT",
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	model := toKnowledgeDocumentScheduleExecModel(exec)
	roundTrip := toKnowledgeDocumentScheduleExecDomain(model)
	if roundTrip.ID != exec.ID || roundTrip.ScheduleID != exec.ScheduleID || roundTrip.Status != exec.Status {
		t.Fatalf("unexpected schedule exec round trip result: %+v", roundTrip)
	}
	if roundTrip.FileSize == nil || *roundTrip.FileSize != size {
		t.Fatalf("unexpected schedule exec file size after round trip: %+v", roundTrip.FileSize)
	}
}

func TestKnowledgeDocumentScheduleModelTableNames(t *testing.T) {
	t.Parallel()

	if got := (models.KnowledgeDocumentScheduleModel{}).TableName(); got != "t_knowledge_document_schedule" {
		t.Fatalf("unexpected schedule table name: %q", got)
	}
	if got := (models.KnowledgeDocumentScheduleExecModel{}).TableName(); got != "t_knowledge_document_schedule_exec" {
		t.Fatalf("unexpected schedule exec table name: %q", got)
	}
}
