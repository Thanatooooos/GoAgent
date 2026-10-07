package work_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	"local/rag-project/internal/app/work/domain"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("WORK_TEST_DSN")
	if dsn == "" {
		t.Skip("WORK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal("open isolated work database:", err)
	}
	var name string
	if err := db.Raw(`SELECT current_database()`).Scan(&name).Error; err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(name, "codex_work_") {
		t.Fatal("WORK_TEST_DSN must point to a codex_work_ database")
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() { sqlDB.Close() })
	return db
}
func mutation(id string, revision int) domain.Mutation {
	return domain.Mutation{RequestID: id, ExpectedRevision: revision}
}
func TestManualWorkLifecycleIsolationAndConcurrentVersions(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	s := postgreswork.NewStore(db)
	user := fmt.Sprintf("w%d", os.Getpid())
	key := fmt.Sprintf("run-%d-", time.Now().UnixNano())
	request := domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "技术选型", Description: "验证后形成结论"}
	topic, err := s.CreateTopic(ctx, user, request)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := s.CreateTopic(ctx, user, request)
	if err != nil || duplicate.ID != topic.ID {
		t.Fatalf("topic replay: %+v %v", duplicate, err)
	}
	request.Name = "不同请求"
	if _, err := s.CreateTopic(ctx, user, request); !errors.Is(err, domain.ErrRequestReused) {
		t.Fatalf("changed input accepted: %v", err)
	}
	if _, err := s.GetTopic(ctx, "other-user", topic.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-user topic read: %v", err)
	}
	item, err := s.CreateItem(ctx, user, topic.ID, domain.CreateItem{Mutation: mutation(key+"item", 0), Name: "验证"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"other", 0), Name: "另一主题"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateConversation(ctx, user, other.ID, domain.CreateConversation{Mutation: mutation(key+"foreign", 0), Title: "讨论", ItemID: item.ID}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-topic item: %v", err)
	}
	conversation, err := s.CreateConversation(ctx, user, topic.ID, domain.CreateConversation{Mutation: mutation(key+"conversation", 0), Title: "验证方案", ItemID: item.ID})
	if err != nil {
		t.Fatal(err)
	}
	ordinary, err := postgresrag.NewConversationRepository(db).ListByUserID(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range ordinary {
		if c.ConversationID == conversation.ID {
			t.Fatal("Work leaked into ordinary chat list")
		}
	}
	if err := runtimeadapter.OrdinaryConversationGuard(db)(ctx, user, conversation.ID); err == nil {
		t.Fatal("ordinary runtime accepted Work conversation")
	}
	state, err := s.GetState(ctx, user, topic.ID)
	if err != nil || len(state.Entries) != 1 || state.Entries[0].Kind != "goal" {
		t.Fatalf("initial state: %+v %v", state, err)
	}
	state, err = s.SaveState(ctx, user, topic.ID, domain.SaveState{Mutation: mutation(key+"state", 1), Entries: []domain.StateEntry{{ID: "d1", Kind: "decision", Text: "由用户确认的决定", ItemID: item.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveState(ctx, user, topic.ID, domain.SaveState{Mutation: mutation(key+"stale-state", 1), Entries: state.Entries}); err == nil {
		t.Fatal("stale progress overwrite accepted")
	}
	create := domain.SaveArtifact{Mutation: mutation(key+"doc", 0), Title: "选型说明", ItemID: item.ID, Body: domain.EmptyDocument()}
	doc, err := s.CreateArtifact(ctx, user, topic.ID, create)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.GetArtifact(ctx, user, other.ID, doc.Artifact.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-topic artifact: %v", err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			input := create
			input.Body = domain.EmptyDocument()
			input.Mutation = mutation(fmt.Sprintf("%ssave-%d", key, i), 1)
			input.Body.Root.Content[0].Content = []domain.Node{{Type: "text", Text: fmt.Sprintf("human %d", i)}}
			_, err := s.SaveArtifact(ctx, user, topic.ID, doc.Artifact.ID, input)
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else {
			var conflict *domain.Conflict
			if errors.As(err, &conflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatalf("CAS success=%d conflicts=%d", success, conflicts)
	}
	versions, err := s.ListVersions(ctx, user, topic.ID, doc.Artifact.ID, domain.Page{Limit: 20})
	if err != nil || len(versions) != 2 {
		t.Fatalf("versions: %d %v", len(versions), err)
	}
	restore := domain.RestoreArtifact{Mutation: mutation(key+"restore", 2), Revision: 1}
	restored, err := s.RestoreArtifact(ctx, user, topic.ID, doc.Artifact.ID, restore)
	if err != nil || restored.Version.Revision != 3 || restored.Version.RestoredFrom != 1 {
		t.Fatalf("restore: %+v %v", restored, err)
	}
	if _, err := s.RestoreArtifact(ctx, user, topic.ID, doc.Artifact.ID, restore); err != nil {
		t.Fatal("restore replay", err)
	}
	var count int64
	if err := db.Raw(`SELECT COUNT(*) FROM t_work_artifact_version WHERE artifact_id=?`, doc.Artifact.ID).Scan(&count).Error; err != nil || count != 3 {
		t.Fatalf("duplicate restore created versions: %d %v", count, err)
	}
	reopened := postgreswork.NewStore(db.Session(&gorm.Session{NewDB: true}))
	persisted, err := reopened.GetArtifact(ctx, user, topic.ID, doc.Artifact.ID)
	if err != nil || persisted.Version.Revision != 3 {
		t.Fatalf("fresh repository read: %+v %v", persisted, err)
	}
	_, err = s.UpdateTopic(ctx, user, topic.ID, domain.UpdateTopic{Mutation: mutation(key+"archive", 1), Name: topic.Name, Description: topic.Description, Status: "archived"})
	if err != nil {
		t.Fatal(err)
	}
	create.Mutation = mutation(key+"archived-doc", 0)
	if _, err := s.CreateArtifact(ctx, user, topic.ID, create); !errors.Is(err, domain.ErrArchived) {
		t.Fatalf("write to archived topic: %v", err)
	}
	if _, err := s.GetArtifact(ctx, user, topic.ID, doc.Artifact.ID); err != nil {
		t.Fatal("archived read", err)
	}
	if _, err := s.UpdateTopic(ctx, user, topic.ID, domain.UpdateTopic{Mutation: mutation(key+"unarchive", 2), Name: topic.Name, Description: topic.Description, Status: "active"}); err != nil {
		t.Fatal(err)
	}
}
