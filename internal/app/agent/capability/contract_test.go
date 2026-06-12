package capability_test

import (
	"context"
	"errors"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentcontentsummarize "local/rag-project/internal/app/agent/content_summarize"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentexternal "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentknowledgediscovery "local/rag-project/internal/app/agent/knowledge_discovery"
	agentmemoryrecall "local/rag-project/internal/app/agent/memory_recall"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentthink "local/rag-project/internal/app/agent/think"
	longtermmemory "local/rag-project/internal/app/rag/service/longtermmemory"
	"local/rag-project/internal/framework/convention"
	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
)

func TestDecodeAndValidateInputSupportsTypedAndStructuredValues(t *testing.T) {
	spec := validSpec(agentcapability.NameWebSearch, agentcapability.KindTool, agentcapability.FamilyExternalEvidence, []string{agentcapability.RoleSearch})

	typed, err := agentcapability.DecodeAndValidateInput[agentsearch.CapabilityInput](spec, agentsearch.CapabilityInput{Query: "golang"}, "search input is required", "search input")
	if err != nil {
		t.Fatalf("DecodeAndValidateInput(typed) error = %v", err)
	}
	if typed.Query != "golang" {
		t.Fatalf("expected typed query to round-trip, got %+v", typed)
	}

	structured, err := agentcapability.DecodeAndValidateInput[agentsearch.CapabilityInput](spec, map[string]any{"query": "go"}, "search input is required", "search input")
	if err != nil {
		t.Fatalf("DecodeAndValidateInput(structured) error = %v", err)
	}
	if structured.Query != "go" {
		t.Fatalf("expected structured query to decode, got %+v", structured)
	}
}

func TestDecodeAndValidateInputRejectsUnexpectedTypeAndPreconditions(t *testing.T) {
	spec := validSpec(agentcapability.NameWebSearch, agentcapability.KindTool, agentcapability.FamilyExternalEvidence, []string{agentcapability.RoleSearch})
	spec.Preconditions = []agentcapability.Precondition{{
		Field:       "query",
		Requirement: agentcapability.PreconditionRequirementNonEmpty,
		Description: "query is required",
	}}

	if _, err := agentcapability.DecodeAndValidateInput[agentsearch.CapabilityInput](spec, map[string]any{"query": 1}, "search input is required", "search input"); err == nil {
		t.Fatal("expected unexpected type to fail")
	}
	if _, err := agentcapability.DecodeAndValidateInput[agentsearch.CapabilityInput](spec, map[string]any{"query": "   "}, "search input is required", "search input"); !agentcapability.IsPreconditionError(err) {
		t.Fatalf("expected precondition error, got %v", err)
	}
}

func TestCatalogBuilderBuildsCardsWithHints(t *testing.T) {
	registry := agentcapability.NewRegistry()
	spec := validSpec(agentcapability.NameWebSearch, agentcapability.KindTool, agentcapability.FamilyExternalEvidence, []string{agentcapability.RoleSearch})
	spec.Preconditions = []agentcapability.Precondition{{
		Field:       "query",
		Requirement: agentcapability.PreconditionRequirementNonEmpty,
		Description: "Search requires a non-empty normalized query.",
	}}
	if err := registry.Register(stubHandle{spec: spec}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	cards, err := agentcatalog.NewBuilder().Build(registry)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(cards) != 1 {
		t.Fatalf("expected one card, got %+v", cards)
	}
	if cards[0].Name != agentcapability.NameWebSearch || cards[0].Summary == "" || len(cards[0].InputHints) == 0 {
		t.Fatalf("unexpected card projection: %+v", cards[0])
	}
}

func TestRegistryResolverRejectsAmbiguousSelection(t *testing.T) {
	registry := agentcapability.NewRegistry()
	if err := registry.Register(stubHandle{spec: validSpec(agentcapability.NameWebSearch, agentcapability.KindTool, agentcapability.FamilyExternalEvidence, []string{agentcapability.RoleSearch})}); err != nil {
		t.Fatalf("Register(search) error = %v", err)
	}
	if err := registry.Register(stubHandle{spec: validSpec(agentcapability.NameWebFetch, agentcapability.KindTool, agentcapability.FamilyExternalEvidence, []string{agentcapability.RoleFetch})}); err != nil {
		t.Fatalf("Register(fetch) error = %v", err)
	}

	resolver := agentresolve.NewRegistryResolver(registry)
	_, err := resolver.Resolve(selectcapability.CapabilitySelection{Family: agentcapability.FamilyExternalEvidence})
	var ambiguous agentresolve.AmbiguousError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected ambiguous error, got %v", err)
	}
}

