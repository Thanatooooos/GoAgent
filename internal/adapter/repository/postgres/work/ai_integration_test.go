package work_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
)

func testIdentity(t *testing.T) (string, string) {
	t.Helper()
	n := time.Now().UnixNano()
	return fmt.Sprintf("w%d", n), fmt.Sprintf("ai-%d-", n)
}

func TestAIRoundWritesAndHumanConfirmation(t *testing.T) {
	db := testDB(t)
	s := postgreswork.NewStore(db)
	ctx := context.Background()
	user, request := testIdentity(t)
	topic, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: domain.Mutation{RequestID: request + "topic"}, Name: "协作文档"})
	if err != nil {
		t.Fatal(err)
	}
	doc, err := s.CreateArtifact(ctx, user, topic.ID, domain.SaveArtifact{Mutation: domain.Mutation{RequestID: request + "doc"}, Title: "方案", Body: domain.EmptyDocument()})
	if err != nil {
		t.Fatal(err)
	}
	turn, err := s.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: request + "turn", Question: "补充第一段", Action: "edit_document", ArtifactID: doc.Artifact.ID, ArtifactRevision: 1})
	if err != nil {
		t.Fatal(err)
	}
	again, err := s.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: request + "turn", Question: "补充第一段", Action: "edit_document", ArtifactID: doc.Artifact.ID, ArtifactRevision: 1})
	if err != nil || again.ID != turn.ID {
		t.Fatalf("duplicate input: %v", err)
	}
	if ok, err := s.StartTurn(ctx, user, topic.ID, turn.ID); err != nil || !ok {
		t.Fatal(err)
	}
	tool := capability.Context{Context: ctx, UserID: user, ToolCallID: "call1", Work: &capability.WorkScope{TopicID: topic.ID, TurnID: turn.ID}}
	node := domain.Node{ID: "p1", Type: "paragraph", Content: []domain.Node{{Type: "text", Text: "AI 增补"}}}
	change := domain.AIDocumentChange{Summary: "补充第一段", Changes: []domain.BlockChange{{Kind: "replace", TargetID: "p1", Node: &node}}}
	saved, err := s.WriteDocument(tool, change)
	if err != nil || saved.Version.Revision != 2 || saved.Version.Author != "ai" {
		t.Fatalf("write: %v", err)
	}
	replay, err := s.WriteDocument(tool, change)
	if err != nil || replay.Version.Revision != 2 {
		t.Fatalf("replay: %v", err)
	}
	changes := []domain.StateChange{{Kind: "add", Entry: domain.StateEntry{ID: "next1", Kind: "next", Text: "验证方案"}}}
	proposal, err := s.SuggestProgress(tool, changes)
	if err != nil {
		t.Fatal(err)
	}
	current, err := s.GetState(ctx, user, topic.ID)
	if err != nil || len(current.Entries) != 0 {
		t.Fatalf("suggestion mutated progress: %+v %v", current, err)
	}
	applied, err := s.ResolveProposal(ctx, user, topic.ID, proposal.ID, domain.ResolveProposal{Mutation: domain.Mutation{RequestID: request + "apply", ExpectedRevision: current.Revision}})
	if err != nil || applied.Status != "applied" {
		t.Fatal(err)
	}
	current, _ = s.GetState(ctx, user, topic.ID)
	if len(current.Entries) != 1 || current.Entries[0].References[0].ID != turn.UserMessageID {
		t.Fatalf("missing basis: %+v", current)
	}
	if err := s.FinishTurn(ctx, user, topic.ID, turn.ID, "completed", ""); err != nil {
		t.Fatal(err)
	}
	outputs, err := s.GetTurn(ctx, user, topic.ID, turn.ID)
	if err != nil || len(outputs.Outputs) != 2 || outputs.Outputs[0].ArtifactID != doc.Artifact.ID || outputs.Outputs[0].Revision != 2 || outputs.Outputs[1].ProposalID != proposal.ID {
		t.Fatalf("durable result projection: %+v %v", outputs.Outputs, err)
	}
	if _, err := s.WriteDocument(tool, change); err != nil {
		t.Fatalf("terminal replay failed: %v", err)
	}
	late, err := s.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: request + "late", Question: "补充", Action: "edit_document", ArtifactID: doc.Artifact.ID, ArtifactRevision: 2})
	if err != nil {
		t.Fatal(err)
	}
	s.StartTurn(ctx, user, topic.ID, late.ID)
	_, err = s.SaveArtifact(ctx, user, topic.ID, doc.Artifact.ID, domain.SaveArtifact{Mutation: domain.Mutation{RequestID: request + "manual", ExpectedRevision: 2}, Title: "人工修改", Body: domain.EmptyDocument()})
	if err != nil {
		t.Fatal(err)
	}
	tool.Work.TurnID = late.ID
	_, err = s.WriteDocument(tool, change)
	var conflict *domain.Conflict
	if !errors.As(err, &conflict) {
		t.Fatalf("late overwrite permitted: %v", err)
	}
	if err := s.CancelTurn(ctx, user, topic.ID, late.ID); err != nil {
		t.Fatal(err)
	}
	tool.ToolCallID = "call2"
	if _, err = s.SuggestProgress(tool, changes); err == nil {
		t.Fatal("cancelled turn may write")
	}
	versions, _ := s.ListVersions(ctx, user, topic.ID, doc.Artifact.ID, domain.Page{Limit: 30})
	if len(versions) != 3 {
		t.Fatalf("extra versions: %d", len(versions))
	}
}
