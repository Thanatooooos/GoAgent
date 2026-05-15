package web

import (
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

func WebSearchBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewWebSearchResult(result)
			if !ok {
				return nil, fmt.Errorf("web_search result view unavailable")
			}
			return view, nil
		},
		Next: func(result ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			hintCall, done, reason := nextActionWebSearch(result)
			return nextDecisionFromHint(hintCall, done, reason)
		},
		Observe: func(result ragcore.Result, input ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeWebSearch(result, input), true
		},
	}
}

func WebFetchBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewWebFetchResult(result)
			if !ok {
				return nil, fmt.Errorf("web_fetch result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "web_fetch_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return observeWebFetch(result), true
		},
	}
}

func ExternalEvidenceWorkflowBehavior() ragcore.ToolBehavior {
	return ragcore.ToolBehavior{
		Decode: func(result ragcore.Result) (any, error) {
			view, ok := ViewExternalEvidenceWorkflowResult(result)
			if !ok {
				return nil, fmt.Errorf("external_evidence_workflow result view unavailable")
			}
			return view, nil
		},
		Next: func(_ ragcore.Result, _ ragcore.WorkflowInput) ragcore.NextDecision {
			return ragcore.NextDecision{Done: true, Reason: "external_evidence_workflow_terminal", Terminal: true}
		},
		Observe: func(result ragcore.Result, _ ragcore.ObserveInput) (ragcore.ObserveResult, bool) {
			return ragcore.NewObserveResult(true, "The external evidence workflow already collected and assessed web evidence, so the agent can answer with the current sources.", ragcore.ObserveState(
				"complete",
				ragcore.FirstNonEmpty(result.GetString("readinessReasoning"), result.Summary, result.ErrorMessage),
				clampConfidence(ragcore.ReadDataFloat(result.Data, "readinessConfidence")),
				nil,
				[]string{result.Name},
			)), true
		},
	}
}

func nextActionWebSearch(result ragcore.Result) (*ragcore.HintCall, bool, string) {
	view, ok := ViewWebSearchResult(result)
	results := result.GetStringSlice("urls")
	if len(results) == 0 && ok {
		results = view.FetchableURLs(3)
	}
	if len(results) == 0 && ok {
		results = view.URLs(3)
	}
	if len(results) == 0 {
		return nil, true, "web_search_no_results"
	}
	if len(results) > 3 {
		results = results[:3]
	}
	return &ragcore.HintCall{Name: "web_fetch", Arguments: map[string]any{"urls": results}}, false, "web_search_has_results"
}

func nextDecisionFromHint(hintCall *ragcore.HintCall, done bool, reason string) ragcore.NextDecision {
	decision := ragcore.NextDecision{Done: done, Reason: reason, Terminal: done}
	if hintCall != nil {
		decision.HintCalls = []ragcore.HintCall{*hintCall}
		decision.Done = false
		decision.Terminal = false
	}
	return decision
}

func observeWebSearch(result ragcore.Result, input ragcore.ObserveInput) ragcore.ObserveResult {
	hintCall, done, reason := nextActionWebSearch(result)
	view, _ := ViewWebSearchResult(result)
	if done || hintCall == nil {
		return ragcore.NewObserveResult(true, "Web search completed. The results are sufficient to answer with source attribution.", ragcore.ObserveState("complete", result.Summary, 0.75, nil, []string{result.Name}))
	}
	switch reason {
	case "web_search_has_results":
		resultCount := result.GetInt("resultCount")
		if view.ResultCount > resultCount {
			resultCount = view.ResultCount
		}
		return ragcore.NewObserveResult(false, "Web search found results. Fetch content from the top results for more detail before answering.", ragcore.ObserveState("fetching", fmt.Sprintf("found %d web results", resultCount), 0.45, []ragcore.HintCall{*hintCall}, []string{result.Name}, "What does each result page actually say?"))
	default:
		return ragcore.NewObserveResult(true, "Web search completed. The results are sufficient to answer with source attribution.", ragcore.ObserveState("complete", result.Summary, 0.75, nil, []string{result.Name}))
	}
}

func observeWebFetch(result ragcore.Result) ragcore.ObserveResult {
	view, _ := ViewWebFetchResult(result)
	text := strings.TrimSpace(view.ReadableText())
	wasTruncated := view.AnyPageTruncated()
	if text == "" {
		return ragcore.NewObserveResult(true, "Web page fetched but no readable text was extracted. Answer with the available search snippets.", ragcore.ObserveState("complete", result.Summary, 0.55, nil, []string{result.Name}))
	}
	confidence := 0.8
	if wasTruncated {
		confidence = 0.7
	}
	return ragcore.NewObserveResult(true, "Web page content has been fetched. The agent can now synthesize an answer using both knowledge base context and web sources with proper attribution.", ragcore.ObserveState("complete", result.Summary, confidence, nil, []string{result.Name}))
}

func clampConfidence(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
