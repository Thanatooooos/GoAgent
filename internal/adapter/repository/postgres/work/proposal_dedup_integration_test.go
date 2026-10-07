package work_test

import (
	"context"
	"testing"

	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
)

func TestUnchangedSuggestionsDoNotCreateNewPendingRecords(t *testing.T) {
	s := postgreswork.NewStore(testDB(t))
	ctx := context.Background()
	user, key := testIdentity(t)
	topic, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "建议去重"})
	if err != nil {
		t.Fatal(err)
	}
	start := func(label string) capability.Context {
		t.Helper()
		turn, err := s.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: key + label, Question: "继续讨论", Action: "discuss"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.StartTurn(ctx, user, topic.ID, turn.ID); err != nil {
			t.Fatal(err)
		}
		return capability.Context{Context: ctx, UserID: user, ToolCallID: label, Work: &capability.WorkScope{TopicID: topic.ID, TurnID: turn.ID}}
	}
	first := start("first")
	change := []domain.StateChange{{Kind: "add", Entry: domain.StateEntry{ID: "next1", Kind: "next", Text: "验证方案"}}}
	p, err := s.SuggestProgress(first, change)
	if err != nil {
		t.Fatal(err)
	}
	if replay, err := s.SuggestProgress(first, change); err != nil || replay.ID != p.ID {
		t.Fatal("replay changed caller input", err)
	}
	body := domain.EmptyDocument()
	d, err := s.SuggestDocument(first, domain.SuggestDocument{Change: domain.AIDocumentChange{Title: "拟定方案", Summary: "建议", Body: &body}})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.FinishTurn(ctx, user, topic.ID, first.Work.TurnID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	second := start("second")
	change[0].Entry.ID = "another-generated-id"
	repeated, err := s.SuggestProgress(second, change)
	if err != nil || repeated.ID != p.ID {
		t.Fatal("same progress created another proposal", err)
	}
	repeatedDoc, err := s.SuggestDocument(second, domain.SuggestDocument{Change: domain.AIDocumentChange{Title: "拟定方案", Summary: "换了摘要", Body: &body}})
	if err != nil || repeatedDoc.ID != d.ID {
		t.Fatal("same document created another proposal", err)
	}
	secondTurn, err := s.GetTurn(ctx, user, topic.ID, second.Work.TurnID)
	if err != nil || len(secondTurn.Outputs) != 0 {
		t.Fatal("unchanged suggestions produced repeated notification cards", err)
	}
	if _, err = s.ResolveProposal(ctx, user, topic.ID, d.ID, domain.ResolveProposal{Mutation: mutation(key+"ignore", 0), Ignore: true}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.ResolveProposal(ctx, user, topic.ID, p.ID, domain.ResolveProposal{Mutation: mutation(key+"apply", 1)}); err != nil {
		t.Fatal(err)
	}
	if err = s.FinishTurn(ctx, user, topic.ID, second.Work.TurnID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	third := start("third")
	if _, err = s.SuggestProgress(third, change); err == nil {
		t.Fatal("confirmed identical next step created another suggestion")
	}
	ignored, err := s.SuggestDocument(third, domain.SuggestDocument{Change: domain.AIDocumentChange{Title: "拟定方案", Body: &body}})
	if err != nil || ignored.ID != d.ID || ignored.Status != "ignored" {
		t.Fatal("ignored unchanged suggestion was re-opened", err)
	}
	list, err := s.ListProposals(ctx, user, topic.ID, domain.Page{Limit: 30})
	if err != nil || len(list) != 2 {
		t.Fatal("duplicate pending records", err)
	}
}
