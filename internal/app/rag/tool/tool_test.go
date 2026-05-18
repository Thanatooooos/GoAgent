package tool

import (
	"context"
	"errors"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"
	"local/rag-project/internal/framework/convention"
)

type toolStub struct {
	definition Definition
	result     Result
	err        error
	lastCall   Call
}

func TestBuildAnswerGuidancePrefersDeeperNodeEvidence(t *testing.T) {
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:   "document_ingestion_diagnose",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"conclusion":   "document ingestion failed, but no failed node was captured",
				"confidence":   "medium",
				"facts":        []string{"文档当前状态为失败。", "最近一次关联任务为 task_fail_01。"},
				"inferences":   []string{"推断：失败发生在某个尚未被完整记录的阶段。"},
				"nextActions":  []string{"check task log"},
				"latestTaskId": "task_fail_01",
			},
		},
		{
			Name:   "ingestion_task_node_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId":       "task_fail_01",
				"nodeId":       "indexer",
				"nodeOrder":    4,
				"status":       "failed",
				"durationMs":   5210,
				"errorMessage": "connection refused: vector store unavailable",
			},
		},
	})
	if !strings.Contains(guidance, "失败发生在 indexer 节点") {
		t.Fatalf("expected upgraded conclusion from node evidence, got %q", guidance)
	}
	if !strings.Contains(guidance, "connection refused: vector store unavailable") {
		t.Fatalf("expected node error to appear in guidance, got %q", guidance)
	}
	if !strings.Contains(guidance, "当前置信度：high") {
		t.Fatalf("expected confidence to be upgraded to high, got %q", guidance)
	}
	if strings.Contains(guidance, "尚未被完整记录") {
		t.Fatalf("expected stale inference to be replaced, got %q", guidance)
	}
}

func TestAgentStatePromptStringIncludesStructuredHintCalls(t *testing.T) {
	state := AgentState{
		Phase: "deep_dive",
		NextHintCalls: []HintCall{{
			Name: "ingestion_task_query",
			Arguments: map[string]any{
				"taskId":       "task-1",
				"includeNodes": true,
			},
		}},
	}.Normalize()

	prompt := state.PromptString()
	if !strings.Contains(prompt, "\"nextHintCalls\"") {
		t.Fatalf("expected prompt to include nextHintCalls, got %q", prompt)
	}
	if !strings.Contains(prompt, "\"name\":\"ingestion_task_query\"") {
		t.Fatalf("expected prompt to include hint call name, got %q", prompt)
	}
	if state.NextHint != "tool:ingestion_task_query|taskId=task-1|includeNodes=true" {
		t.Fatalf("expected legacy nextHint to remain available, got %q", state.NextHint)
	}
}

func (s *toolStub) Definition() Definition {
	return s.definition
}

func (s *toolStub) Invoke(ctx context.Context, call Call) (Result, error) {
	s.lastCall = call
	return s.result, s.err
}

func TestDefinitionValidate(t *testing.T) {
	if err := (Definition{}).Validate(); err == nil {
		t.Fatal("expected error for empty tool name")
	}

	err := (Definition{
		Name: "document_query",
		Parameters: []ParameterDefinition{
			{Name: "", Type: ParamTypeString},
		},
	}).Validate()
	if err == nil {
		t.Fatal("expected error for empty parameter name")
	}
}

func TestRegistryRegisterAndListDefinitions(t *testing.T) {
	registry := NewRegistry()
	docTool := &toolStub{
		definition: Definition{Name: "document_query", Description: "query document"},
	}
	traceTool := &toolStub{
		definition: Definition{Name: "trace_node_query", Description: "query trace node"},
	}

	if err := registry.Register(traceTool); err != nil {
		t.Fatalf("register trace tool: %v", err)
	}
	if err := registry.Register(docTool); err != nil {
		t.Fatalf("register doc tool: %v", err)
	}
	if err := registry.Register(docTool); err == nil {
		t.Fatal("expected duplicate register error")
	}

	items := registry.ListDefinitions()
	if len(items) != 2 {
		t.Fatalf("expected 2 definitions, got %d", len(items))
	}
	if items[0].Name != "document_query" || items[1].Name != "trace_node_query" {
		t.Fatalf("expected sorted definitions, got %+v", items)
	}
}

