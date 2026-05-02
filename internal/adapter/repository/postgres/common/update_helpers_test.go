package common

import (
	"testing"
)

func TestBuildUpdateAssignments(t *testing.T) {
	type assignment struct {
		field string
		value any
	}

	updates, err := BuildUpdateAssignments(
		[]assignment{{field: "name", value: "alice"}},
		func(item assignment) string { return item.field },
		func(item assignment) any { return item.value },
		func(field string) (string, bool) { return field, true },
		func(_ string, value any) (any, error) { return value, nil },
	)
	if err != nil {
		t.Fatalf("BuildUpdateAssignments returned error: %v", err)
	}
	if got := updates["name"]; got != "alice" {
		t.Fatalf("unexpected assignment value: %#v", got)
	}
}

func TestConditionalUpdateRequiresConditions(t *testing.T) {
	err := ConditionalUpdateRequiresConditions("demo")
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got != "update demo with conditions: conditions are required" {
		t.Fatalf("unexpected error: %q", got)
	}
}
