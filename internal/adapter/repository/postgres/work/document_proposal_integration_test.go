package work_test

import (
	"context"
	"errors"
	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/work/domain"
	"testing"
)

func TestDocumentSuggestionsRequireHumanApplicationAndProtectLaterEdits(t *testing.T) {
	db := testDB(t)
	s := postgreswork.NewStore(db)
	ctx := context.Background()
	user, key := testIdentity(t)
	topic, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "主动文档建议"})
	if err != nil {
		t.Fatal(err)
	}
	start := func(request string, artifact string, revision int) capability.Context {
		t.Helper()
		turn, err := s.AcceptTurn(ctx, user, topic.ID, domain.ChatRequest{RequestID: key + request, Question: "只讨论改进，不保存", Action: "discuss", ArtifactID: artifact, ArtifactRevision: revision})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.StartTurn(ctx, user, topic.ID, turn.ID); err != nil {
			t.Fatal(err)
		}
		return capability.Context{Context: ctx, UserID: user, ToolCallID: request, Work: &capability.WorkScope{TopicID: topic.ID, TurnID: turn.ID}}
	}
	tool := start("new", "", 0)
	body := domain.EmptyDocument()
	input := domain.SuggestDocument{Change: domain.AIDocumentChange{Title: "建议文档", Summary: "建议创建而未保存", Body: &body}}
	p, err := s.SuggestDocument(tool, input)
	if err != nil {
		t.Fatal(err)
	}
	if rows, err := s.ListArtifacts(ctx, user, topic.ID, domain.Page{Limit: 30}); err != nil || len(rows) != 0 {
		t.Fatal("suggestion created a document", err)
	}
	if _, err := s.WriteDocument(tool, input.Change); err == nil {
		t.Fatal("discussion authorized a write")
	}
	again, err := s.SuggestDocument(tool, input)
	if err != nil || again.ID != p.ID {
		t.Fatal("duplicate proposal", err)
	}
	applied, err := s.ResolveProposal(ctx, user, topic.ID, p.ID, domain.ResolveProposal{Mutation: mutation(key+"apply", 0)})
	if err != nil || applied.AppliedRevision != 1 || applied.Document.ArtifactID == "" {
		t.Fatal("application", err)
	}
	repeated, err := s.ResolveProposal(ctx, user, topic.ID, p.ID, domain.ResolveProposal{Mutation: mutation(key+"apply-again", 0)})
	if err != nil || repeated.Document.ArtifactID != applied.Document.ArtifactID {
		t.Fatal("second document", err)
	}
	doc, err := s.GetArtifact(ctx, user, topic.ID, applied.Document.ArtifactID)
	if err != nil {
		t.Fatal(err)
	}
	s.FinishTurn(ctx, user, topic.ID, tool.Work.TurnID, "completed", "")
	tool = start("update", doc.Artifact.ID, 1)
	paragraph := domain.Node{ID: doc.Version.Body.Root.Content[0].ID, Type: "paragraph", Content: []domain.Node{{Type: "text", Text: "AI 拟修改正文"}}}
	edit := domain.SuggestDocument{ArtifactID: doc.Artifact.ID, Change: domain.AIDocumentChange{Summary: "建议补充", Changes: []domain.BlockChange{{Kind: "replace", TargetID: paragraph.ID, Node: &paragraph}}}}
	update, err := s.SuggestDocument(tool, edit)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := s.SaveArtifact(ctx, user, topic.ID, doc.Artifact.ID, domain.SaveArtifact{Mutation: mutation(key+"manual", 1), Title: doc.Artifact.Title, Body: body})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.ResolveProposal(ctx, user, topic.ID, update.ID, domain.ResolveProposal{Mutation: mutation(key+"conflict", saved.Version.Revision)})
	var conflict *domain.Conflict
	if !errors.As(err, &conflict) {
		t.Fatal("late proposal overwrote human edit", err)
	}
	list, err := s.ListProposals(ctx, user, topic.ID, domain.Page{Limit: 30})
	if err != nil || list[0].Document.Body.Root.Content[0].Content[0].Text != "AI 拟修改正文" || list[0].Status != "pending" {
		t.Fatal("conflict discarded proposed body", err)
	}
	ignored, err := s.ResolveProposal(ctx, user, topic.ID, update.ID, domain.ResolveProposal{Mutation: mutation(key+"ignore", 0), Ignore: true})
	if err != nil || ignored.Status != "ignored" {
		t.Fatal(err)
	}
	current, err := s.GetArtifact(ctx, user, topic.ID, doc.Artifact.ID)
	if err != nil || current.Version.Revision != 2 {
		t.Fatal("ignore mutated body", err)
	}
	s.FinishTurn(ctx, user, topic.ID, tool.Work.TurnID, "completed", "")
	tool = start("fresh", doc.Artifact.ID, 2)
	update, err = s.SuggestDocumentText(tool, doc.Artifact.ID, paragraph.ID, "AI 拟修改正文", "建议补充")
	if err != nil {
		t.Fatal(err)
	}
	applied, err = s.ResolveProposal(ctx, user, topic.ID, update.ID, domain.ResolveProposal{Mutation: mutation(key+"update-apply", 2)})
	if err != nil || applied.AppliedRevision != 3 {
		t.Fatal(err)
	}
	replayed, err := s.SuggestDocumentText(tool, doc.Artifact.ID, paragraph.ID, "AI 拟修改正文", "建议补充")
	if err != nil || replayed.ID != update.ID {
		t.Fatal("text proposal replay changed after application", err)
	}
	state, err := s.GetState(ctx, user, topic.ID)
	if err != nil || state.Revision != 1 {
		t.Fatal("document proposal confirmed progress", err)
	}
	output, err := s.GetTurn(ctx, user, topic.ID, tool.Work.TurnID)
	if err != nil || len(output.Outputs) != 1 || output.Outputs[0].Kind != "document_suggestion" || output.Outputs[0].ProposalID != update.ID {
		t.Fatal("missing durable card", err)
	}
	if _, err = s.ResolveProposal(ctx, "other-user", topic.ID, update.ID, domain.ResolveProposal{Mutation: mutation(key+"foreign", 2)}); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("foreign user applied", err)
	}
}
