package scheduledtask_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	storepkg "local/rag-project/internal/adapter/repository/postgres/scheduledtask"
	ragdomain "local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/app/scheduledtask/domain"
)

// TestPendingDraftListingStaysWithinUserAndConversation covers the reload path:
// a chat page that lost its tool events asks which drafts its conversation is
// still waiting on, and must never see anyone else's.
func TestPendingDraftListingStaysWithinUserAndConversation(t *testing.T) {
	dsn := os.Getenv("SCHEDULED_TASK_TEST_DSN")
	if dsn == "" {
		t.Skip("SCHEDULED_TASK_TEST_DSN is not set")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := postgresrepo.RunMigrations(db); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	store := storepkg.NewStore(db)
	conversations := postgresrag.NewConversationRepository(db)
	now := time.Now().UTC().Truncate(time.Second)
	suffix := time.Now().UnixNano() % 100000000
	owner := fmt.Sprintf("u%d", suffix)
	stranger := fmt.Sprintf("u%d", suffix+1)
	first := newDraftConversation(t, ctx, conversations, owner, suffix)
	second := newDraftConversation(t, ctx, conversations, owner, suffix+1)
	other := newDraftConversation(t, ctx, conversations, stranger, suffix+2)
	config := storepkg.ProposedConfig{
		Name: "官网公告追踪", Prompt: "Report the confirmed event",
		Schedule:   domain.Schedule{Kind: domain.ScheduleOnce, Timezone: "UTC", At: now.Add(time.Hour)},
		ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone,
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM t_scheduled_task_draft WHERE user_id IN ?`, []string{owner, stranger})
		db.Exec(`DELETE FROM t_scheduled_task_version WHERE task_id IN (SELECT id FROM t_scheduled_task WHERE user_id IN ?)`,
			[]string{owner, stranger})
		db.Exec(`DELETE FROM t_scheduled_task WHERE user_id IN ?`, []string{owner, stranger})
		db.Exec(`DELETE FROM t_conversation WHERE user_id IN ?`, []string{owner, stranger})
	})

	draft, err := store.CreateDraft(ctx, owner, first, "", 0, config, now)
	if err != nil {
		t.Fatal(err)
	}
	pending, err := store.ListPendingDrafts(ctx, owner, first, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 || pending[0].ID != draft.ID {
		t.Fatalf("pending draft not returned: %+v", pending)
	}
	if pending[0].Config.Prompt != config.Prompt || pending[0].Config.Schedule.At.IsZero() || !pending[0].ExpiresAt.After(now) {
		t.Fatalf("pending draft lost its saved preview: %+v", pending[0])
	}

	// Confirmation consumes the preview: the reload path must not offer it again.
	task, _, err := store.ConfirmDraft(ctx, owner, draft.ID, now, false)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err = store.ListPendingDrafts(ctx, owner, first, now); err != nil || len(pending) != 0 {
		t.Fatalf("confirmed draft is still pending: %v, %+v", err, pending)
	}

	// A later preview of the same prompt is flagged exactly like the live one.
	duplicate, err := store.CreateDraft(ctx, owner, first, "", 0, config, now)
	if err != nil {
		t.Fatal(err)
	}
	if pending, err = store.ListPendingDrafts(ctx, owner, first, now); err != nil || len(pending) != 1 ||
		pending[0].ID != duplicate.ID || pending[0].DuplicateTaskID != task.ID {
		t.Fatalf("restored draft lost its duplicate warning: %v, %+v", err, pending)
	}

	// Drafts belong to the conversation that produced them.
	if _, err := store.CreateDraft(ctx, owner, second, "", 0, config, now); err != nil {
		t.Fatal(err)
	}
	if _, err := store.CreateDraft(ctx, stranger, other, "", 0, config, now); err != nil {
		t.Fatal(err)
	}
	if pending, err = store.ListPendingDrafts(ctx, owner, first, now); err != nil || len(pending) != 1 {
		t.Fatalf("listing leaked drafts of other conversations: %v, %+v", err, pending)
	}
	if pending, err = store.ListPendingDrafts(ctx, owner, second, now); err != nil || len(pending) != 1 {
		t.Fatalf("second conversation lost its draft: %v, %+v", err, pending)
	}
	if _, err := store.ListPendingDrafts(ctx, owner, other, now); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("another user's conversation was readable: %v", err)
	}
	if _, err := store.ListPendingDrafts(ctx, owner, "conversation-that-does-not-exist", now); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("unknown conversation was readable: %v", err)
	}

	// Expiry is enforced on read, not only on confirmation.
	expired := storepkg.ProposedConfig{
		Name: "过期草稿", Prompt: "Report the expired event",
		Schedule:   domain.Schedule{Kind: domain.ScheduleOnce, Timezone: "UTC", At: now.Add(time.Hour)},
		ReportMode: domain.ReportAlways, ConditionKind: domain.ConditionNone,
	}
	if _, err := store.CreateDraft(ctx, owner, second, "", 0, expired, now.Add(-25*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if pending, err = store.ListPendingDrafts(ctx, owner, second, now); err != nil || len(pending) != 1 {
		t.Fatalf("expired draft is still offered: %v, %+v", err, pending)
	}

	// Edit previews keep their task and base version, otherwise a restored card
	// would silently edit the wrong version.
	edited := config
	edited.Prompt = "Report the revised event"
	edit, err := store.CreateDraft(ctx, owner, first, task.ID, task.CurrentVersion, edited, now)
	if err != nil {
		t.Fatal(err)
	}
	pending, err = store.ListPendingDrafts(ctx, owner, first, now)
	if err != nil {
		t.Fatal(err)
	}
	var restored *storepkg.Draft
	for i := range pending {
		if pending[i].ID == edit.ID {
			restored = &pending[i]
		}
	}
	if restored == nil || restored.TaskID != task.ID || restored.BaseVersion != task.CurrentVersion || restored.DuplicateTaskID != "" {
		t.Fatalf("edit draft lost its base version: %+v", pending)
	}
}

func newDraftConversation(t *testing.T, ctx context.Context, repo *postgresrag.ConversationRepository, userID string, suffix int64) string {
	t.Helper()
	id := fmt.Sprintf("c%d", suffix)
	if _, err := repo.Create(ctx, ragdomain.Conversation{
		ID: id, ConversationID: id, UserID: userID, Title: "draft listing",
		CreateTime: time.Now().UTC(), UpdateTime: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	return id
}
