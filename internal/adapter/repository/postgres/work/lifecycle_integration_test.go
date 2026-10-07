package work_test

import (
	"context"
	"errors"
	"io"
	"strconv"
	"testing"
	"time"

	postgresrag "local/rag-project/internal/adapter/repository/postgres/rag"
	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	ragdomain "local/rag-project/internal/app/rag/domain"
	ragport "local/rag-project/internal/app/rag/port"
	conversationservice "local/rag-project/internal/app/rag/service/conversation"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
	knowledgebootstrap "local/rag-project/internal/bootstrap/knowledge"
	workbootstrap "local/rag-project/internal/bootstrap/work"
	"local/rag-project/internal/framework/distributedid"
)

func TestProfileObservationStillClaimsOrdinaryConversationsAndExcludesWork(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	user, key := testIdentity(t)
	s := postgreswork.NewStore(db)
	topic, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "画像隔离"})
	if err != nil {
		t.Fatal(err)
	}
	conversation, err := s.CreateConversation(ctx, user, topic.ID, domain.CreateConversation{Mutation: mutation(key+"work", 0), Title: "专题对话"})
	if err != nil {
		t.Fatal(err)
	}
	observer := postgresrag.NewConversationProfileStateRepository(db)
	due := time.Now().Add(24 * time.Hour)
	ordinaryID, err := distributedid.NextID()
	if err != nil {
		t.Fatal(err)
	}
	ordinary := strconv.FormatInt(ordinaryID, 10)
	if _, err = postgresrag.NewConversationRepository(db).Create(ctx, ragdomain.Conversation{ID: ordinary, ConversationID: ordinary, UserID: user, Title: "普通对话", CreateTime: time.Now(), UpdateTime: time.Now()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Exec(`DELETE FROM t_conversation_profile_state WHERE conversation_id IN (?,?)`, conversation.ID, ordinary)
	})
	if err = observer.Defer(ctx, conversation.ID, user, due); err != nil {
		t.Fatal(err)
	}
	if err = observer.Defer(ctx, ordinary, user, due); err != nil {
		t.Fatal(err)
	}
	claimed, err := observer.ClaimDue(ctx, due.Add(time.Second), 1000)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, row := range claimed {
		if row.ConversationID == conversation.ID {
			t.Fatal("Work entered automatic profile observation")
		}
		if row.ConversationID == ordinary {
			found = true
		}
	}
	if !found {
		t.Fatal("ordinary observation was no longer claimable")
	}
}

