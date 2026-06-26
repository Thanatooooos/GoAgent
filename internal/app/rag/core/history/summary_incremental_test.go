package history

import (
	"context"
	"testing"
	"time"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/rag/port"
	"local/rag-project/internal/framework/convention"
)

func TestCompressionLoadsOnlyUncoveredMessages(t *testing.T) {
	messageRepo := &incrementalMessageRepo{
		messages: []domain.ConversationMessage{
			{ID: "11", ConversationID: "c1", UserID: "u1", Role: "user", Content: "new question"},
			{ID: "12", ConversationID: "c1", UserID: "u1", Role: "assistant", Content: "new answer"},
		},
	}
	summaryRepo := &incrementalSummaryRepo{
		latest: domain.ConversationSummary{
			ID:                    "s1",
			ConversationID:        "c1",
			UserID:                "u1",
			Content:               "目标：old",
			StructuredSummaryJSON: `{"schema_version":1,"goal":"old"}`,
			LastMessageID:         "10",
			CoveredToMessageID:    "10",
		},
	}
	engine := summaryCompressionEngine{
		summaryRepo:   summaryRepo,
		messageRepo:   messageRepo,
		chatService:   &mockChatServiceForCompress{response: `{"schema_version":1,"goal":"answer the new question","recent_progress":["new answer"]}`},
		triggerTokens: 1,
		estimator:     tokenbudget.FixedEstimator(100),
		maxChars:      200,
		now:           time.Now,
	}

	err := engine.runConversationSummaryCompression(context.Background(), SummaryJobInput{
		ConversationID:  "c1",
		UserID:          "u1",
		TargetMessageID: "12",
	})
	if err != nil {
		t.Fatalf("runConversationSummaryCompression() error = %v", err)
	}
	if messageRepo.filter.AfterID != "10" || messageRepo.filter.ThroughID != "12" {
		t.Fatalf("message filter = %+v, want AfterID=10 ThroughID=12", messageRepo.filter)
	}
	if summaryRepo.created.CoveredFromMessageID != "11" || summaryRepo.created.CoveredToMessageID != "12" {
		t.Fatalf("created summary coverage = %q..%q, want 11..12",
			summaryRepo.created.CoveredFromMessageID,
			summaryRepo.created.CoveredToMessageID,
		)
	}
	if !summaryRepo.advancingWriteCalled {
		t.Fatal("expected coverage-advancing summary write")
	}
}

type incrementalMessageRepo struct {
	messages []domain.ConversationMessage
	filter   port.ConversationMessageListFilter
}

func (r *incrementalMessageRepo) Create(_ context.Context, message domain.ConversationMessage) (domain.ConversationMessage, error) {
	return message, nil
}

func (r *incrementalMessageRepo) GetByID(context.Context, string) (domain.ConversationMessage, error) {
	return domain.ConversationMessage{}, nil
}

func (r *incrementalMessageRepo) List(_ context.Context, filter port.ConversationMessageListFilter) ([]domain.ConversationMessage, error) {
	r.filter = filter
	return append([]domain.ConversationMessage(nil), r.messages...), nil
}

func (r *incrementalMessageRepo) CountByConversationIDAndUserIDAndRole(context.Context, string, string, string) (int64, error) {
	return 0, nil
}

func (r *incrementalMessageRepo) FindMaxIDAtOrBefore(context.Context, string, string, time.Time) (string, error) {
	return "", nil
}

func (r *incrementalMessageRepo) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

type incrementalSummaryRepo struct {
	latest               domain.ConversationSummary
	created              domain.ConversationSummary
	advancingWriteCalled bool
}

func (r *incrementalSummaryRepo) Create(_ context.Context, summary domain.ConversationSummary) (domain.ConversationSummary, error) {
	r.created = summary
	return summary, nil
}

func (r *incrementalSummaryRepo) CreateIfCoverageAdvances(_ context.Context, summary domain.ConversationSummary) (bool, error) {
	r.advancingWriteCalled = true
	r.created = summary
	return true, nil
}

func (r *incrementalSummaryRepo) FindLatestByConversationIDAndUserID(context.Context, string, string) (domain.ConversationSummary, error) {
	return r.latest, nil
}

func (r *incrementalSummaryRepo) DeleteByConversationIDAndUserID(context.Context, string, string) error {
	return nil
}

var _ port.ConversationMessageRepository = (*incrementalMessageRepo)(nil)
var _ port.ConversationSummaryRepository = (*incrementalSummaryRepo)(nil)
var _ = convention.UserRole
