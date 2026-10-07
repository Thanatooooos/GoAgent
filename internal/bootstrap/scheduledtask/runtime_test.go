package scheduledtask

import (
	"context"
	"testing"

	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	"local/rag-project/internal/app/scheduledtask/domain"
	"local/rag-project/internal/app/scheduledtask/service"
)

type revokedAccess struct{}

func (revokedAccess) AccessibleIDs(context.Context, string) ([]string, error) { return nil, nil }

func TestPublicationRechecksKnowledgeAccess(t *testing.T) {
	r := &Runtime{Executor: service.Executor{Access: revokedAccess{}}}
	claim := storepkg.Claim{Task: domain.Task{ID: "task", UserID: "owner"}, Version: domain.Version{KnowledgeBaseIDs: []string{"revoked-kb"}}}
	if err := r.publish(context.Background(), claim, domain.Outcome{Signal: domain.SignalReport, Body: "Private knowledge"}); err == nil {
		t.Fatal("a report with revoked knowledge access must not reach the publisher")
	}
}