func TestExistingCapabilitiesSatisfySharedContract(t *testing.T) {
	externalEvidenceHandle, externalEvidencePrerequisites := mustExternalEvidenceContractSetup(t)

	tests := []struct {
		name                string
		handle              agentcapability.Handle
		prerequisites       []agentcapability.Handle
		validInput          map[string]any
		invalidInput        map[string]any
		assertResolvedInput func(t *testing.T, input any)
		assertResult        func(t *testing.T, result agentcapability.InvocationResult)
	}{
		{
			name: "search",
			handle: mustSearchCapabilityHandle(t, contractSearchInvoker{
				output: agentsearch.SearchOutput{
					Query:   "golang",
					Summary: "found result",
					URLs:    []string{"https://go.dev"},
					Results: []agentsearch.SearchResultItem{{
						Title:   "Go",
						URL:     "https://go.dev",
						Snippet: "The Go Programming Language",
						Domain:  "go.dev",
					}},
				},
			}),
			validInput:   map[string]any{"query": "golang"},
			invalidInput: map[string]any{"query": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentsearch.CapabilityInput)
				if !ok || typed.Query != "golang" {
					t.Fatalf("expected search input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameWebSearch {
					t.Fatalf("unexpected search result: %+v", result)
				}
			},
		},
		{
			name: "fetch",
			handle: mustFetchCapabilityHandle(t, contractFetchInvoker{
				output: agentfetch.Output{
					Summary: "fetched page",
					Pages: []agentfetch.PageResult{{
						URL:  "https://go.dev",
						Text: "The Go Programming Language",
					}},
				},
			}),
			validInput:   map[string]any{"urls": []string{"https://go.dev"}},
			invalidInput: map[string]any{"urls": "https://go.dev"},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentfetch.CapabilityInput)
				if !ok || len(typed.URLs) != 1 || typed.URLs[0] != "https://go.dev" {
					t.Fatalf("expected fetch input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameWebFetch {
					t.Fatalf("unexpected fetch result: %+v", result)
				}
			},
		},
		{
			name:          "external evidence",
			handle:        externalEvidenceHandle,
			prerequisites: externalEvidencePrerequisites,
			validInput:    map[string]any{"query": "golang"},
			invalidInput:  map[string]any{"query": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentexternal.CapabilityInput)
				if !ok || typed.Query != "golang" {
					t.Fatalf("expected external evidence input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameExternalEvidenceCollect {
					t.Fatalf("unexpected external evidence result: %+v", result)
				}
			},
		},
		{
			name:         "document investigation",
			handle:       mustDocumentInvestigationCapabilityHandle(t),
			validInput:   map[string]any{"document_id": "doc-1"},
			invalidInput: map[string]any{"document_id": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentdocumentinvestigation.CapabilityInput)
				if !ok || typed.DocumentID != "doc-1" {
					t.Fatalf("expected document investigation input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameDocumentInvestigation {
					t.Fatalf("unexpected document investigation result: %+v", result)
				}
			},
		},
		{
			name:         "think",
			handle:       mustThinkCapabilityHandle(t),
			validInput:   map[string]any{"thought": "plan next step"},
			invalidInput: map[string]any{"thought": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentthink.CapabilityInput)
				if !ok || typed.Thought != "plan next step" {
					t.Fatalf("expected think input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameThink {
					t.Fatalf("unexpected think result: %+v", result)
				}
			},
		},
		{
			name:         "knowledge discovery",
			handle:       mustKnowledgeDiscoveryCapabilityHandle(t),
			validInput:   map[string]any{"action": agentknowledgediscovery.ActionListBases},
			invalidInput: map[string]any{"action": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentknowledgediscovery.CapabilityInput)
				if !ok || typed.Action != agentknowledgediscovery.ActionListBases {
					t.Fatalf("expected knowledge discovery input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameKnowledgeDiscovery {
					t.Fatalf("unexpected knowledge discovery result: %+v", result)
				}
			},
		},
		{
			name:         "memory recall",
			handle:       mustMemoryRecallCapabilityHandle(t),
			validInput:   map[string]any{"query": "preference", "user_id": "u1"},
			invalidInput: map[string]any{"query": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentmemoryrecall.CapabilityInput)
				if !ok || typed.Query != "preference" || typed.UserID != "u1" {
					t.Fatalf("expected memory recall input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameMemoryRecall {
					t.Fatalf("unexpected memory recall result: %+v", result)
				}
			},
		},
		{
			name:         "content summarize",
			handle:       mustContentSummarizeCapabilityHandle(t),
			validInput:   map[string]any{"content": "long content"},
			invalidInput: map[string]any{"content": 1},
			assertResolvedInput: func(t *testing.T, input any) {
				typed, ok := input.(agentcontentsummarize.CapabilityInput)
				if !ok || typed.Content != "long content" {
					t.Fatalf("expected content summarize input, got %#v", input)
				}
			},
			assertResult: func(t *testing.T, result agentcapability.InvocationResult) {
				if result.Status != agentcapability.StatusSucceeded || result.Action.Name != agentcapability.NameContentSummarize {
					t.Fatalf("unexpected content summarize result: %+v", result)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			runCapabilityContract(t, tc.handle, tc.prerequisites, tc.validInput, tc.invalidInput, tc.assertResolvedInput, tc.assertResult)
		})
	}
}

