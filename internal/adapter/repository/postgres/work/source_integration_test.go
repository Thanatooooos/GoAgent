package work_test

import (
	"context"
	"errors"
	postgresknowledge "local/rag-project/internal/adapter/repository/postgres/knowledge"
	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	runtimeadapter "local/rag-project/internal/adapter/runtime"
	pgvector "local/rag-project/internal/adapter/vectorstore/pgvector"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeport "local/rag-project/internal/app/knowledge/port"
	corevector "local/rag-project/internal/app/rag/core/vector"
	"local/rag-project/internal/app/work/domain"
	"slices"
	"testing"
)

func TestPrivateSourceScopePromotionAndRemoval(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()
	s := postgreswork.NewStore(db)
	user, key := testIdentity(t)
	topic, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"topic", 0), Name: "附件范围"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateTopic(ctx, user, domain.CreateTopic{Mutation: mutation(key+"other", 0), Name: "其他专题"})
	if err != nil {
		t.Fatal(err)
	}
	one, err := s.CreateConversation(ctx, user, topic.ID, domain.CreateConversation{Mutation: mutation(key+"one", 0), Title: "第一段"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := s.CreateConversation(ctx, user, topic.ID, domain.CreateConversation{Mutation: mutation(key+"two", 0), Title: "第二段"})
	if err != nil {
		t.Fatal(err)
	}
	source, err := s.ReserveSource(ctx, user, topic.ID, "test", domain.ReserveSource{Mutation: mutation(key+"source", 0), Name: "私有验证材料", SourceType: "file", ConversationID: one.ID})
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.ClaimSource(ctx, user, topic.ID, source.ID); err != nil || !ok {
		t.Fatal(err)
	}
	doc := knowledgedomain.NewUploadedKnowledgeDocument(source.ID, source.KnowledgeBaseID, "private_scope_sentinel", "fixture", "txt", user, 1)
	doc.Status = "success"
	docRepo := postgresknowledge.NewKnowledgeDocumentRepository(db, nil)
	if _, err := docRepo.Create(ctx, doc); err != nil {
		t.Fatal(err)
	}
	if err := s.AttachSource(ctx, user, topic.ID, source.ID, doc.ID, "fixture"); err != nil {
		t.Fatal(err)
	}
	ids, err := s.SourceScope(ctx, user, topic.ID, one.ID)
	if err != nil || !slices.Contains(ids, source.KnowledgeBaseID) {
		t.Fatalf("owner scope: %v %v", ids, err)
	}
	ids, err = s.SourceScope(ctx, user, topic.ID, two.ID)
	if err != nil || slices.Contains(ids, source.KnowledgeBaseID) {
		t.Fatal("attachment leaked to other conversation")
	}
	if _, err := s.GetSource(ctx, "another-user", topic.ID, source.ID, one.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("source leaked to another user")
	}
	if _, err := s.GetSource(ctx, user, other.ID, source.ID, one.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("source leaked to another topic")
	}
	public, err := runtimeadapter.NewGlobalKnowledgeBaseAccess(db).AccessibleIDs(ctx, user)
	if err != nil || slices.Contains(public, source.KnowledgeBaseID) {
		t.Fatal("private source in ordinary/scheduled scope", err)
	}
	bases, err := postgresknowledge.NewKnowledgeBaseRepository(db).List(ctx, knowledgeport.KnowledgeBaseListFilter{})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range bases {
		if b.ID == source.KnowledgeBaseID {
			t.Fatal("private container in discovery")
		}
	}
	vectors := pgvector.NewVectorStore(db)
	v := make([]float32, 1024)
	v[0] = 1
	if err := vectors.UpsertDocumentChunks(ctx, []knowledgeport.ChunkVector{{ChunkID: source.ID, DocumentID: doc.ID, KnowledgeBaseID: source.KnowledgeBaseID, Text: "private_scope_sentinel", Embedding: v, Metadata: map[string]any{"document_name": "private_scope_sentinel"}}}); err != nil {
		t.Fatal(err)
	}
	otherDimension := make([]float32, 16)
	otherDimension[0] = 1
	if err := vectors.UpsertDocumentChunks(ctx, []knowledgeport.ChunkVector{{ChunkID: source.ID + "mixed", DocumentID: doc.ID, KnowledgeBaseID: source.KnowledgeBaseID, Text: "incompatible vector", Embedding: otherDimension}}); err != nil {
		t.Fatal(err)
	}
	hits, err := vectors.Search(ctx, corevector.SearchRequest{Vector: v, KnowledgeBaseIDs: []string{source.KnowledgeBaseID}, TopK: 10})
	if err != nil || len(hits) != 1 {
		t.Fatalf("scoped vector: %d %v", len(hits), err)
	}
	hits, err = vectors.Search(ctx, corevector.SearchRequest{Vector: v, TopK: 100})
	if err != nil {
		t.Fatal(err)
	}
	for _, h := range hits {
		if h.KnowledgeBaseID == source.KnowledgeBaseID {
			t.Fatal("empty vector scope leaked private data")
		}
	}
	for _, search := range []func(context.Context, string, []string, int) ([]corevector.SearchHit, error){vectors.SearchByKeyword, vectors.SearchByMetadata} {
		hits, err := search(ctx, "private_scope_sentinel", nil, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range hits {
			if h.KnowledgeBaseID == source.KnowledgeBaseID {
				t.Fatal("empty lexical scope leaked private data")
			}
		}
	}
	if _, err := s.ChangeSource(ctx, user, topic.ID, source.ID, true, mutation(key+"promote", 0)); err != nil {
		t.Fatal(err)
	}
	ids, _ = s.SourceScope(ctx, user, topic.ID, two.ID)
	if !slices.Contains(ids, source.KnowledgeBaseID) {
		t.Fatal("promotion did not share attachment")
	}
	if _, err := s.ChangeSource(ctx, user, topic.ID, source.ID, false, mutation(key+"remove", 0)); err != nil {
		t.Fatal(err)
	}
	ids, _ = s.SourceScope(ctx, user, topic.ID, one.ID)
	if slices.Contains(ids, source.KnowledgeBaseID) {
		t.Fatal("removed source remained searchable before cleanup")
	}
	if _, err := s.GetSource(ctx, user, topic.ID, source.ID, one.ID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatal("removed source remained downloadable")
	}
	if _, err := s.ChangeSource(ctx, user, topic.ID, source.ID, true, mutation(key+"late-promote", 0)); err == nil {
		t.Fatal("cleanup source can be promoted")
	}
}
