package rag

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"local/rag-project/internal/app/rag/port"
)

func TestConversationMessageRepositoryListOrdersTiesByID(t *testing.T) {
	for _, test := range []struct {
		name  string
		order port.ConversationMessageOrder
		want  string
	}{
		{name: "ascending", want: "order by create_time asc,id asc"},
		{name: "descending", order: port.ConversationMessageOrderDesc, want: "order by create_time desc,id desc"},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &gormTraceRecorder{Interface: logger.Default.LogMode(logger.Info)}
			db, err := gorm.Open(postgres.New(postgres.Config{
				DSN: "host=localhost user=test password=test dbname=test sslmode=disable",
			}), &gorm.Config{
				DryRun:               true,
				DisableAutomaticPing: true,
				Logger:               recorder,
			})
			if err != nil {
				t.Fatalf("open gorm db: %v", err)
			}

			_, err = NewConversationMessageRepository(db).List(context.Background(), port.ConversationMessageListFilter{Order: test.order})
			if err != nil {
				t.Fatalf("List returned error: %v", err)
			}
			if !strings.Contains(strings.ToLower(recorder.lastSQL), test.want) {
				t.Fatalf("expected %q in SQL, got %q", test.want, recorder.lastSQL)
			}
		})
	}
}