func TestDeleteConversationPreservesWorkAndRetriesAttachmentCleanup(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	store := postgreswork.NewStore(db)
	user, key := testIdentity(t)
	topic, err := store.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "生命周期"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: key + "turn", Question: "讨论", Action: "discuss"})
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := store.CreateArtifact(ctx, user, topic.ID, domain.SaveArtifact{Mutation: mutation(key+"doc", 0), Title: "独立文档", Body: domain.EmptyDocument()})
	if err != nil {
		t.Fatal(err)
	}
	state, err := store.SaveState(ctx, user, topic.ID, domain.SaveState{Mutation: mutation(key+"state", 1), Entries: []domain.StateEntry{{ID: "decided", Kind: "decision", Text: "已确认", References: []domain.Reference{{Kind: "message", ID: turn.UserMessageID}}}}})
	if err != nil {
		t.Fatal(err)
	}
	local, err := store.ReserveSource(ctx, user, topic.ID, "test", domain.ReserveSource{Mutation: mutation(key+"local", 0), Name: "本次附件", SourceType: "file", ConversationID: turn.ConversationID})
	if err != nil {
		t.Fatal(err)
	}
	promoted, err := store.ReserveSource(ctx, user, topic.ID, "test", domain.ReserveSource{Mutation: mutation(key+"promoted", 0), Name: "保留资料", SourceType: "file", ConversationID: turn.ConversationID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ChangeSource(ctx, user, topic.ID, promoted.ID, true, mutation(key+"promote", 0)); err != nil {
		t.Fatal(err)
	}
	deletion := postgresrag.NewConversationDeleteTransaction(db)
	if err = deletion(ctx, user, turn.ConversationID, func(context.Context, ragport.ConversationRepository, ragport.ConversationMessageRepository, ragport.ConversationSummaryRepository) error {
		return errors.New("injected downstream failure")
	}); err == nil {
		t.Fatal("missing rollback error")
	}
	if _, err = store.GetSource(ctx, user, topic.ID, local.ID, turn.ConversationID); err != nil {
		t.Fatal("failed deletion leaked cleanup state", err)
	}
	service := conversationservice.NewConversationService(postgresrag.NewConversationRepository(db), postgresrag.NewConversationMessageRepository(db), postgresrag.NewConversationSummaryRepository(db), nil, nil, 30, deletion)
	if err = service.Delete(ctx, conversationservice.DeleteConversationInput{UserID: user, ConversationID: turn.ConversationID}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetArtifact(ctx, user, topic.ID, artifact.Artifact.ID); err != nil {
		t.Fatal("document deleted", err)
	}
	after, err := store.GetState(ctx, user, topic.ID)
	if err != nil || after.Revision != state.Revision || len(after.Entries) != 1 {
		t.Fatal("confirmed state lost", err)
	}
	if _, err = store.GetMessage(ctx, user, topic.ID, turn.UserMessageID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("deleted basis still readable", err)
	}
	after.Entries[0].Text = "删除讨论后仍可更新确认进展"
	if _, err = store.SaveState(ctx, user, topic.ID, domain.SaveState{Mutation: mutation(key+"state-after-delete", after.Revision), Entries: after.Entries}); err != nil {
		t.Fatal("deleted historical basis blocked future progress edits", err)
	}
	next, err := store.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: key + "next-after-delete", Question: "继续推进", Action: "discuss"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.StartTurn(ctx, user, topic.ID, next.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.SuggestProgress(capability.Context{Context: ctx, UserID: user, ToolCallID: "after-delete", Work: &capability.WorkScope{TopicID: topic.ID, TurnID: next.ID}}, []domain.StateChange{{Kind: "add", Entry: domain.StateEntry{ID: "next-after-delete", Kind: "next", Text: "新讨论提出下一步"}}}); err != nil {
		t.Fatal("deleted historical basis blocked future AI proposals", err)
	}
	if _, err = store.GetSource(ctx, user, topic.ID, local.ID, turn.ConversationID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("attachment accessible before physical cleanup", err)
	}
	if _, err = store.GetSource(ctx, user, topic.ID, promoted.ID, ""); err != nil {
		t.Fatal("promoted source lost", err)
	}
	if _, err = store.ChangeSource(ctx, user, topic.ID, local.ID, true, mutation(key+"late-promote", 0)); err == nil {
		t.Fatal("cleanup can be promoted")
	}
	if _, err = store.GetTurn(ctx, user, topic.ID, turn.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("deleted conversation remains readable through turn metadata", err)
	}
	var stoppedStatus string
	if err = db.Raw(`SELECT status FROM t_work_turn WHERE id=?`, turn.ID).Scan(&stoppedStatus).Error; err != nil || stoppedStatus != "cancelled" {
		t.Fatal("deleted conversation turn was not stopped", err)
	}
	if err = db.Exec(`UPDATE t_work_source SET file_key='test-failure-retry' WHERE id=?`, local.ID).Error; err != nil {
		t.Fatal(err)
	}
	storage := &cleanupStorage{fail: true}
	runtime := workbootstrap.Runtime{Knowledge: &knowledgebootstrap.Runtime{DB: db, Storage: storage}}
	runtime.CleanupOnce(ctx)
	var row struct {
		Status          string
		CleanupAttempts int
	}
	if err = db.Raw(`SELECT status,cleanup_attempts FROM t_work_source WHERE id=?`, local.ID).Scan(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != "cleanup_pending" || row.CleanupAttempts != 1 {
		t.Fatalf("cleanup failure lost retry: %+v", row)
	}
	storage.fail = false
	if err = db.Exec(`UPDATE t_work_source SET next_cleanup_at=CURRENT_TIMESTAMP WHERE id=?`, local.ID).Error; err != nil {
		t.Fatal(err)
	}
	runtime.CleanupOnce(ctx)
	db.Raw(`SELECT status,cleanup_attempts FROM t_work_source WHERE id=?`, local.ID).Scan(&row)
	if row.Status != "cleaned" {
		t.Fatalf("cleanup retry failed: %+v", row)
	}
}

type cleanupStorage struct{ fail bool }

func (s *cleanupStorage) Upload(context.Context, knowledgeport.FileUpload) (knowledgeport.StoredFile, error) {
	return knowledgeport.StoredFile{}, errors.New("unused")
}
func (s *cleanupStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("unused")
}
func (s *cleanupStorage) Delete(context.Context, string) error {
	if s.fail {
		return errors.New("injected storage failure")
	}
	return nil
}

func TestWorkInputUsesLegacyChatWallClock(t *testing.T) {
	db := testDB(t)
	previous := time.Local
	time.Local = time.FixedZone("test-Asia-Shanghai", 8*60*60)
	t.Cleanup(func() { time.Local = previous })
	store := postgreswork.NewStore(db)
	user, key := testIdentity(t)
	ctx := context.Background()
	topic, err := store.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "消息时间"})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := store.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: key + "turn", Question: "第一轮", Action: "discuss"})
	if err != nil {
		t.Fatal(err)
	}
	var storedHour int
	if err = db.Raw(`SELECT EXTRACT(HOUR FROM create_time)::integer FROM t_message WHERE id=?`, turn.UserMessageID).Scan(&storedHour).Error; err != nil {
		t.Fatal(err)
	}
	if storedHour != turn.CreatedAt.Hour() {
		t.Fatalf("legacy timestamp hour=%d, runtime hour=%d", storedHour, turn.CreatedAt.Hour())
	}
}
