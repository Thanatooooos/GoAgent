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
	ragservice "local/rag-project/internal/app/rag/service"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/distributedid"
	"local/rag-project/internal/framework/stream"
)

func TestExecutionRecoveryLoopStartupPeriodicAndClose(t *testing.T) {
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
	ctx := context.Background()
	store := postgresruntime.NewStore(db)
	convRepo := postgresrag.NewConversationRepository(db)
	msgRepo := postgresrag.NewConversationMessageRepository(db)
	sumRepo := postgresrag.NewConversationSummaryRepository(db)
	conv := ragservice.NewConversationService(convRepo, msgRepo, sumRepo, nil, nil, 30, postgresrag.NewConversationDeleteTransaction(db))
	messages := ragservice.NewConversationMessageService(convRepo, msgRepo, sumRepo, nil)
	cache := stream.NewMemoryStreamManager()
	ids := []string{}
	for i := 0; i < 2; i++ {
		n, err := distributedid.NextID()
		if err != nil {
			t.Fatal(err)
		}
		id := strconv.FormatInt(n, 10)
		ids = append(ids, id)
		if _, err := conv.CreateOrUpdate(ctx, ragservice.CreateOrUpdateConversationInput{ConversationID: id, UserID: id, Question: "execution fixture"}); err != nil {
			t.Fatal(err)
		}
		user, err := messages.AddMessage(ctx, ragservice.AddConversationMessageInput{ConversationID: id, UserID: id, Role: convention.UserRole, Content: "execution fixture"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := store.CreateOrLoadSession(ctx, persistence.Session{ExecutionManaged: true, ID: id, TraceID: id, ConversationID: id, UserID: id, UserMessageID: user.ID, Status: persistence.StatusRunning, NextSequence: 1, CreatedAt: time.Now(), UpdatedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
		raghttp.SendRuntimeMeta(cache, id, id)
	}
	expire := func(id string) {
		t.Helper()
		if err := db.Exec(`UPDATE t_runtime_chat_execution SET lease_until=clock_timestamp()-INTERVAL '1 second' WHERE task_id=?`, id).Error; err != nil {
			t.Fatal(err)
		}
	}
	wait := func(id string, deadline time.Time) {
		t.Helper()
		for {
			events, _, err := cache.GetEvents(ctx, id, 0)
			if err != nil {
				t.Fatal(err)
			}
			if len(events) > 0 && events[len(events)-1].Done {
				if len(events) != 3 || events[1].Name != "error" || !strings.Contains(string(events[1].Data), `"status":"interrupted"`) {
					t.Fatalf("startup stream outcome: %+v", events)
				}
				return
			}
			if time.Now().After(deadline) {
				t.Fatal("execution recovery did not run")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	expire(ids[0])
	runtime := &Runtime{DB: db, StreamManager: cache}
	runtime.startExecutionRecovery(store)
	t.Cleanup(func() { runtime.Close() })
	wait(ids[0], time.Now().Add(3*time.Second))
	if events, _, err := cache.GetEvents(ctx, ids[1], 0); err != nil || len(events) != 1 {
		t.Fatal("live acceptance interrupted", err)
	}
	expire(ids[1])
	wait(ids[1], time.Now().Add(7*time.Second))
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	if runtime.executionCancel != nil {
		t.Fatal("execution recovery did not stop")
	}
}