func runCapabilityContract(t *testing.T, handle agentcapability.Handle, prerequisites []agentcapability.Handle, validInput map[string]any, invalidInput map[string]any, assertResolvedInput func(t *testing.T, input any), assertResult func(t *testing.T, result agentcapability.InvocationResult)) {
	t.Helper()

	registry := agentcapability.NewRegistry()
	for _, prerequisite := range prerequisites {
		if err := registry.Register(prerequisite); err != nil {
			t.Fatalf("Register(prerequisite) error = %v", err)
		}
	}
	if err := registry.Register(handle); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	cards, err := agentcatalog.NewBuilder().Build(registry)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	var matchedCard *agentcatalog.Card
	for idx := range cards {
		if cards[idx].Name == handle.Spec().Name {
			matchedCard = &cards[idx]
			break
		}
	}
	if matchedCard == nil {
		t.Fatalf("expected projected card for %q, got %+v", handle.Spec().Name, cards)
	}
	if matchedCard.Summary == "" {
		t.Fatalf("unexpected projected card: %+v", *matchedCard)
	}

	resolver := agentresolve.NewRegistryResolver(registry)
	spec := handle.Spec()
	selections := []selectcapability.CapabilitySelection{
		{Name: spec.Name, Input: validInput},
	}
	if len(registry.NamesByFamily(spec.Family)) == 1 {
		selections = append(selections, selectcapability.CapabilitySelection{Family: spec.Family, Kind: spec.Kind, Input: validInput})
	}
	if len(spec.Roles) > 0 && len(registry.NamesByRole(spec.Roles[0])) == 1 {
		selections = append(selections, selectcapability.CapabilitySelection{Role: spec.Roles[0], Kind: spec.Kind, Input: validInput})
	}
	for _, selection := range selections {
		resolved, err := resolver.Resolve(selection)
		if err != nil {
			t.Fatalf("Resolve(%+v) error = %v", selection, err)
		}
		if resolved.Name != spec.Name {
			t.Fatalf("expected resolved capability %q, got %+v", spec.Name, resolved)
		}
		assertResolvedInput(t, resolved.Input)
		result, err := resolved.Handle.Invoke(context.Background(), agentcapability.InvocationRequest{Input: resolved.Input})
		if err != nil {
			t.Fatalf("Invoke() error = %v", err)
		}
		assertResult(t, result)
	}

	_, err = resolver.Resolve(selectcapability.CapabilitySelection{
		Name:  spec.Name,
		Input: invalidInput,
	})
	var invalidInputErr agentresolve.InvalidInputError
	if !errors.As(err, &invalidInputErr) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func mustCapabilityHandle(t *testing.T, handle agentcapability.Handle, err error) agentcapability.Handle {
	t.Helper()
	if err != nil {
		t.Fatalf("NewCapability() error = %v", err)
	}
	return handle
}

func mustSearchCapabilityHandle(t *testing.T, invoker contractSearchInvoker) agentcapability.Handle {
	t.Helper()
	handle, err := agentsearch.NewCapability(invoker)
	return mustCapabilityHandle(t, handle, err)
}

func mustFetchCapabilityHandle(t *testing.T, invoker contractFetchInvoker) agentcapability.Handle {
	t.Helper()
	handle, err := agentfetch.NewCapability(invoker)
	return mustCapabilityHandle(t, handle, err)
}

func mustDocumentInvestigationCapabilityHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentdocumentinvestigation.NewCapability(contractInvestigator{})
	return mustCapabilityHandle(t, handle, err)
}

func mustThinkCapabilityHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentthink.NewCapability()
	return mustCapabilityHandle(t, handle, err)
}

func mustKnowledgeDiscoveryCapabilityHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentknowledgediscovery.NewCapability(contractDiscoverer{})
	return mustCapabilityHandle(t, handle, err)
}

func mustMemoryRecallCapabilityHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentmemoryrecall.NewCapability(contractRecaller{})
	return mustCapabilityHandle(t, handle, err)
}

func mustContentSummarizeCapabilityHandle(t *testing.T) agentcapability.Handle {
	t.Helper()
	handle, err := agentcontentsummarize.NewCapability(contractCompleter{})
	return mustCapabilityHandle(t, handle, err)
}

func mustExternalEvidenceContractSetup(t *testing.T) (agentcapability.Handle, []agentcapability.Handle) {
	t.Helper()
	searchHandle := mustSearchCapabilityHandle(t, contractSearchInvoker{
		output: agentsearch.SearchOutput{
			Query:   "golang",
			Summary: "found result",
			URLs:    []string{"https://go.dev"},
			Results: []agentsearch.SearchResultItem{{
				Title:   "Go",
				URL:     "https://go.dev",
				Snippet: "The Go Programming Language",
				Domain:  "go.dev",
			}},
		},
	})
	fetchHandle := mustFetchCapabilityHandle(t, contractFetchInvoker{
		output: agentfetch.Output{
			Summary: "fetched page",
			Pages: []agentfetch.PageResult{{
				URL:  "https://go.dev",
				Text: "The Go Programming Language",
			}},
		},
	})
	handle, err := agentexternal.NewCapability(searchHandle, fetchHandle)
	return mustCapabilityHandle(t, handle, err), []agentcapability.Handle{searchHandle, fetchHandle}
}

type stubHandle struct {
	spec agentcapability.Spec
}

func (h stubHandle) Spec() agentcapability.Spec {
	return h.spec
}

func (h stubHandle) Invoke(_ context.Context, _ agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	return agentcapability.InvocationResult{
		Action: agentcapability.ActionRecord{
			Name:    h.spec.Name,
			Summary: "stub invoke",
		},
		Observation: agentcapability.ObservationRecord{
			Summary: "stub result",
		},
		Status: agentcapability.StatusSucceeded,
	}, nil
}

