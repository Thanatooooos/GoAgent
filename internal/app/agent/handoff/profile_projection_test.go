package handoff

import (
	"context"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
)

func TestBuildCapabilityProfilesProjectsSpecMetadata(t *testing.T) {
	registry := agentcapability.NewRegistry()

	searchCapability, err := agentsearch.NewCapability(
		stubSearchInvoker{},
		agentcapability.WithRiskLevel(agentcapability.RiskLevelMedium),
	)
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(
		stubFetchInvoker{},
		agentcapability.WithRequiresApproval(true),
		agentcapability.WithSupportsResume(true),
	)
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}
	if err := registry.Register(searchCapability); err != nil {
		t.Fatalf("Register() search error = %v", err)
	}
	if err := registry.Register(fetchCapability); err != nil {
		t.Fatalf("Register() fetch error = %v", err)
	}

	profiles := BuildCapabilityProfiles(registry, []NodeCapabilityBinding{
		{Node: "search", Capability: agentcapability.NameWebSearch},
		{Node: "fetch", Capability: agentcapability.NameWebFetch},
	})
	if len(profiles) != 2 {
		t.Fatalf("expected two profiles, got %+v", profiles)
	}

	if profiles[0].Node != "search" || profiles[0].Name != agentcapability.NameWebSearch {
		t.Fatalf("expected projected search profile identity, got %+v", profiles[0])
	}
	if profiles[0].WorkflowCapability != "search" || profiles[0].RiskLevel != agentcapability.RiskLevelMedium {
		t.Fatalf("expected projected search workflow metadata, got %+v", profiles[0])
	}

	if profiles[1].Node != "fetch" || profiles[1].Name != agentcapability.NameWebFetch {
		t.Fatalf("expected projected fetch profile identity, got %+v", profiles[1])
	}
	if profiles[1].WorkflowCapability != "search" || !profiles[1].RequiresApproval || !profiles[1].SupportsResume {
		t.Fatalf("expected projected fetch workflow metadata, got %+v", profiles[1])
	}
}

func TestBuildCapabilityProfilesSupportsWorkflowCapabilities(t *testing.T) {
	registry := agentcapability.NewRegistry()

	searchCapability, err := agentsearch.NewCapability(stubSearchInvoker{})
	if err != nil {
		t.Fatalf("search.NewCapability() error = %v", err)
	}
	fetchCapability, err := agentfetch.NewCapability(stubFetchInvoker{})
	if err != nil {
		t.Fatalf("fetch.NewCapability() error = %v", err)
	}
	workflowCapability, err := agentexternal.NewCapability(searchCapability, fetchCapability)
	if err != nil {
		t.Fatalf("external_evidence.NewCapability() error = %v", err)
	}
	for _, handle := range []agentcapability.Handle{searchCapability, fetchCapability, workflowCapability} {
		if err := registry.Register(handle); err != nil {
			t.Fatalf("Register() error = %v", err)
		}
	}

	profiles := BuildCapabilityProfiles(registry, []NodeCapabilityBinding{
		{Node: "external_evidence", Capability: agentcapability.NameExternalEvidenceCollect},
	})
	if len(profiles) != 1 {
		t.Fatalf("expected one workflow profile, got %+v", profiles)
	}
	if profiles[0].Kind != agentcapability.KindWorkflow || profiles[0].WorkflowCapability != "search" {
		t.Fatalf("expected workflow profile projection, got %+v", profiles[0])
	}
}

func TestBuildCapabilityProfilesMapsFamiliesToWorkflowCapabilities(t *testing.T) {
	cases := []struct {
		name     string
		family   string
		expected string
	}{
		{name: "external evidence", family: agentcapability.FamilyExternalEvidence, expected: "search"},
		{name: "document investigation", family: agentcapability.FamilyDocumentInvestigation, expected: "diagnosis"},
		{name: "trace investigation", family: agentcapability.FamilyTraceInvestigation, expected: "diagnosis"},
		{name: "discovery", family: agentcapability.FamilyDiscovery, expected: "knowledge"},
		{name: "meta", family: agentcapability.FamilyMeta, expected: "reasoning"},
		{name: "memory", family: agentcapability.FamilyMemory, expected: "memory"},
		{name: "generation", family: agentcapability.FamilyGeneration, expected: "generation"},
		{name: "unknown fallback", family: "custom", expected: "general"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			profile := capabilityProfileForNode("node", agentcapability.Spec{
				Name:   "test_capability",
				Kind:   agentcapability.KindWorkflow,
				Family: tc.family,
			})
			if profile.WorkflowCapability != tc.expected {
				t.Fatalf("expected workflow capability %q, got %+v", tc.expected, profile)
			}
		})
	}
}

type stubSearchInvoker struct{}

func (stubSearchInvoker) Search(_ context.Context, _ string) (agentsearch.SearchOutput, error) {
	return agentsearch.SearchOutput{}, nil
}

type stubFetchInvoker struct{}

func (stubFetchInvoker) Fetch(_ context.Context, _ []string) (agentfetch.Output, error) {
	return agentfetch.Output{}, nil
}
