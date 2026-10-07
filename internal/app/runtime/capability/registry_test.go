package capability

import (
	"context"
	"encoding/json"
	"testing"
)

func TestRegistryPrepareValidatesBeforeDescribe(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	described := false
	if err := registry.Register(Def{ID: "x", Description: "x", JSONSchema: json.RawMessage(`{"type":"object"}`), Validate: func(Value) error { return assertError{} }, Describe: func(Value, Context) (Operation, error) { described = true; return Operation{}, nil }, Execute: func(Value, Context) (Result, error) { return Result{}, nil }}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := registry.Prepare("x", Value(`{}`), Context{Context: context.Background()}); err == nil {
		t.Fatal("invalid arguments must fail")
	}
	if described {
		t.Fatal("describe ran after validation failed")
	}
}

func TestRegistryListsCallbackFreeDefinitionsInIDOrder(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	for _, id := range []string{"z", "a"} {
		if err := registry.Register(testDef(id)); err != nil {
			t.Fatal(err)
		}
	}
	defs := registry.List()
	if len(defs) != 2 || defs[0].ID != "a" || defs[1].ID != "z" {
		t.Fatalf("definitions = %#v", defs)
	}
}

func testDef(id string) Def {
	return Def{ID: id, Description: id, JSONSchema: json.RawMessage(`{"type":"object"}`), Validate: func(Value) error { return nil }, Describe: func(Value, Context) (Operation, error) { return Operation{}, nil }, Execute: func(Value, Context) (Result, error) { return Result{}, nil }}
}

type assertError struct{}

func (assertError) Error() string { return "invalid" }
