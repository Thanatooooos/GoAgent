package planexecute

import (
	"context"
	"strings"
	"testing"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentcatalog "local/rag-project/internal/app/agent/capability/catalog"
	agentresolve "local/rag-project/internal/app/agent/capability/resolve"
	selectcapability "local/rag-project/internal/app/agent/capability/select"
	agentdocumentinvestigation "local/rag-project/internal/app/agent/document_investigation"
	agentexternalevidence "local/rag-project/internal/app/agent/external_evidence"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"
)

func TestDefaultPlanSynthesizer_SynthesizeTemplatePlan(t *testing.T) {
	synthesizer := newDefaultPlanSynthesizer(
		nil,
		agentcapability.Spec{RequiresApproval: true},
		agentcapability.Spec{},
		nil,
		nil,
		nil,
	)

	result, err := synthesizer.Synthesize(context.Background(), PlanSynthesisInput{
		Session: newSession("sess-synth-template", "golang generics", agentstate.OutputModeFinalAnswer),
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if result.Reasoning != "built linear search-then-fetch plan" {
		t.Fatalf("expected template reasoning, got %q", result.Reasoning)
	}
	if len(result.Plan.Steps) != 2 {
		t.Fatalf("expected two-step template plan, got %+v", result.Plan.Steps)
	}
	if !result.Plan.Steps[0].RequiresApproval {
		t.Fatalf("expected search step to inherit spec approval, got %+v", result.Plan.Steps[0])
	}
	if result.Plan.Steps[0].Query != "golang generics" {
		t.Fatalf("expected normalized query on search step, got %+v", result.Plan.Steps[0])
	}
}

func TestDefaultPlanSynthesizer_SynthesizeSelectorPlan(t *testing.T) {
	searchHandle, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(context.Context, string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}
	fetchHandle, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(context.Context, []string) (agentfetch.Output, error) {
			return agentfetch.Output{}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}
	registry := registerHandles(t, searchHandle, fetchHandle)
	synthesizer := newDefaultPlanSynthesizer(
		registry,
		agentcapability.Spec{},
		agentcapability.Spec{},
		agentcatalog.NewBuilder(),
		stubSelector{
			selectFn: func(_ context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
				if len(input.Capabilities) != 2 {
					t.Fatalf("expected full capability catalog, got %+v", input.Capabilities)
				}
				return selectcapability.SelectionOutput{
					Selections: []selectcapability.CapabilitySelection{
						{
							Name: agentcapability.NameWebFetch,
							Input: map[string]any{
								"urls": []string{"https://example.com/doc-77"},
							},
							Reason:     "Fetch the known source directly",
							Confidence: "high",
						},
					},
				}, nil
			},
		},
		agentresolve.NewRegistryResolver(registry),
	)

	result, err := synthesizer.Synthesize(context.Background(), PlanSynthesisInput{
		Session: newSession("sess-synth-selector", "why did doc-77 fail", agentstate.OutputModeFinalAnswer),
	})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if result.Reasoning != "built selector-driven plan around "+agentcapability.NameWebFetch {
		t.Fatalf("unexpected reasoning: %q", result.Reasoning)
	}
	if len(result.Plan.Steps) != 1 {
		t.Fatalf("expected one selector step, got %+v", result.Plan.Steps)
	}
	step := result.Plan.Steps[0]
	if step.CapabilityName != agentcapability.NameWebFetch {
		t.Fatalf("expected selected capability step, got %+v", step)
	}
	if step.CompletionPolicy != completionPolicyEvidence {
		t.Fatalf("expected evidence policy for evidence-producing capability, got %+v", step)
	}
	if got := toStringSlice(step.CapabilityInput["urls"]); len(got) != 1 || got[0] != "https://example.com/doc-77" {
		t.Fatalf("expected structured selector input, got %+v", step.CapabilityInput)
	}
}

func TestDefaultPlanSynthesizer_BackfillsExplicitWikiIDs(t *testing.T) {
	registry := registerHandles(t, stubPlanCapabilityHandle{spec: agentcapability.Spec{
		Name:             agentcapability.NameWikiWrite,
		Kind:             agentcapability.KindWorkflow,
		Family:           agentcapability.FamilyWiki,
		Roles:            []string{agentcapability.RoleWriteWiki},
		Description:      "Generates wiki pages from a knowledge document and persists them as a linked wiki.",
		InputSchema:      agentcapability.NewSchema(struct{}{}),
		OutputSchema:     agentcapability.NewSchema(struct{}{}),
		RiskLevel:        agentcapability.RiskLevelMedium,
		Idempotency:      agentcapability.IdempotencyBestEffort,
	}})
	synthesizer := newDefaultPlanSynthesizer(
		registry,
		agentcapability.Spec{},
		agentcapability.Spec{},
		agentcatalog.NewBuilder(),
		stubSelector{
			selectFn: func(context.Context, selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
				return selectcapability.SelectionOutput{Selections: []selectcapability.CapabilitySelection{{
					Name: agentcapability.NameWikiWrite,
				}}}, nil
			},
		},
		agentresolve.NewRegistryResolver(registry),
	)

	result, err := synthesizer.Synthesize(context.Background(), PlanSynthesisInput{Session: newSession(
		"sess-synth-wiki-input",
		"请调用 wiki_write，knowledge_base_id=kb_demo_01，document_id=doc_ok_01。",
		agentstate.OutputModeFinalAnswer,
	)})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if got := result.Plan.Steps[0].CapabilityInput["knowledge_base_id"]; got != "kb_demo_01" {
		t.Fatalf("expected knowledge base id to be backfilled, got %#v", got)
	}
	if got := result.Plan.Steps[0].CapabilityInput["document_id"]; got != "doc_ok_01" {
		t.Fatalf("expected document id to be backfilled, got %#v", got)
	}
}

func TestDefaultPlanSynthesizer_UsesExplicitWikiRequestWhenSelectorReturnsEmpty(t *testing.T) {
	registry := registerHandles(t, stubPlanCapabilityHandle{spec: agentcapability.Spec{
		Name:             agentcapability.NameWikiWrite,
		Kind:             agentcapability.KindWorkflow,
		Family:           agentcapability.FamilyWiki,
		Roles:            []string{agentcapability.RoleWriteWiki},
		Description:      "Generates wiki pages from a knowledge document and persists them as a linked wiki.",
		InputSchema:      agentcapability.NewSchema(struct{}{}),
		OutputSchema:     agentcapability.NewSchema(struct{}{}),
		RiskLevel:        agentcapability.RiskLevelMedium,
		Idempotency:      agentcapability.IdempotencyBestEffort,
	}})
	synthesizer := newDefaultPlanSynthesizer(
		registry,
		agentcapability.Spec{},
		agentcapability.Spec{},
		agentcatalog.NewBuilder(),
		stubSelector{selectFn: func(context.Context, selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
			return selectcapability.SelectionOutput{}, nil
		}},
		agentresolve.NewRegistryResolver(registry),
	)

	result, err := synthesizer.Synthesize(context.Background(), PlanSynthesisInput{Session: newSession(
		"sess-synth-wiki-explicit",
		"请直接调用 wiki_write，knowledge_base_id=kb_demo_01，document_id=doc_ok_01，生成并写入 Wiki 页面。",
		agentstate.OutputModeFinalAnswer,
	)})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(result.Plan.Steps) != 1 || result.Plan.Steps[0].CapabilityName != agentcapability.NameWikiWrite {
		t.Fatalf("expected explicit wiki plan, got %+v", result.Plan.Steps)
	}
}

func TestDefaultPlanSynthesizer_ExplicitWikiRequestWinsOverSelector(t *testing.T) {
	wikiSpec := agentcapability.Spec{
		Name:             agentcapability.NameWikiWrite,
		Kind:             agentcapability.KindWorkflow,
		Family:           agentcapability.FamilyWiki,
		Roles:            []string{agentcapability.RoleWriteWiki},
		Description:      "Generates wiki pages from a knowledge document and persists them as a linked wiki.",
		InputSchema:      agentcapability.NewSchema(struct{}{}),
		OutputSchema:     agentcapability.NewSchema(struct{}{}),
		RiskLevel:        agentcapability.RiskLevelMedium,
		Idempotency:      agentcapability.IdempotencyBestEffort,
	}
	fetchHandle, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(context.Context, []string) (agentfetch.Output, error) {
			return agentfetch.Output{}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}
	registry := registerHandles(t, stubPlanCapabilityHandle{spec: wikiSpec}, fetchHandle)
	synthesizer := newDefaultPlanSynthesizer(
		registry,
		agentcapability.Spec{},
		agentcapability.Spec{},
		agentcatalog.NewBuilder(),
		// The selector would pick web_fetch, but the question explicitly
		// requests wiki_write with concrete IDs — the explicit request must win.
		stubSelector{selectFn: func(context.Context, selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
			return selectcapability.SelectionOutput{Selections: []selectcapability.CapabilitySelection{{
				Name: agentcapability.NameWebFetch,
				Input: map[string]any{
					"urls": []string{"https://example.com/ignored"},
				},
			}}}, nil
		}},
		agentresolve.NewRegistryResolver(registry),
	)

	result, err := synthesizer.Synthesize(context.Background(), PlanSynthesisInput{Session: newSession(
		"sess-synth-wiki-priority",
		"请调用 wiki_write，knowledge_base_id=31794389852418305，document_id=31794655553188097，生成 wiki 页面。",
		agentstate.OutputModeFinalAnswer,
	)})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if len(result.Plan.Steps) != 1 || result.Plan.Steps[0].CapabilityName != agentcapability.NameWikiWrite {
		t.Fatalf("expected explicit wiki_write plan to win over selector, got %+v", result.Plan.Steps)
	}
	if got := result.Plan.Steps[0].CapabilityInput["document_id"]; got != "31794655553188097" {
		t.Fatalf("expected document id carried through, got %#v", got)
	}
}

func TestDefaultPlanSynthesizer_SynthesizeMixedPlan(t *testing.T) {
	searchHandle, err := agentsearch.NewCapability(stubSearchInvoker{
		search: func(context.Context, string) (agentsearch.SearchOutput, error) {
			return agentsearch.SearchOutput{}, nil
		},
	})
	if err != nil {
		t.Fatalf("search capability: %v", err)
	}
	fetchHandle, err := agentfetch.NewCapability(stubFetchInvoker{
		fetch: func(context.Context, []string) (agentfetch.Output, error) {
			return agentfetch.Output{}, nil
		},
	})
	if err != nil {
		t.Fatalf("fetch capability: %v", err)
	}
	documentHandle, err := agentdocumentinvestigation.NewCapability(stubDocumentInvestigator{
		getFn: func(context.Context, knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error) {
			return knowledgedomain.KnowledgeDocument{}, nil
		},
		pageLogsFn: func(context.Context, knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error) {
			return knowledgeservice.KnowledgeDocumentChunkLogPageResult{}, nil
		},
	})
	if err != nil {
		t.Fatalf("document capability: %v", err)
	}
	externalHandle, err := agentexternalevidence.NewCapability(searchHandle, fetchHandle)
	if err != nil {
		t.Fatalf("external evidence capability: %v", err)
	}
	registry := registerHandles(t, searchHandle, fetchHandle, documentHandle, externalHandle)
	synthesizer := newDefaultPlanSynthesizer(
		registry,
		agentcapability.Spec{},
		agentcapability.Spec{},
		agentcatalog.NewBuilder(),
		stubSelector{
			selectFn: func(_ context.Context, input selectcapability.SelectionInput) (selectcapability.SelectionOutput, error) {
				return selectcapability.SelectionOutput{
					Selections: []selectcapability.CapabilitySelection{
						{
							Family: agentcapability.FamilyDocumentInvestigation,
							Role:   agentcapability.RoleInvestigateDocument,
							Input: map[string]any{
								"document_id": "doc-77",
							},
							Reason:     "Investigate the internal document directly",
							Confidence: "high",
						},
					},
				}, nil
			},
		},
		agentresolve.NewRegistryResolver(registry),
	)

	session := newSession("sess-synth-mixed", "why did doc-77 fail", agentstate.OutputModeFinalAnswer)
	session.Snapshot.Request.RuntimeOptions.AllowWebSearch = true
	result, err := synthesizer.Synthesize(context.Background(), PlanSynthesisInput{Session: session})
	if err != nil {
		t.Fatalf("Synthesize() error = %v", err)
	}
	if !strings.Contains(result.Reasoning, "mixed-capability") {
		t.Fatalf("expected mixed reasoning, got %q", result.Reasoning)
	}
	if len(result.Plan.Steps) != 2 {
		t.Fatalf("expected two-step mixed plan, got %+v", result.Plan.Steps)
	}
	if result.Plan.Steps[0].CapabilityName != agentcapability.NameDocumentInvestigation {
		t.Fatalf("expected first mixed step to investigate document, got %+v", result.Plan.Steps[0])
	}
	if result.Plan.Steps[1].CapabilityName != agentcapability.NameExternalEvidenceCollect {
		t.Fatalf("expected second mixed step to collect external evidence, got %+v", result.Plan.Steps[1])
	}
	if !contains(result.Plan.Steps[1].Consumes, artifactKindStructuredOutput) {
		t.Fatalf("expected mixed follow-up step to consume structured output, got %+v", result.Plan.Steps[1])
	}
}

type stubPlanSynthesizer struct {
	result PlanSynthesisResult
	err    error
}

func (s stubPlanSynthesizer) Synthesize(context.Context, PlanSynthesisInput) (PlanSynthesisResult, error) {
	return s.result, s.err
}

func TestBuildPlanNode_UsesSynthesizerOutput(t *testing.T) {
	node, err := newBuildPlanNode(stubPlanSynthesizer{
		result: PlanSynthesisResult{
			Plan: agentstate.PlanState{
				PlanID: "plan_custom",
				Steps: []agentstate.PlanStep{
					{
						StepID: "step_custom",
						Query:  "refactored query",
					},
				},
			},
			Reasoning: "built custom plan",
			Notes:     []string{"custom plan note"},
		},
	})
	if err != nil {
		t.Fatalf("newBuildPlanNode() error = %v", err)
	}

	result, err := node.Run(context.Background(), &agentruntime.RuntimeSession{
		Snapshot: agentstate.StateSnapshot{
			Plan: agentstate.PlanState{
				ReplanCount: 2,
			},
		},
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Decision == nil || result.Decision.Reasoning != "built custom plan" {
		t.Fatalf("expected custom reasoning, got %+v", result.Decision)
	}
	if result.Delta.Plan == nil || result.Delta.Plan.Replace == nil {
		t.Fatalf("expected plan replacement delta, got %+v", result.Delta.Plan)
	}
	if result.Delta.Plan.Replace.ReplanCount != 2 {
		t.Fatalf("expected existing replan count to be preserved, got %+v", result.Delta.Plan.Replace)
	}
	if result.Delta.Context == nil || result.Delta.Context.SearchQuery == nil || *result.Delta.Context.SearchQuery != "refactored query" {
		t.Fatalf("expected search query projection from synthesized first step, got %+v", result.Delta.Context)
	}
	if len(result.Delta.Context.Notes) != 1 || result.Delta.Context.Notes[0] != "custom plan note" {
		t.Fatalf("expected synthesized notes, got %+v", result.Delta.Context)
	}
}
