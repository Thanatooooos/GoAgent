package graph

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

type DocumentRootCauseDiagnosisView struct {
	DocumentID     string
	LatestTaskID   string
	LatestNodeID   string
	Conclusion     string
	Confidence     string
	DiagnosisDepth string
	ChainLength    int
}

type DocumentDiagnoseWithSearchView struct {
	DocumentID        string
	Conclusion        string
	DiagnosisDepth    string
	SearchQuery       string
	SearchResultCount int
}

func DocumentRootCauseDiagnosisBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewDocumentRootCauseDiagnosisResult(result)
			if !ok {
				return nil, fmt.Errorf("document_root_cause_diagnosis result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "document_root_cause_diagnosis_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeGraphDiagnosisResult(result), true
		},
		RenderContext: renderDocumentRootCauseDiagnosisContext,
		BuildGuidance: func(result ragcore.Result, _ ragcore.GuidanceInput) []ragcore.GuidanceNote {
			return buildDocumentRootCauseDiagnosisGuidanceNotes(result)
		},
	}
}

func DocumentDiagnoseWithSearchBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewDocumentDiagnoseWithSearchResult(result)
			if !ok {
				return nil, fmt.Errorf("document_diagnose_with_search result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "document_diagnose_with_search_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			confidence := 0.72
			if strings.TrimSpace(result.GetString("searchQuery")) != "" && result.GetInt("searchResultCount") > 0 {
				confidence = 0.8
			}
			return ragcore.NewObserveResult(true, "The diagnose-and-search graph already completed the diagnosis chain and optional search step, so the agent can answer with the current evidence.", ragcore.ObserveState(
				"complete",
				ragcore.FirstNonEmpty(result.GetString("conclusion"), result.Summary, result.ErrorMessage),
				confidence,
				nil,
				[]string{result.Name},
			)), true
		},
		RenderContext: renderDocumentDiagnoseWithSearchContext,
		BuildGuidance: func(result ragcore.Result, input ragcore.GuidanceInput) []ragcore.GuidanceNote {
			return buildDocumentDiagnoseWithSearchGuidanceNotes(result, input.AllResults)
		},
	}
}

func ViewDocumentRootCauseDiagnosisResult(result ragcore.Result) (DocumentRootCauseDiagnosisView, bool) {
	if strings.TrimSpace(result.Name) != "document_root_cause_diagnosis" {
		return DocumentRootCauseDiagnosisView{}, false
	}
	return DocumentRootCauseDiagnosisView{
		DocumentID:     result.GetString("documentId"),
		LatestTaskID:   result.GetString("latestTaskId"),
		LatestNodeID:   result.GetString("latestNodeId"),
		Conclusion:     result.GetString("conclusion"),
		Confidence:     result.GetString("confidence"),
		DiagnosisDepth: result.GetString("diagnosisDepth"),
		ChainLength:    result.GetInt("chainLength"),
	}, true
}

func ViewDocumentDiagnoseWithSearchResult(result ragcore.Result) (DocumentDiagnoseWithSearchView, bool) {
	if strings.TrimSpace(result.Name) != "document_diagnose_with_search" {
		return DocumentDiagnoseWithSearchView{}, false
	}
	return DocumentDiagnoseWithSearchView{
		DocumentID:        result.GetString("documentId"),
		Conclusion:        result.GetString("conclusion"),
		DiagnosisDepth:    result.GetString("diagnosisDepth"),
		SearchQuery:       result.GetString("searchQuery"),
		SearchResultCount: result.GetInt("searchResultCount"),
	}, true
}

func observeGraphDiagnosisResult(result ragcore.Result) ragcore.ObserveResult {
	switch strings.ToLower(strings.TrimSpace(result.GetString("diagnosisDepth"))) {
	case "node_level":
		return ragcore.NewObserveResult(true, "The diagnosis graph already reached node-level evidence. The agent can answer with high confidence.", ragcore.ObserveState("complete", ragcore.FirstNonEmpty(result.GetString("conclusion"), result.Summary, result.ErrorMessage), 0.95, nil, []string{result.Name}))
	case "task_level":
		return ragcore.NewObserveResult(true, "The diagnosis graph reached task-level evidence. The agent can answer, but should avoid overstating node-level certainty.", ragcore.ObserveState("complete", ragcore.FirstNonEmpty(result.GetString("conclusion"), result.Summary, result.ErrorMessage), 0.75, nil, []string{result.Name}))
	default:
		return ragcore.NewObserveResult(true, "The graph tool already gathered the main diagnosis evidence needed for the final answer.", ragcore.ObserveState("complete", ragcore.FirstNonEmpty(result.GetString("conclusion"), result.Summary, result.ErrorMessage), 0.6, nil, []string{result.Name}))
	}
}

