package rag

import (
	"testing"
	"time"

	"local/rag-project/internal/adapter/repository/postgres/rag/models"
	"local/rag-project/internal/app/rag/domain"
)

func TestConversationSummaryMapperRoundTrip(t *testing.T) {
	now := time.Date(2026, 6, 9, 12, 0, 0, 0, time.UTC)
	input := domain.ConversationSummary{
		ID:                   "1",
		ConversationID:       "c1",
		UserID:               "u1",
		Content:              "摘要",
		LastMessageID:        "m9",
		SummaryVersion:       domain.SummaryVersionV1,
		CoveredFromMessageID: "m1",
		CoveredToMessageID:   "m9",
		SourceMessageCount:   9,
		QualityStatus:        domain.SummaryQualityUnchecked,
		LastRebuildReason:    "threshold_reached",
		CreateTime:           now,
		UpdateTime:           now,
	}

	model := toConversationSummaryModel(input)
	if model.SummaryVersion != domain.SummaryVersionV1 {
		t.Fatalf("unexpected model summary version: %d", model.SummaryVersion)
	}
	if model.CoveredFromMessageID != "m1" || model.CoveredToMessageID != "m9" {
		t.Fatalf("unexpected covered range: %#v", model)
	}

	output := toConversationSummaryDomain(model)
	if output != input {
		t.Fatalf("round trip mismatch:\nwant %#v\ngot  %#v", input, output)
	}
}

func TestConversationSummaryMapperDefaultsEmptyLifecycleFields(t *testing.T) {
	now := time.Now()
	model := toConversationSummaryModel(domain.ConversationSummary{
		ID:             "1",
		ConversationID: "c1",
		UserID:         "u1",
		Content:        "legacy",
		LastMessageID:  "m1",
		CreateTime:     now,
		UpdateTime:     now,
	})
	if model.SummaryVersion != domain.SummaryVersionV1 {
		t.Fatalf("expected default summary version, got %d", model.SummaryVersion)
	}
	if model.QualityStatus != domain.SummaryQualityUnchecked {
		t.Fatalf("expected default quality status, got %q", model.QualityStatus)
	}

	domainItem := toConversationSummaryDomain(models.ConversationSummaryModel{
		ID:             "1",
		ConversationID: "c1",
		UserID:         "u1",
		LastMessageID:  "m1",
		Content:        "legacy",
		SummaryVersion: domain.SummaryVersionV1,
		QualityStatus:  domain.SummaryQualityUnchecked,
		CreateTime:     now,
		UpdateTime:     now,
	})
	if domainItem.SummaryVersion != domain.SummaryVersionV1 || domainItem.QualityStatus != domain.SummaryQualityUnchecked {
		t.Fatalf("expected lifecycle fields from model, got %#v", domainItem)
	}
}
