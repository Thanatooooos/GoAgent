package graph

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
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