func TestRegistryRegisterModuleAndGetSpec(t *testing.T) {
	registry := NewRegistry()
	module := NewLegacyToolAdapterWithSpec(&toolStub{
		definition: Definition{Name: "web_search", Description: "search web", ReadOnly: true},
	}, ToolSpec{
		Capability:          CapabilitySearch,
		EvidenceSources:     []string{EvidenceSourceExternalWeb},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "web",
	}).Module()

	if err := registry.RegisterModule(module); err != nil {
		t.Fatalf("register module: %v", err)
	}
	if _, ok := registry.GetModule("web_search"); !ok {
		t.Fatal("expected module lookup to succeed")
	}
	spec, ok := registry.GetSpec("web_search")
	if !ok {
		t.Fatal("expected spec lookup to succeed")
	}
	if spec.Capability != CapabilitySearch {
		t.Fatalf("unexpected capability: %q", spec.Capability)
	}
}

func TestLegacyRegisterInfersKnownBehavior(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(&toolStub{
		definition: Definition{
			Name:        "document_query",
			Description: "query document",
			ReadOnly:    true,
			Parameters:  []ParameterDefinition{{Name: "documentId", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:   "document_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"documentId":  "doc-1",
				"status":      "failed",
				"processMode": "pipeline",
			},
		},
	})

	behavior, ok := registry.GetBehavior("document_query")
	if !ok || behavior.Next == nil || behavior.Observe == nil {
		t.Fatalf("expected inferred legacy behavior, got ok=%v behavior=%+v", ok, behavior)
	}

	decision := nextDecisionWithRegistry(registry, WorkflowInput{}, Result{
		Name:   "document_query",
		Status: CallStatusSuccess,
		Data: map[string]any{
			"documentId":  "doc-1",
			"status":      "failed",
			"processMode": "pipeline",
		},
	})
	if decision.Done || len(decision.HintCalls) != 1 || decision.HintCalls[0].Name != "document_ingestion_diagnose" {
		t.Fatalf("expected inferred behavior-driven continuation, got %+v", decision)
	}
}

func TestExecutorExecuteSuccess(t *testing.T) {
	registry := NewRegistry()
	tool := &toolStub{
		definition: Definition{Name: "document_query", Description: "query document"},
		result: Result{
			Summary: "matched doc-1",
			Data: map[string]any{
				"documentId": "doc-1",
			},
		},
	}
	if err := registry.RegisterModule(NewLegacyToolAdapterWithSpec(tool, ToolSpec{
		Capability:          CapabilityDiagnosis,
		EvidenceSources:     []string{EvidenceSourceSystemRecords},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "system",
	}).Module()); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	executor := ragruntime.NewExecutor(registry)
	result, err := executor.Execute(context.Background(), Call{
		Name: "document_query",
		Arguments: map[string]any{
			"documentId": "doc-1",
		},
	})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	if result.Name != "document_query" {
		t.Fatalf("unexpected result name: %q", result.Name)
	}
	if result.Status != CallStatusSuccess {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
	if tool.lastCall.Arguments["documentId"] != "doc-1" {
		t.Fatalf("unexpected invoke arguments: %+v", tool.lastCall.Arguments)
	}
	if result.Meta.Capability != CapabilityDiagnosis {
		t.Fatalf("expected result meta capability, got %+v", result.Meta)
	}
}

func TestExecutorExecuteFailure(t *testing.T) {
	registry := NewRegistry()
	tool := &toolStub{
		definition: Definition{Name: "trace_node_query", Description: "query trace node"},
		result: Result{
			Summary: "trace lookup failed",
		},
		err: errors.New("repo unavailable"),
	}
	if err := registry.RegisterModule(NewLegacyToolAdapterWithSpec(tool, ToolSpec{
		Capability:          CapabilityDiagnosis,
		EvidenceSources:     []string{EvidenceSourceRAGTrace},
		ExecutionMode:       ExecutionModeReadOnly,
		RiskLevel:           RiskLevelLow,
		ApprovalRequirement: ApprovalRequirementNone,
		ReadOnly:            true,
		Family:              "trace",
	}).Module()); err != nil {
		t.Fatalf("register tool: %v", err)
	}

	executor := ragruntime.NewExecutor(registry)
	result, err := executor.Execute(context.Background(), Call{Name: "trace_node_query"})
	if err == nil {
		t.Fatal("expected execution error")
	}
	if result.Status != CallStatusFailed {
		t.Fatalf("unexpected failed status: %q", result.Status)
	}
	if result.ErrorMessage != "repo unavailable" {
		t.Fatalf("unexpected error message: %q", result.ErrorMessage)
	}
}

func TestExecutorExecuteUnknownTool(t *testing.T) {
	executor := ragruntime.NewExecutor(NewRegistry())
	result, err := executor.Execute(context.Background(), Call{Name: "missing_tool"})
	if err == nil {
		t.Fatal("expected unknown tool error")
	}
	if result.Status != CallStatusFailed {
		t.Fatalf("unexpected result status: %q", result.Status)
	}
}

func TestRenderContextAndToCallSummaries(t *testing.T) {
	results := []Result{
		{
			Name:    "document_query",
			Status:  CallStatusSuccess,
			Summary: "matched doc-1",
		},
		{
			Name:         "trace_node_query",
			Status:       CallStatusFailed,
			ErrorMessage: "trace not found",
		},
	}

	contextText := RenderContext(results)
	if contextText == "" {
		t.Fatal("expected rendered context")
	}
	if summaries := ToCallSummaries(results); len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
}

func TestRenderContextIncludesWebSearchAndFetchDetails(t *testing.T) {
	results := []Result{
		{
			Name:    "web_search",
			Status:  CallStatusSuccess,
			Summary: "found 1 web result",
			Data: map[string]any{
				"results": []map[string]any{
					{
						"title":   "Vector Store Troubleshooting",
						"url":     "https://example.com/vector-store",
						"snippet": "How to debug connection refused errors.",
					},
				},
			},
		},
		{
			Name:    "web_fetch",
			Status:  CallStatusSuccess,
			Summary: "fetched 1 urls: 1 ok, 0 failed",
			Data: map[string]any{
				"combinedText": "[https://example.com/vector-store]\nCheck service health and network reachability first.",
			},
		},
	}

	contextText := RenderContext(results)
	if !strings.Contains(contextText, "Vector Store Troubleshooting") {
		t.Fatalf("expected search result title in rendered context: %q", contextText)
	}
	if !strings.Contains(contextText, "Check service health and network reachability first.") {
		t.Fatalf("expected fetched page text in rendered context: %q", contextText)
	}
}

func TestBuildAnswerGuidanceFromDiagnosisResult(t *testing.T) {
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:   "document_ingestion_diagnose",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"conclusion":  "document ingestion failed at node indexer",
				"confidence":  "high",
				"facts":       []string{"文档当前状态为失败。", "失败节点是 indexer。"},
				"rawEvidence": []string{"document.status=failed", "failedNode=indexer"},
				"inferences":  []string{"document ingestion failed at node indexer"},
				"nextActions": []string{"check vector store connectivity"},
			},
		},
	})
	if guidance == "" {
		t.Fatal("expected non-empty guidance")
	}
	if !strings.Contains(guidance, "结论 / 证据 / 建议") {
		t.Fatalf("unexpected guidance: %q", guidance)
	}
	if !strings.Contains(guidance, "document ingestion failed at node indexer") {
		t.Fatalf("missing diagnosis conclusion: %q", guidance)
	}
	if !strings.Contains(guidance, "推断") {
		t.Fatalf("expected inference boundary in guidance: %q", guidance)
	}
}

