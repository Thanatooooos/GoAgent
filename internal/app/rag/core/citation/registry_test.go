package citation

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

func TestRegistryRegisterAndResolveChunk(t *testing.T) {
	r := NewRegistry()
	handle := r.RegisterChunk(ChunkReference{ChunkID: "chunk-1", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", DocumentTitle: "标题"})
	if handle != "c1" {
		t.Fatalf("handle = %q, want c1", handle)
	}
	if got := r.RegisterChunk(ChunkReference{ChunkID: "chunk-1"}); got != "c1" {
		t.Fatalf("re-register = %q, want same c1", got)
	}
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-1" || ref.DocumentID != "doc-1" || ref.KnowledgeBaseID != "kb-1" || ref.DocumentTitle != "标题" {
		t.Fatalf("resolve c1 = %+v, %v", ref, ok)
	}
	if _, ok := r.ResolveChunk("c9"); ok {
		t.Fatal("resolve unknown handle should fail")
	}
}

func TestRegistryHandleShapedIDsAreNotRegistered(t *testing.T) {
	r := NewRegistry()
	if got := r.RegisterChunk(ChunkReference{ChunkID: "c1"}); got != "" {
		t.Fatalf("handle-shaped id registered as %q, want empty", got)
	}
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	if got := r.RegisterChunk(ChunkReference{ChunkID: "c1"}); got != "c1" {
		t.Fatalf("existing c1 not echoed back, got %q", got)
	}
}

func TestRegistryRegisterChunks(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunks([]convention.RetrievedChunk{
		{ID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a"},
		{ID: "chunk-b", DocumentID: "doc-b", KnowledgeBaseID: "kb-b"},
	})
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-a" {
		t.Fatalf("first chunk resolve = %+v, %v", ref, ok)
	}
	ref, ok = r.ResolveChunk("c2")
	if !ok || ref.ChunkID != "chunk-b" || ref.DocumentID != "doc-b" {
		t.Fatalf("second chunk resolve = %+v, %v", ref, ok)
	}
}

func TestRegistryEmptyChunkIDNotRegistered(t *testing.T) {
	r := NewRegistry()
	if got := r.RegisterChunk(ChunkReference{}); got != "" {
		t.Fatalf("empty chunk id registered as %q", got)
	}
}
