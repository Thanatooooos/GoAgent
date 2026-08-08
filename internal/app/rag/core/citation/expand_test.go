package citation

import (
	"strings"
	"testing"
)

func TestExpandTextExpandsKnownRef(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a", DocumentID: "doc-a", KnowledgeBaseID: "kb-a", DocumentTitle: "标题"})
	got := r.ExpandText("根据 <ref id=\"c1\"/> 说明。", true)
	want := `<kb doc="标题" chunk_id="chunk-a" kb_id="kb-a" />`
	if !strings.Contains(got, want) {
		t.Fatalf("expanded output missing %q: %s", want, got)
	}
	if strings.Contains(got, "c1") {
		t.Fatalf("handle leaked in output: %s", got)
	}
}

func TestExpandTextDropsUnknownRef(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	got := r.ExpandText("x <ref id=\"c9\"/> y", true)
	if strings.Contains(got, "ref") || strings.Contains(got, "c9") {
		t.Fatalf("unknown ref not dropped: %s", got)
	}
}

func TestExpandTextStripsModelKBAndDisabledRefs(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a"})
	got := r.ExpandText("a <kb doc=\"x\" chunk_id=\"chunk-a\"/> b", true)
	if strings.Contains(got, "<kb") {
		t.Fatalf("model-written <kb> not stripped: %s", got)
	}
	got = r.ExpandText("a <ref id=\"c1\"/> b", false)
	if strings.Contains(got, "ref") {
		t.Fatalf("refs not stripped when disabled: %s", got)
	}
}

func TestStreamExpanderHandlesSplitTags(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a", DocumentTitle: "标题"})
	d := NewStreamExpander(r, true)
	var out strings.Builder
	out.WriteString(d.Feed("前文 <re"))
	out.WriteString(d.Feed("f id=\"c1\"/> 后文"))
	out.WriteString(d.Flush())
	if !strings.Contains(out.String(), `<kb doc="标题" chunk_id="chunk-a" />`) {
		t.Fatalf("split tag not expanded: %s", out.String())
	}
	if strings.Contains(out.String(), "c1") {
		t.Fatalf("handle leaked: %s", out.String())
	}
}

func TestStreamExpanderFlushDropsPartialTag(t *testing.T) {
	r := NewRegistry()
	d := NewStreamExpander(r, true)
	got := d.Feed("abc <ref id=\"c")
	got += d.Flush()
	if got != "abc " {
		t.Fatalf("flush output = %q, want %q", got, "abc ")
	}
}

func TestStreamExpanderPassthroughWhenNilRegistry(t *testing.T) {
	d := NewStreamExpander(nil, true)
	if got := d.Feed("plain <ref id=\"c1\"/>"); got != "plain <ref id=\"c1\"/>" {
		t.Fatalf("nil registry should pass through, got %q", got)
	}
}

func TestExpandTextUnterminatedRefDoesNotSwallowProse(t *testing.T) {
	r := NewRegistry()
	got := r.ExpandText("x <ref for citation. rest of prose", true)
	if got != "x <ref for citation. rest of prose" {
		t.Fatalf("unterminated ref swallowed prose, got %q", got)
	}
}

func TestExpandTextDropsBareRefTags(t *testing.T) {
	r := NewRegistry()
	got := r.ExpandText("a <ref> b <ref/> c", true)
	if got != "a  b  c" {
		t.Fatalf("bare ref tags not dropped, got %q", got)
	}
}

func TestStreamExpanderDropsBareRefTag(t *testing.T) {
	r := NewRegistry()
	d := NewStreamExpander(r, true)
	if got := d.Feed("a <ref> b"); got != "a  b" {
		t.Fatalf("bare ref tag not dropped, got %q", got)
	}
}

func TestStreamExpanderThreeChunkSplit(t *testing.T) {
	r := NewRegistry()
	r.RegisterChunk(ChunkReference{ChunkID: "chunk-a", DocumentTitle: "标题"})
	d := NewStreamExpander(r, true)
	var out strings.Builder
	out.WriteString(d.Feed("<re"))
	out.WriteString(d.Feed("f id=\"c1\""))
	out.WriteString(d.Feed("/> done"))
	out.WriteString(d.Flush())
	want := `<kb doc="标题" chunk_id="chunk-a" /> done`
	if out.String() != want {
		t.Fatalf("three-chunk split output = %q, want %q", out.String(), want)
	}
}

func TestStreamExpanderLessThanBoundaryAndNonTag(t *testing.T) {
	r := NewRegistry()
	d := NewStreamExpander(r, true)
	got := d.Feed("a <")
	got += d.Feed("10 b")
	if got != "a <10 b" {
		t.Fatalf("less-than boundary output = %q, want %q", got, "a <10 b")
	}
	d = NewStreamExpander(r, true)
	if got := d.Feed("use <refactor> here"); got != "use <refactor> here" {
		t.Fatalf("non-tag word mangled, got %q", got)
	}
}
