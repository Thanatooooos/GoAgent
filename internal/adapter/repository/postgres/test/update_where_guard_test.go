package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

func TestUpdateWhereRequiresConditions(t *testing.T) {
	t.Parallel()

	now := time.Now()
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "knowledge base",
			run: func() error {
				_, err := postgresknowledge.NewKnowledgeBaseRepository(nil).UpdateWhere(context.Background(), port.KnowledgeBaseConditions{}, port.KnowledgeBasePatch{
					Name: port.ValueOf("kb"),
				})
				return err
			},
		},
		{
			name: "knowledge document",
			run: func() error {
				_, err := postgresknowledge.NewKnowledgeDocumentRepository(nil, nil).UpdateWhere(context.Background(), port.KnowledgeDocumentConditions{}, port.KnowledgeDocumentPatch{
					Status: port.ValueOf(domain.KnowledgeDocumentStatusRunning),
				})
				return err
			},
		},
		{
			name: "knowledge document schedule",
			run: func() error {
				_, err := postgresknowledge.NewKnowledgeDocumentScheduleRepository(nil).UpdateWhere(context.Background(), port.KnowledgeDocumentScheduleConditions{}, port.KnowledgeDocumentSchedulePatch{
					UpdatedAt: port.ValueOf(now),
				})
				return err
			},
		},
		{
			name: "knowledge document schedule exec",
			run: func() error {
				_, err := postgresknowledge.NewKnowledgeDocumentScheduleExecRepository(nil).UpdateWhere(context.Background(), port.KnowledgeDocumentScheduleExecConditions{}, port.KnowledgeDocumentScheduleExecPatch{
					Status: port.ValueOf(domain.KnowledgeDocumentScheduleRunStatusRunning),
				})
				return err
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.run()
			if err == nil {
				t.Fatal("UpdateWhere should reject non-empty patches without conditions")
			}
			if !strings.Contains(err.Error(), "conditions are required") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
