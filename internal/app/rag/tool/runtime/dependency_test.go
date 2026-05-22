package runtime

import (
	"context"
	"testing"

	. "local/rag-project/internal/app/rag/tool/core"
)

func makeSpec(name string, after []string) ToolSpec {
	return ToolSpec{
		Definition: Definition{Name: name, ReadOnly: true},
		ReadOnly:   true,
		After:      after,
	}
}

func makeRegistry(entries ...ToolSpec) *Registry {
	r := NewRegistry()
	for _, spec := range entries {
		r.MustRegisterModule(ToolModule{Name: spec.Definition.Name, Invoker: testInvoker{}, Spec: spec}.Normalize())
	}
	return r
}

type testInvoker struct{}

func (testInvoker) Invoke(_ context.Context, _ Call) (Result, error) { return Result{}, nil }

func TestResolveLevelsNoDependencies(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_b", nil),
		makeSpec("tool_c", nil),
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
		{Name: "tool_c", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (no constraints), got %v", levels)
	}
}

func TestResolveLevelsSingleChain(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_b", []string{"tool_a"}),
		makeSpec("tool_c", []string{"tool_b"}),
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
		{Name: "tool_c", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels == nil {
		t.Fatal("expected 3 levels for chain a->b->c")
	}
	if len(levels) != 3 {
		t.Fatalf("expected 3 levels, got %d: %v", len(levels), levels)
	}
	if len(levels[0]) != 1 || calls[levels[0][0]].Name != "tool_a" {
		t.Fatalf("expected level 0 to be [tool_a], got %v", levelNames(calls, levels[0]))
	}
	if len(levels[1]) != 1 || calls[levels[1][0]].Name != "tool_b" {
		t.Fatalf("expected level 1 to be [tool_b], got %v", levelNames(calls, levels[1]))
	}
	if len(levels[2]) != 1 || calls[levels[2][0]].Name != "tool_c" {
		t.Fatalf("expected level 2 to be [tool_c], got %v", levelNames(calls, levels[2]))
	}
}

func TestResolveLevelsFanIn(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_b", nil),
		makeSpec("tool_c", []string{"tool_a", "tool_b"}),
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
		{Name: "tool_c", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels == nil {
		t.Fatal("expected 2 levels for fan-in a,b->c")
	}
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %v", len(levels), levels)
	}
	if len(levels[0]) != 2 {
		t.Fatalf("expected level 0 to have 2 calls, got %d", len(levels[0]))
	}
	if len(levels[1]) != 1 || calls[levels[1][0]].Name != "tool_c" {
		t.Fatalf("expected level 1 to be [tool_c], got %v", levelNames(calls, levels[1]))
	}
}

func TestResolveLevelsFanOut(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_b", []string{"tool_a"}),
		makeSpec("tool_c", []string{"tool_a"}),
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
		{Name: "tool_c", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels == nil {
		t.Fatal("expected 2 levels for fan-out a->b,a->c")
	}
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %v", len(levels), levels)
	}
	if len(levels[0]) != 1 || calls[levels[0][0]].Name != "tool_a" {
		t.Fatalf("expected level 0 to be [tool_a], got %v", levelNames(calls, levels[0]))
	}
	if len(levels[1]) != 2 {
		t.Fatalf("expected level 1 to have 2 calls, got %d", len(levels[1]))
	}
}

func TestResolveLevelsMissingDepNotInPlan(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_b", []string{"tool_a"}), // tool_a not in plan
	)
	calls := []Call{
		{Name: "tool_b", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (single call, no deps in plan), got %v", levels)
	}
}

func TestResolveLevelsDependencyNotInPlan(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_b", []string{"tool_missing"}), // missing not in plan
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (b's dep is not in plan, so no constraints), got %v", levels)
	}
}

func TestResolveLevelsCycle(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", []string{"tool_b"}),
		makeSpec("tool_b", []string{"tool_a"}),
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (cycle fallback), got %v", levels)
	}
}

func TestResolveLevelsSelfDepIgnored(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", []string{"tool_a"}), // self-reference
		makeSpec("tool_b", nil),
	)
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (self-dependency filtered, no real edge), got %v", levels)
	}
}

func TestResolveLevelsNilRegistry(t *testing.T) {
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, nil)
	if levels != nil {
		t.Fatalf("expected nil (nil registry), got %v", levels)
	}
}

func TestResolveLevelsEmptyCalls(t *testing.T) {
	reg := makeRegistry(makeSpec("tool_a", nil))
	levels := resolveExecutionLevels(nil, reg)
	if levels != nil {
		t.Fatalf("expected nil (empty calls), got %v", levels)
	}
}

func TestResolveLevelsSingleCall(t *testing.T) {
	reg := makeRegistry(makeSpec("tool_a", []string{"tool_b"}))
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (single call), got %v", levels)
	}
}

func TestResolveLevelsOrDeps(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_b", nil),
		makeSpec("tool_c", []string{"tool_a", "tool_b"}), // depends on both
	)
	calls := []Call{
		{Name: "tool_c", Arguments: map[string]any{}},
		{Name: "tool_b", Arguments: map[string]any{}},
		{Name: "tool_a", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels == nil {
		t.Fatal("expected 2 levels for or-deps")
	}
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %v", len(levels), levels)
	}
	if len(levels[0]) != 2 {
		t.Fatalf("expected level 0 to have 2 calls (a,b), got %d", len(levels[0]))
	}
	if len(levels[1]) != 1 || calls[levels[1][0]].Name != "tool_c" {
		t.Fatalf("expected level 1 to be [tool_c], got %v", levelNames(calls, levels[1]))
	}
}

func TestResolveLevelsOrDepsPartial(t *testing.T) {
	reg := makeRegistry(
		makeSpec("tool_a", nil),
		makeSpec("tool_c", []string{"tool_a", "tool_missing"}), // only tool_a in plan
	)
	calls := []Call{
		{Name: "tool_c", Arguments: map[string]any{}},
		{Name: "tool_a", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels == nil {
		t.Fatal("expected 2 levels for partial or-deps")
	}
	if len(levels) != 2 {
		t.Fatalf("expected 2 levels, got %d: %v", len(levels), levels)
	}
	if len(levels[0]) != 1 || calls[levels[0][0]].Name != "tool_a" {
		t.Fatalf("expected level 0 to be [tool_a], got %v", levelNames(calls, levels[0]))
	}
	if len(levels[1]) != 1 || calls[levels[1][0]].Name != "tool_c" {
		t.Fatalf("expected level 1 to be [tool_c], got %v", levelNames(calls, levels[1]))
	}
}

func TestResolveLevelsSpecNotFound(t *testing.T) {
	reg := makeRegistry(makeSpec("tool_a", nil))
	calls := []Call{
		{Name: "tool_a", Arguments: map[string]any{}},
		{Name: "tool_missing", Arguments: map[string]any{}},
	}
	levels := resolveExecutionLevels(calls, reg)
	if levels != nil {
		t.Fatalf("expected nil (missing has no spec, no deps), got %v", levels)
	}
}

func levelNames(calls []Call, indices []int) []string {
	names := make([]string, len(indices))
	for i, idx := range indices {
		names[i] = calls[idx].Name
	}
	return names
}
