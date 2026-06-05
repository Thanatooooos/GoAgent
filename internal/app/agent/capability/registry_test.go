package capability

import (
	"context"
	"reflect"
	"testing"

	agentstate "local/rag-project/internal/app/agent/state"
)

type stubHandle struct {
	spec Spec
}

func (h stubHandle) Spec() Spec {
	return h.spec
}

func (h stubHandle) Invoke(_ context.Context, _ InvocationRequest) (InvocationResult, error) {
	return InvocationResult{
		Action: ActionRecord{
			Name:    h.spec.Name,
			Summary: "stub invoke",
		},
		Observation: ObservationRecord{
			Summary: "stub result",
		},
		Delta:  agentstate.StateDelta{},
		Status: StatusSucceeded,
	}, nil
}

func TestRegistry_RegisterLookupAndSpecs(t *testing.T) {
	registry := NewRegistry()
	searchHandle := stubHandle{spec: validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})}
	fetchHandle := stubHandle{spec: validSpec(NameWebFetch, KindTool, FamilyExternalEvidence, []string{RoleFetch})}

	if err := registry.Register(searchHandle); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchHandle); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	handle, err := registry.Handle(NameWebSearch)
	if err != nil {
		t.Fatalf("Handle() error = %v", err)
	}
	if handle.Spec().Name != NameWebSearch {
		t.Fatalf("unexpected handle spec: %+v", handle.Spec())
	}

	specs := registry.Specs()
	if len(specs) != 2 {
		t.Fatalf("expected 2 specs, got %+v", specs)
	}
	if specs[0].Name != NameWebFetch || specs[1].Name != NameWebSearch {
		t.Fatalf("expected stable sorted specs, got %+v", specs)
	}
	if !reflect.DeepEqual(registry.NamesByRole(RoleSearch), []string{NameWebSearch}) {
		t.Fatalf("unexpected role index: %+v", registry.NamesByRole(RoleSearch))
	}
	if !reflect.DeepEqual(registry.NamesByFamily(FamilyExternalEvidence), []string{NameWebFetch, NameWebSearch}) {
		t.Fatalf("unexpected family index: %+v", registry.NamesByFamily(FamilyExternalEvidence))
	}
}

func TestRegistry_RejectsDuplicateRegistration(t *testing.T) {
	registry := NewRegistry()
	handle := stubHandle{spec: validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})}
	if err := registry.Register(handle); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if err := registry.Register(handle); err == nil {
		t.Fatal("expected duplicate capability registration to fail")
	}
}

func TestRegistry_RejectsMissingSchemaAndDescription(t *testing.T) {
	registry := NewRegistry()
	handle := stubHandle{spec: Spec{
		Name:   NameWebSearch,
		Kind:   KindTool,
		Family: FamilyExternalEvidence,
		Roles:  []string{RoleSearch},
	}}
	if err := registry.Register(handle); err == nil {
		t.Fatal("expected missing description/schema registration to fail")
	}
}

func TestRegistry_RejectsMissingDependency(t *testing.T) {
	registry := NewRegistry()
	handle := stubHandle{spec: func() Spec {
		spec := validSpec(NameExternalEvidenceCollect, KindWorkflow, FamilyExternalEvidence, []string{RoleCollectExternalEvidence})
		spec.Dependencies = []string{NameWebSearch, NameWebFetch}
		return spec
	}()}
	if err := registry.Register(handle); err == nil {
		t.Fatal("expected missing dependency registration to fail")
	}
}

func TestRegistry_RejectsSelfDependency(t *testing.T) {
	registry := NewRegistry()
	handle := stubHandle{spec: func() Spec {
		spec := validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})
		spec.Dependencies = []string{NameWebSearch}
		return spec
	}()}
	if err := registry.Register(handle); err == nil {
		t.Fatal("expected self dependency registration to fail")
	}
}

func TestRegistry_RejectsUnsupportedMetadata(t *testing.T) {
	tests := []struct {
		name string
		spec Spec
	}{
		{
			name: "family",
			spec: func() Spec {
				spec := validSpec(NameWebSearch, KindTool, "custom_family", []string{RoleSearch})
				return spec
			}(),
		},
		{
			name: "role",
			spec: func() Spec {
				spec := validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{"custom_role"})
				return spec
			}(),
		},
		{
			name: "risk level",
			spec: func() Spec {
				spec := validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})
				spec.RiskLevel = "custom"
				return spec
			}(),
		},
		{
			name: "idempotency",
			spec: func() Spec {
				spec := validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})
				spec.Idempotency = "custom"
				return spec
			}(),
		},
		{
			name: "precondition requirement",
			spec: func() Spec {
				spec := validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})
				spec.Preconditions = []Precondition{{
					Field:       "query",
					Requirement: "custom",
					Description: "unsupported",
				}}
				return spec
			}(),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			registry := NewRegistry()
			if err := registry.Register(stubHandle{spec: tc.spec}); err == nil {
				t.Fatalf("expected registration with unsupported %s to fail", tc.name)
			}
		})
	}
}

func TestResolveBindingRequiresExplicitRoleWhenAmbiguous(t *testing.T) {
	registry := NewRegistry()
	primary := stubHandle{spec: validSpec(NameWebSearch, KindTool, FamilyExternalEvidence, []string{RoleSearch})}
	alternate := stubHandle{spec: validSpec("web_search_alt", KindTool, FamilyExternalEvidence, []string{RoleSearch})}
	if err := registry.Register(primary); err != nil {
		t.Fatalf("Register() primary error = %v", err)
	}
	if err := registry.Register(alternate); err != nil {
		t.Fatalf("Register() alternate error = %v", err)
	}
	if _, err := ResolveBinding(registry, nil, RoleSearch); err == nil {
		t.Fatal("expected ambiguous role resolution to fail")
	}
}

func validSpec(name, kind, family string, roles []string) Spec {
	return Spec{
		Name:             name,
		Kind:             kind,
		Family:           family,
		Roles:            roles,
		Description:      "valid test capability",
		InputSchema:      NewSchema(struct{ Query string }{}),
		OutputSchema:     NewSchema(struct{ Summary string }{}),
		RiskLevel:        RiskLevelLow,
		SupportsParallel: false,
		SupportsResume:   false,
		Idempotency:      IdempotencyUnknown,
	}
}