func validSpec(name, kind, family string, roles []string) agentcapability.Spec {
	return agentcapability.Spec{
		Name:             name,
		Kind:             kind,
		Family:           family,
		Roles:            roles,
		Description:      "valid test capability",
		InputSchema:      agentcapability.NewSchema(struct{ Query string }{}),
		OutputSchema:     agentcapability.NewSchema(struct{ Summary string }{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: false,
		SupportsResume:   false,
		Idempotency:      agentcapability.IdempotencyUnknown,
	}
}

type contractSearchInvoker struct {
	output agentsearch.SearchOutput
	err    error
}

func (s contractSearchInvoker) Search(_ context.Context, _ string) (agentsearch.SearchOutput, error) {
	return s.output, s.err
}

type contractFetchInvoker struct {
	output agentfetch.Output
	err    error
}

func (s contractFetchInvoker) Fetch(_ context.Context, _ []string) (agentfetch.Output, error) {
	return s.output, s.err
}

type contractInvestigator struct{}

func (contractInvestigator) Get(context.Context, knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
	return knowledgedomain.KnowledgeDocument{
		ID:          "doc-1",
		Name:        "Product Spec",
		ProcessMode: knowledgedomain.KnowledgeDocumentProcessModePipeline,
		Status:      knowledgedomain.KnowledgeDocumentStatusFailed,
		PipelineID:  "pipe-1",
		ChunkCount:  0,
	}, nil
}

type contractDiscoverer struct{}

func (contractDiscoverer) PageBases(context.Context, knowledgeservice.PageKnowledgeBaseInput) (knowledgeservice.KnowledgeBasePageResult, error) {
	return knowledgeservice.KnowledgeBasePageResult{
		Items: []knowledgedomain.KnowledgeBase{{ID: "kb-1", Name: "Eval KB"}},
		DocumentCounts: map[string]int{
			"kb-1": 1,
		},
		Total: 1,
		Page:  1,
	}, nil
}

func (contractDiscoverer) PageDocuments(context.Context, knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error) {
	return knowledgeservice.KnowledgeDocumentPageResult{
		Items: []knowledgedomain.KnowledgeDocument{{ID: "doc-1", Name: "Doc 1", KnowledgeBaseID: "kb-1"}},
		Total: 1,
		Page:  1,
	}, nil
}

func (contractDiscoverer) SearchDocuments(context.Context, knowledgeservice.SearchKnowledgeDocumentsInput) ([]knowledgeservice.KnowledgeDocumentSearchItem, error) {
	return []knowledgeservice.KnowledgeDocumentSearchItem{
		{ID: "doc-1", Name: "Doc 1", KnowledgeBaseID: "kb-1"},
	}, nil
}

type contractRecaller struct{}

func (contractRecaller) RecallMemories(context.Context, longtermmemory.RecallMemoriesInput) (longtermmemory.RecallMemoriesResult, error) {
	return longtermmemory.RecallMemoriesResult{
		Used:           true,
		Context:        "memory context",
		SelectedCount:  1,
		CandidateCount: 1,
		SelectedMemoryIDs: []string{
			"mem-1",
		},
		SelectedEntries: []longtermmemory.RecallMemoryEntry{
			{ID: "mem-1", MemoryType: "knowledge", Summary: "user prefers zh", FinalScore: 8},
		},
	}, nil
}

type contractCompleter struct{}

func (contractCompleter) ChatWithRequest(convention.ChatRequest) (string, error) {
	return "summary text", nil
}

func (contractInvestigator) PageChunkLogs(context.Context, knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
	return knowledgeservice.KnowledgeDocumentChunkLogPageResult{
		Items: []knowledgeservice.KnowledgeDocumentChunkLogItem{{
			Log: knowledgedomain.KnowledgeDocumentChunkLog{
				Status:       knowledgedomain.KnowledgeDocumentChunkLogStatusFailed,
				ChunkCount:   0,
				ErrorMessage: "pipeline failed",
			},
			IngestionTask: &ingestiondomain.Task{
				ID:     "task-1",
				Status: ingestiondomain.TaskStatusFailed,
			},
			IngestionNodes: []ingestiondomain.TaskNode{{
				NodeID:       "indexer",
				Status:       ingestiondomain.TaskStatusFailed,
				ErrorMessage: "connection refused",
			}},
		}},
	}, nil
}