func TestBuildAnswerGuidanceResolvesStatusConflictDiagnoseFailedTaskRunning(t *testing.T) {
	// doc_run_01 scenario: document_ingestion_diagnose says failed,
	// but ingestion_task_query and ingestion_task_node_query both show running.
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:   "document_ingestion_diagnose",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"conclusion":   "document ingestion failed at node indexer",
				"confidence":   "high",
				"facts":        []string{"文档当前状态为失败。", "最近一次关联任务为 task_run_01。"},
				"latestTaskId": "task_run_01",
			},
		},
		{
			Name:   "ingestion_task_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId": "task_run_01",
				"status": "running",
			},
		},
		{
			Name:   "ingestion_task_node_query",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"taskId":       "task_run_01",
				"nodeId":       "indexer",
				"nodeOrder":    4,
				"status":       "running",
				"durationMs":   15200,
				"errorMessage": "",
			},
		},
	})
	if !strings.Contains(guidance, "仍在处理中") {
		t.Fatalf("expected conclusion to override to running state, got %q", guidance)
	}
	if !strings.Contains(guidance, "当前置信度：high") {
		t.Fatalf("expected confidence to be high after conflict resolution, got %q", guidance)
	}
	if !strings.Contains(guidance, "异步更新") {
		t.Fatalf("expected conflict explanation about async state lag, got %q", guidance)
	}
	if !strings.Contains(guidance, "当前建议结论：文档仍在处理中") {
		t.Fatalf("expected conclusion to say document is still processing, got %q", guidance)
	}
	if !strings.Contains(guidance, "状态不一致") {
		t.Fatalf("expected risk hint about status inconsistency, got %q", guidance)
	}
}

