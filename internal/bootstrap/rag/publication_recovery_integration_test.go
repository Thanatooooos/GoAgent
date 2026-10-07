package rag

import (
	"context"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	raghttp "local/rag-project/internal/adapter/http/rag"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	ragservice "local/rag-project/internal/app/rag/service"
	convruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/stream"
)

func TestPublicationRecoveryLoopRunsOnStartupAndCloses(t *testing.T) {
	dsn := os.Getenv("WORK_TEST_DSN")
	if dsn == "" {
		t.Skip("WORK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	var name string
	if err := db.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "codex_work_") {
		t.Fatal("dedicated codex_work_ database required")
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ids := func() (string, error) { n, err := distributedid.NextID(); return strconv.FormatInt(n, 10), err }
	id, err := ids()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	convRepo := postgresrag.NewConversationRepository(db)
	msgRepo := postgresrag.NewConversationMessageRepository(db)
	sumRepo := postgresrag.NewConversationSummaryRepository(db)
	conv := ragservice.NewConversationService(convRepo, msgRepo, sumRepo, nil, nil, 30, postgresrag.NewConversationDeleteTransaction(db))
	if _, err := conv.CreateOrUpdate(ctx, ragservice.CreateOrUpdateConversationInput{ConversationID: id, UserID: id, Question: "worker fixture"}); err != nil {
		t.Fatal(err)
	}
	messages := ragservice.NewConversationMessageService(convRepo, msgRepo, sumRepo, nil)
	user, err := messages.AddMessage(ctx, ragservice.AddConversationMessageInput{ConversationID: id, UserID: id, Role: convention.UserRole, Content: "worker fixture"})
	if err != nil {
		t.Fatal(err)
	}
	lifecycle := convruntime.NewLifecycle(postgresruntime.NewStore(db), ids)
	session, err := lifecycle.StartSession(ctx, convruntime.RunRequest{ConversationID: id, UserID: id, UserMessageID: user.ID, TraceID: id, Question: "worker fixture"})
	if err != nil {
		t.Fatal(err)
	}
	if err := lifecycle.ReadyAnswer(ctx, session, `recovered <web title="worker" url="https://example.com/worker" />`); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`UPDATE t_runtime_chat_publication SET next_attempt_at='2000-01-01' WHERE runtime_session_id=?`, session.ID).Error; err != nil {
		t.Fatal(err)
	}
	cache := stream.NewMemoryStreamManager()
	raghttp.SendRuntimeMeta(cache, id, id)
	runtime := &Runtime{DB: db, StreamManager: cache}
	runtime.startPublicationRecovery(postgresruntime.NewChatPublisher(db, runtimeadapter.NewConversationMessages(messages)))
	t.Cleanup(func() { runtime.Close() })
	deadline := time.Now().Add(3 * time.Second)
	for {
		events, _, err := cache.GetEvents(ctx, id, 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) > 0 && events[len(events)-1].Done {
			found := false
			for _, event := range events {
				if event.Name == "finish" && strings.Contains(string(event.Data), "https://example.com/worker") {
					found = true
				}
			}
			if !found {
				t.Fatalf("worker did not emit finish/sources: %+v", events)
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("startup did not recover pending publication")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.publicationCancel != nil {
		t.Fatal("recovery loop did not release cancellation handle")
	}
}
