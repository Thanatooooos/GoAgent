package citation

import (
	"strings"
	"testing"
)

func TestCompactPublicCitationsFoldsKBTag(t *testing.T) {
	r := NewRegistry()
	history := `根据 <kb doc="标题" chunk_id="chunk-a" kb_id="kb-a" /> 说明。`
	got := r.CompactPublicCitations(history)
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("kb tag not folded to ref: %s", got)
	}
	if strings.Contains(got, "chunk-a") || strings.Contains(got, "<kb") {
		t.Fatalf("durable id leaked after compaction: %s", got)
	}
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.ChunkID != "chunk-a" || ref.DocumentTitle != "标题" || ref.KnowledgeBaseID != "kb-a" {
		t.Fatalf("compact should register chunk: %+v, %v", ref, ok)
	}
}

func TestCompactPublicCitationsReusesHandle(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	got := r.CompactPublicCitations(`<kb doc="标题" chunk_id="chunk-a" />`)
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("same chunk should reuse c1: %s", got)
	}
}

func TestCompactPublicCitationsKeepsNonKBTag(t *testing.T) {
	r := NewRegistry()
	got := r.CompactPublicCitations("普通文本 <ref id=\"c1\"/> 保持原样")
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("existing ref changed: %s", got)
	}
}

func TestCompactPublicCitationsEmptyAndNil(t *testing.T) {
	r := NewRegistry()
	if got := r.CompactPublicCitations(""); got != "" {
		t.Fatalf("empty input = %q, want empty", got)
	}
	var nilRegistry *Registry
	if got := nilRegistry.CompactPublicCitations("<kb doc=\"x\" chunk_id=\"c\"/>"); got != "<kb doc=\"x\" chunk_id=\"c\"/>" {
		t.Fatalf("nil registry should pass through: %s", got)
	}
}

func TestCompactPublicCitationsUnescapesAttributes(t *testing.T) {
	r := NewRegistry()
	got := r.CompactPublicCitations(`<kb doc="A &amp; B" chunk_id="kb&amp;1" />`)
	if !strings.Contains(got, `<ref id="c1"/>`) {
		t.Fatalf("kb tag not folded: %s", got)
	}
	ref, ok := r.ResolveChunk("c1")
	if !ok || ref.DocumentTitle != "A & B" || ref.ChunkID != "kb&1" {
		t.Fatalf("attributes not unescaped: %+v, %v", ref, ok)
	}
}

func TestCompactPublicCitationsKeepsTagWithoutChunkID(t *testing.T) {
	r := NewRegistry()
	input := `a <kb doc="标题" /> b`
	if got := r.CompactPublicCitations(input); got != input {
		t.Fatalf("tag without chunk_id should stay as-is: %s", got)
	}
}

func TestCompactPublicCitationsMultipleTags(t *testing.T) {
	r := NewRegistry()
	got := r.CompactPublicCitations(`<kb doc="A" chunk_id="chunk-a" /> x <kb doc="B" chunk_id="chunk-b" /> y <kb doc="A" chunk_id="chunk-a" />`)
	if !strings.Contains(got, `<ref id="c1"/>`) || !strings.Contains(got, `<ref id="c2"/>`) {
		t.Fatalf("tags not folded: %s", got)
	}
	// duplicate chunk reuses the same handle
	if strings.Count(got, `<ref id="c1"/>`) != 2 {
		t.Fatalf("duplicate chunk should reuse c1: %s", got)
	}
}