func TestViewWebSearchResultParsesGenericSlices(t *testing.T) {
	view, ok := ViewWebSearchResult(Result{
		Name: "web_search",
		Data: map[string]any{
			"query":        "vector store connection refused",
			"provider":     "tavily",
			"allowedCount": 1,
			"neutralCount": 1,
			"results": []any{
				map[string]any{
					"title":         "Vector Store Troubleshooting",
					"url":           "https://example.com/a",
					"snippet":       "Check the service endpoint first.",
					"domain":        "example.com",
					"provider":      "tavily",
					"providerScore": 0.92,
					"sourceType":    "official_docs",
					"policy":        "allow",
					"riskFlags":     []any{},
					"reasons":       []any{"domain example.com matched allow list"},
				},
				map[string]any{
					"title":      "Network Debugging",
					"url":        "https://example.com/b",
					"snippet":    "Verify DNS and firewall rules.",
					"domain":     "example.com",
					"sourceType": "forum",
					"policy":     "neutral",
					"riskFlags":  []any{"user_generated"},
				},
			},
		},
	})
	if !ok {
		t.Fatal("expected web search view to parse")
	}
	if view.Query != "vector store connection refused" {
		t.Fatalf("unexpected query: %q", view.Query)
	}
	if view.Provider != "tavily" {
		t.Fatalf("unexpected provider: %q", view.Provider)
	}
	if len(view.Results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(view.Results))
	}
	if view.AllowedCount != 1 || view.NeutralCount != 1 {
		t.Fatalf("unexpected policy counters: %+v", view)
	}
	if view.Results[0].Policy != "allow" || view.Results[0].SourceType != "official_docs" {
		t.Fatalf("expected first result policy metadata, got %+v", view.Results[0])
	}
	if len(view.Results[1].RiskFlags) != 1 || view.Results[1].RiskFlags[0] != "user_generated" {
		t.Fatalf("expected second result risk flags, got %+v", view.Results[1].RiskFlags)
	}
	urls := view.URLs(1)
	if len(urls) != 1 || urls[0] != "https://example.com/a" {
		t.Fatalf("unexpected urls: %#v", urls)
	}
	fetchable := view.FetchableURLs(2)
	if len(fetchable) != 2 {
		t.Fatalf("expected both allow and neutral urls to be fetchable, got %#v", fetchable)
	}
}

