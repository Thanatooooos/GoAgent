package citation

import "testing"

func TestHandleTableRegisterDedup(t *testing.T) {
	table := newHandleTable[string]("c")
	if got := table.register("chunk-a", "A"); got != "c1" {
		t.Fatalf("first register = %q, want c1", got)
	}
	if got := table.register("chunk-a", "A2"); got != "c1" {
		t.Fatalf("duplicate register = %q, want same handle c1", got)
	}
	if got := table.register("chunk-b", "B"); got != "c2" {
		t.Fatalf("second register = %q, want c2", got)
	}
}

func TestHandleTableResolve(t *testing.T) {
	table := newHandleTable[string]("c")
	table.register("chunk-a", "A")
	key, value, ok := table.resolve("c1")
	if !ok || key != "chunk-a" || value != "A" {
		t.Fatalf("resolve c1 = (%q, %q, %v)", key, value, ok)
	}
	if _, _, ok := table.resolve("c99"); ok {
		t.Fatal("resolve unknown handle should fail")
	}
	if key, _, ok := table.resolve("C1"); !ok || key != "chunk-a" {
		t.Fatalf("resolve C1 (case-insensitive) = (%q, %v)", key, ok)
	}
}

func TestHandleTableHas(t *testing.T) {
	table := newHandleTable[int]("b")
	table.register("kb-x", 7)
	if !table.has("b1") {
		t.Fatal("has b1 = false, want true")
	}
	if table.has("b2") {
		t.Fatal("has b2 = true, want false")
	}
}
