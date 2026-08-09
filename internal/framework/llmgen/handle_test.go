package llmgen

import "testing"

func TestHandleSetEncodeDedup(t *testing.T) {
	h := NewHandleSet("r")
	h1, ok := h.Encode("chunk-abc")
	if !ok || h1 != "r1" {
		t.Fatalf("first encode = (%q,%v), want (r1,true)", h1, ok)
	}
	h2, ok := h.Encode("chunk-abc")
	if !ok || h2 != "r1" {
		t.Fatalf("re-encode same id = (%q,%v), want (r1,true)", h2, ok)
	}
	h3, ok := h.Encode("chunk-def")
	if !ok || h3 != "r2" {
		t.Fatalf("second encode = (%q,%v), want (r2,true)", h3, ok)
	}
}

func TestHandleSetEncodeEmpty(t *testing.T) {
	h := NewHandleSet("r")
	if _, ok := h.Encode("  "); ok {
		t.Fatal("empty id should not encode")
	}
}

func TestHandleSetResolveRoundTripAndFailClosed(t *testing.T) {
	h := NewHandleSet("r")
	h.Encode("chunk-abc")
	if id, ok := h.Resolve("r1"); !ok || id != "chunk-abc" {
		t.Fatalf("resolve r1 = (%q,%v)", id, ok)
	}
	if id, ok := h.Resolve("R1"); !ok || id != "chunk-abc" {
		t.Fatalf("resolve R1 (case-insensitive) = (%q,%v)", id, ok)
	}
	if _, ok := h.Resolve("r9"); ok {
		t.Fatal("resolve unknown handle should fail closed")
	}
}

func TestHandleSetResolveAll(t *testing.T) {
	h := NewHandleSet("r")
	h.Encode("chunk-a")
	h.Encode("chunk-b")
	resolved, unresolved := h.ResolveAll([]string{"r1", "r9", "r2"})
	if len(resolved) != 2 || len(unresolved) != 1 || unresolved[0] != "r9" {
		t.Fatalf("ResolveAll = (%v, %v)", resolved, unresolved)
	}
}