func TestBuildAnswerGuidanceFromWebSearchIncludesLocalEvidenceAndSources(t *testing.T) {
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:    "document_list",
			Status:  CallStatusSuccess,
			Summary: "matched 2 local documents about vector store operations",
		},
		{
			Name:   "web_search",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"results": []any{
					map[string]any{
						"title":      "Official Troubleshooting Guide",
						"url":        "https://example.com/official",
						"snippet":    "Connection refused usually means the service is unavailable.",
						"sourceType": "official_docs",
						"policy":     "allow",
					},
				},
			},
		},
		{
			Name:   "web_fetch",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"pages": []any{
					map[string]any{
						"url":          "https://example.com/official",
						"text":         "Check the vector store health before retrying.",
						"wasTruncated": true,
					},
				},
			},
		},
	})
	if !strings.Contains(guidance, "本地/知识库侧已知证据") {
		t.Fatalf("expected local evidence section, got %q", guidance)
	}
	if !strings.Contains(guidance, "document_list: matched 2 local documents") {
		t.Fatalf("expected local evidence summary, got %q", guidance)
	}
	if !strings.Contains(guidance, "https://example.com/official") {
		t.Fatalf("expected source url in guidance, got %q", guidance)
	}
	if !strings.Contains(guidance, "policy=allow") || !strings.Contains(guidance, "type=official_docs") {
		t.Fatalf("expected source policy metadata in guidance, got %q", guidance)
	}
}

func TestViewExternalEvidenceWorkflowResultParsesQualityAndSourceReview(t *testing.T) {
	view, ok := ViewExternalEvidenceWorkflowResult(Result{
		Name: "external_evidence_workflow",
		Data: map[string]any{
			"question":            "What is Go generics?",
			"searchQuery":         "go generics overview",
			"selectedUrls":        []string{"https://go.dev/doc/tutorial/generics"},
			"selectedDomains":     []string{"go.dev"},
			"selectedSourceTypes": []string{"official_docs"},
			"sourceCoverage":      "allow_only",
			"qualityAssessment": map[string]any{
				"quality":         "strong",
				"confidence":      0.8,
				"reasoning":       "Readable official content was fetched.",
				"sourceDiversity": "low",
				"corroboration":   "single_source",
				"successfulPages": 1,
			},
			"sourceReview": map[string]any{
				"totalResults":  2,
				"allowedCount":  1,
				"selectedCount": 1,
				"coverage":      "allow_only",
				"selectedSources": []any{
					map[string]any{
						"title":      "Generics tutorial",
						"url":        "https://go.dev/doc/tutorial/generics",
						"domain":     "go.dev",
						"policy":     "allow",
						"sourceType": "official_docs",
					},
				},
			},
			"readiness":           "ready",
			"readinessConfidence": 0.82,
			"citedUrls":           []string{"https://go.dev/doc/tutorial/generics"},
		},
	})
	if !ok {
		t.Fatal("expected external evidence workflow view to parse")
	}
	if view.SourceCoverage != "allow_only" {
		t.Fatalf("unexpected source coverage: %q", view.SourceCoverage)
	}
	if view.Quality.Quality != "strong" || view.Quality.Corroboration != "single_source" {
		t.Fatalf("unexpected quality view: %+v", view.Quality)
	}
	if len(view.SourceReview.SelectedSources) != 1 {
		t.Fatalf("expected 1 selected source, got %+v", view.SourceReview.SelectedSources)
	}
	if view.SourceReview.SelectedSources[0].SourceType != "official_docs" {
		t.Fatalf("unexpected selected source type: %+v", view.SourceReview.SelectedSources[0])
	}
}