func renderDocumentRootCauseDiagnosisContext(result ragcore.Result) string {
	view, ok := ViewDocumentRootCauseDiagnosisResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, 5)
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Conclusion: "+conclusion)
	}
	if confidence := strings.TrimSpace(view.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "Diagnosis depth: "+depth)
	}
	if view.ChainLength > 0 {
		lines = append(lines, fmt.Sprintf("Chain length: %d", view.ChainLength))
	}
	if view.LatestTaskID != "" || view.LatestNodeID != "" {
		lines = append(lines, fmt.Sprintf("Latest task/node: %s / %s", ragcore.FirstNonEmpty(view.LatestTaskID, "-"), ragcore.FirstNonEmpty(view.LatestNodeID, "-")))
	}
	return strings.Join(lines, "\n")
}

func renderDocumentDiagnoseWithSearchContext(result ragcore.Result) string {
	view, ok := ViewDocumentDiagnoseWithSearchResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, 4)
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Conclusion: "+conclusion)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "Diagnosis depth: "+depth)
	}
	if query := strings.TrimSpace(view.SearchQuery); query != "" {
		lines = append(lines, "Search query: "+query)
	}
	if view.SearchResultCount > 0 {
		lines = append(lines, fmt.Sprintf("Search results: %d", view.SearchResultCount))
	}
	return strings.Join(lines, "\n")
}

func buildDocumentRootCauseDiagnosisGuidanceNotes(result ragcore.Result) []ragcore.GuidanceNote {
	view, ok := ViewDocumentRootCauseDiagnosisResult(result)
	if !ok {
		return nil
	}

	lines := []string{
		"This is a graph-based diagnosis result. Structure the answer as conclusion, evidence boundary, and next steps.",
	}
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Current conclusion: "+conclusion)
	}
	if confidence := strings.TrimSpace(view.Confidence); confidence != "" {
		lines = append(lines, "Current confidence: "+confidence)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "Diagnosis depth: "+depth)
		switch depth {
		case "node_level":
			lines = append(lines, "This already includes node-level evidence. Answer directly, but do not overstate causes beyond the captured evidence.")
		case "task_level":
			lines = append(lines, "Only task-level evidence is available. Do not invent a confirmed node-level failure.")
		}
	}
	if view.LatestTaskID != "" || view.LatestNodeID != "" {
		lines = append(lines, fmt.Sprintf("Recent execution reference: task=%s, node=%s.", ragcore.FirstNonEmpty(view.LatestTaskID, "-"), ragcore.FirstNonEmpty(view.LatestNodeID, "-")))
	}
	return []ragcore.GuidanceNote{{Text: strings.Join(lines, "\n")}}
}

func buildDocumentDiagnoseWithSearchGuidanceNotes(result ragcore.Result, allResults []ragcore.Result) []ragcore.GuidanceNote {
	view, ok := ViewDocumentDiagnoseWithSearchResult(result)
	if !ok {
		return nil
	}

	lines := []string{
		"This result combines diagnosis with search. Present the internal diagnosis first, then describe external search only as supporting context.",
	}
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Current diagnosis: "+conclusion)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "Diagnosis depth: "+depth)
	}
	if query := strings.TrimSpace(view.SearchQuery); query != "" {
		lines = append(lines, "External search query: "+query)
	}
	if view.SearchResultCount > 0 {
		lines = append(lines, fmt.Sprintf("Search results count: %d.", view.SearchResultCount))
	}
	if hasFetchedWebEvidence(allResults) {
		lines = append(lines, "When citing external remediation ideas, rely only on fetched page content and cite the source URLs explicitly.")
	} else {
		lines = append(lines, "Only search snippets are available. Do not invent a concrete fix; treat external results as troubleshooting direction until page content is fetched.")
	}
	return []ragcore.GuidanceNote{{Text: strings.Join(lines, "\n")}}
}

func hasFetchedWebEvidence(results []ragcore.Result) bool {
	for _, result := range results {
		if view, ok := webmod.ViewWebFetchResult(result); ok && strings.TrimSpace(view.ReadableText()) != "" {
			return true
		}
		if view, ok := webmod.ViewExternalEvidenceWorkflowResult(result); ok && strings.TrimSpace(view.Fetch.ReadableText()) != "" {
			return true
		}
	}
	return false
}