func TestBuildAnswerGuidanceFromExternalEvidenceWorkflowIncludesQualityAndSources(t *testing.T) {
	guidance := BuildAnswerGuidance([]Result{
		{
			Name:    "document_list",
			Status:  CallStatusSuccess,
			Summary: "matched 1 local document about Go syntax basics",
		},
		{
			Name:   "external_evidence_workflow",
			Status: CallStatusSuccess,
			Data: map[string]any{
				"question":            "What is Go generics?",
				"searchQuery":         "go generics overview",
				"selectedUrls":        []string{"https://go.dev/doc/tutorial/generics"},
				"selectedDomains":     []string{"go.dev"},
				"selectedSourceTypes": []string{"official_docs"},
				"sourceCoverage":      "allow_only",
				"quality":             "strong",
				"qualityConfidence":   0.8,
				"qualityReasoning":    "Readable external evidence was fetched from selected sources with enough quality to ground an answer.",
				"readiness":           "ready",
				"readinessConfidence": 0.82,
				"readinessReasoning":  "The fetched source is sufficient to answer with attribution.",
				"answerStrategy":      "Answer directly and cite the official docs first.",
				"missingInformation":  []string{"Additional corroborating source for ecosystem examples"},
				"citedUrls":           []string{"https://go.dev/doc/tutorial/generics"},
				"sourceReview": map[string]any{
					"coverage": "allow_only",
					"selectedSources": []any{
						map[string]any{
							"title":      "Generics tutorial",
							"url":        "https://go.dev/doc/tutorial/generics",
							"policy":     "allow",
							"sourceType": "official_docs",
						},
					},
				},
				"qualityAssessment": map[string]any{
					"quality":         "strong",
					"confidence":      0.8,
					"reasoning":       "Readable external evidence was fetched from selected sources with enough quality to ground an answer.",
					"sourceDiversity": "low",
					"corroboration":   "single_source",
					"notes":           []string{"No obvious contradiction was detected."},
				},
			},
		},
	})
	if !strings.Contains(guidance, "本地/知识库侧已知证据") {
		t.Fatalf("expected local evidence section, got %q", guidance)
	}
	if !strings.Contains(guidance, "外部来源质量要求") {
		t.Fatalf("expected external quality section, got %q", guidance)
	}
	if !strings.Contains(guidance, "allow_only") || !strings.Contains(guidance, "strong") {
		t.Fatalf("expected source coverage and quality hints, got %q", guidance)
	}
	if !strings.Contains(guidance, "https://go.dev/doc/tutorial/generics") {
		t.Fatalf("expected cited URL in guidance, got %q", guidance)
	}
	if !strings.Contains(guidance, "policy=allow") || !strings.Contains(guidance, "type=official_docs") {
		t.Fatalf("expected selected source metadata, got %q", guidance)
	}
	if !strings.Contains(guidance, "Additional corroborating source for ecosystem examples") {
		t.Fatalf("expected missing information hint, got %q", guidance)
	}
}

func TestBuildWorkflowTraceMetaDetectsSearchCapabilityAndEvidenceSources(t *testing.T) {
	retrieveResult := ragretrieve.Result{
		Chunks: []convention.RetrievedChunk{{ID: "c1", Score: 0.91}},
	}
	control := deriveWorkflowControl(WorkflowInput{
		Control: WorkflowControl{
			ExecutionMode:       ExecutionModeReadOnly,
			RiskLevel:           RiskLevelLow,
			ApprovalRequirement: ApprovalRequirementNone,
		},
		RetrieveResult: retrieveResult,
	}, []Result{
		{Name: "document_list", Summary: "matched local docs"},
		{Name: "web_search"},
		{Name: "web_fetch"},
	})

	if control.Capability != CapabilitySearch {
		t.Fatalf("expected search capability, got %q", control.Capability)
	}

	traceMeta := buildWorkflowTraceMeta(control, retrieveResult, []Result{
		{Name: "document_list"},
		{Name: "web_search"},
		{Name: "web_fetch"},
	})
	if traceMeta.ExecutionMode != ExecutionModeReadOnly {
		t.Fatalf("unexpected execution mode: %q", traceMeta.ExecutionMode)
	}
	if !strings.Contains(strings.Join(traceMeta.EvidenceSources, ","), EvidenceSourceKnowledgeBase) {
		t.Fatalf("expected knowledge base evidence source, got %+v", traceMeta.EvidenceSources)
	}
	if !strings.Contains(strings.Join(traceMeta.EvidenceSources, ","), EvidenceSourceExternalWeb) {
		t.Fatalf("expected external web evidence source, got %+v", traceMeta.EvidenceSources)
	}
}

func TestBuildWorkflowTraceMetaPrefersResultMeta(t *testing.T) {
	control := deriveWorkflowControl(WorkflowInput{
		Control: WorkflowControl{
			ExecutionMode:       ExecutionModeReadOnly,
			RiskLevel:           RiskLevelLow,
			ApprovalRequirement: ApprovalRequirementNone,
		},
	}, []Result{
		{
			Name: "web_search",
			Meta: ResultMeta{
				Capability:          CapabilitySearch,
				EvidenceSources:     []string{EvidenceSourceExternalWeb},
				ExecutionMode:       ExecutionModeReadOnly,
				RiskLevel:           RiskLevelLow,
				ApprovalRequirement: ApprovalRequirementNone,
			},
		},
	})
	if control.Capability != CapabilitySearch {
		t.Fatalf("expected capability from result meta, got %q", control.Capability)
	}

	traceMeta := buildWorkflowTraceMeta(control, ragretrieve.Result{}, []Result{
		{
			Name: "web_search",
			Meta: ResultMeta{
				EvidenceSources: []string{EvidenceSourceExternalWeb},
			},
		},
	})
	if len(traceMeta.EvidenceSources) != 1 || traceMeta.EvidenceSources[0] != EvidenceSourceExternalWeb {
		t.Fatalf("expected evidence source from result meta, got %+v", traceMeta.EvidenceSources)
	}
}

func TestDeriveWorkflowControlFallsBackToLegacyToolSpec(t *testing.T) {
	control := deriveWorkflowControl(WorkflowInput{}, []Result{
		{Name: "trace_node_query"},
	})
	if control.Capability != CapabilityDiagnosis {
		t.Fatalf("expected diagnosis capability from legacy spec, got %q", control.Capability)
	}
	if control.ExecutionMode != ExecutionModeReadOnly {
		t.Fatalf("expected read_only execution mode from legacy spec, got %q", control.ExecutionMode)
	}
	if control.RiskLevel != RiskLevelLow {
		t.Fatalf("expected low risk from legacy spec, got %q", control.RiskLevel)
	}
	if control.ApprovalRequirement != ApprovalRequirementNone {
		t.Fatalf("expected none approval requirement from legacy spec, got %q", control.ApprovalRequirement)
	}
}

func TestFirstMatchedIDRequiresStructuredIdentifiers(t *testing.T) {
	if got := ragcore.FirstMatchedID(ragcore.DocumentIDPattern, "document doc_run_01 对应的最新 ingestion task 现在是什么状态？"); got != "doc_run_01" {
		t.Fatalf("expected doc_run_01, got %q", got)
	}
	if got := ragcore.FirstMatchedID(ragcore.DocumentIDPattern, "document 当前状态是什么"); got != "" {
		t.Fatalf("expected plain keyword document to not be treated as id, got %q", got)
	}
	if got := ragcore.FirstMatchedID(ragcore.TaskIDPattern, "task task_run_01 当前还在运行吗"); got != "task_run_01" {
		t.Fatalf("expected task_run_01, got %q", got)
	}
	if got := ragcore.FirstMatchedID(ragcore.TaskIDPattern, "task 当前状态是什么"); got != "" {
		t.Fatalf("expected plain keyword task to not be treated as id, got %q", got)
	}
	if got := ragcore.FirstMatchedID(ragcore.TraceIDPattern, "trace trace_bad_01 为什么检索效果差"); got != "trace_bad_01" {
		t.Fatalf("expected trace_bad_01, got %q", got)
	}
	if got := ragcore.FirstMatchedID(ragcore.TraceIDPattern, "trace 当前情况如何"); got != "" {
		t.Fatalf("expected plain keyword trace to not be treated as id, got %q", got)
	}
}
