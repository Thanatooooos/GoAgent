package graph

import (
	"context"
	"fmt"
	"strings"

	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	ragruntime "local/rag-project/internal/app/rag/tool/runtime"

	"github.com/cloudwego/eino/compose"
)

// diagnoseSearchState flows through the diagnose → web_search graph.
type diagnoseSearchState struct {
	DocumentID   string
	Diagnosis    ragtool.Result
	SearchQuery  string
	SearchResult ragtool.Result
	Results      []ragtool.Result
	LastError    string
}

// DiagnoseSearchGraphTool wraps an Eino graph that chains:
//
//	document_root_cause_diagnosis → keyword extract → web_search (conditional)
//
// The web_search hop is skipped when the diagnosis does not expose a technical error.
type DiagnoseSearchGraphTool struct {
	runner compose.Runnable[*diagnoseSearchState, *diagnoseSearchState]
}

func NewDiagnoseSearchGraphTool(executor *ragruntime.Executor) (*DiagnoseSearchGraphTool, error) {
	if executor == nil {
		return nil, fmt.Errorf("executor with registry is required")
	}

	graph := compose.NewGraph[*diagnoseSearchState, *diagnoseSearchState]()

	// Node 1: run the existing diagnosis graph tool.
	graph.AddLambdaNode("diagnose", compose.InvokableLambda(
		func(ctx context.Context, state *diagnoseSearchState) (*diagnoseSearchState, error) {
			if state.DocumentID == "" {
				state.LastError = "documentId is required"
				return state, nil
			}
			result, err := executor.Execute(ctx, ragtool.Call{
				Name:      "document_root_cause_diagnosis",
				Arguments: map[string]any{"documentId": state.DocumentID},
			})
			if err != nil {
				state.LastError = err.Error()
				return state, nil
			}
			state.Diagnosis = result
			state.Results = append(state.Results, result)
			return state, nil
		},
	))

	// Node 2: extract keywords and optionally search the web.
	graph.AddLambdaNode("search", compose.InvokableLambda(
		func(ctx context.Context, state *diagnoseSearchState) (*diagnoseSearchState, error) {
			query := extractSearchKeyword(state.Diagnosis)
			if query == "" {
				return state, nil
			}
			state.SearchQuery = query
			result, err := executor.Execute(ctx, ragtool.Call{
				Name:      "web_search",
				Arguments: map[string]any{"query": query},
			})
			if err != nil {
				state.LastError = err.Error()
				return state, nil
			}
			state.SearchResult = result
			state.Results = append(state.Results, result)
			return state, nil
		},
	))

	_ = graph.AddEdge(compose.START, "diagnose")
	_ = graph.AddEdge("diagnose", "search")
	_ = graph.AddEdge("search", compose.END)

	runner, err := graph.Compile(context.Background(), compose.WithGraphName("diagnose_search_chain"))
	if err != nil {
		return nil, fmt.Errorf("compile diagnose-search graph: %w", err)
	}

	return &DiagnoseSearchGraphTool{runner: runner}, nil
}

func (t *DiagnoseSearchGraphTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "document_diagnose_with_search",
		Description: "Diagnose a document's ingestion failure and optionally search the web for solutions to the specific error. Chains: document_root_cause_diagnosis → web_search(error keywords). Web search is skipped when no technical error is found.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "documentId",
				Type:        ragtool.ParamTypeString,
				Description: "Knowledge document id to diagnose.",
				Required:    true,
			},
		},
	}
}

func (t *DiagnoseSearchGraphTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.runner == nil {
		return ragtool.Result{
			Name:         "document_diagnose_with_search",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: "diagnose-search graph runner is not initialized",
		}, nil
	}

	documentID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "documentId"))
	if documentID == "" {
		return ragtool.Result{
			Name:         "document_diagnose_with_search",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: "documentId is required",
		}, nil
	}

	state := &diagnoseSearchState{DocumentID: documentID}
	final, err := t.runner.Invoke(ctx, state)
	if err != nil {
		return ragtool.Result{
			Name:         "document_diagnose_with_search",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: err.Error(),
		}, nil
	}

	if final.LastError != "" && len(final.Results) == 0 {
		return ragtool.Result{
			Name:         "document_diagnose_with_search",
			Status:       ragtool.CallStatusFailed,
			ErrorMessage: final.LastError,
		}, nil
	}

	data := map[string]any{
		"documentId":     final.DocumentID,
		"diagnosisDepth": diagnosisDepthLabel(len(final.Results)),
	}
	if final.Diagnosis.Name != "" {
		data["conclusion"] = final.Diagnosis.GetString("conclusion")
	}
	if final.SearchQuery != "" {
		data["searchQuery"] = final.SearchQuery
		if final.SearchResult.Name != "" {
			data["searchResultCount"] = len(final.SearchResult.Data)
		}
	}

	summary := buildDiagnoseSearchSummary(final)
	return ragtool.Result{
		Name:    "document_diagnose_with_search",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data:    data,
	}, nil
}

// extractSearchKeyword pulls a technical error phrase from a diagnosis result.
// Returns "" when no searchable technical error is found.
func extractSearchKeyword(result ragtool.Result) string {
	if result.Name == "" {
		return ""
	}

	candidates := []string{
		result.GetString("errorMessage"),
		result.GetString("conclusion"),
	}

	for _, raw := range candidates {
		if raw == "" {
			continue
		}
		keyword := pickTechnicalPhrase(raw)
		if keyword != "" {
			return keyword + " troubleshooting"
		}
	}

	// Fallback: check if the data contains any node-level error from deeper results.
	// The diagnosis graph nests results; the conclusion field carries the deepest error.
	conclusion := result.GetString("conclusion")
	if conclusion != "" && !strings.Contains(strings.ToLower(conclusion), "running") {
		keyword := pickTechnicalPhrase(conclusion)
		if keyword != "" {
			return keyword + " solution"
		}
	}

	return ""
}

// pickTechnicalPhrase extracts the most specific technical phrase from a diagnostic message.
// It looks for well-known error patterns and falls back to the first sentence-like segment.
func pickTechnicalPhrase(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	lower := strings.ToLower(raw)

	// Known technical error patterns — match the most specific one.
	patterns := []struct {
		match string
		query string
	}{
		{"connection refused", "connection refused"},
		{"connection reset", "connection reset"},
		{"connection timeout", "connection timeout"},
		{"deadline exceeded", "deadline exceeded"},
		{"i/o timeout", "i/o timeout"},
		{"no such host", "no such host"},
		{"permission denied", "permission denied"},
		{"access denied", "access denied"},
		{"unauthorized", "unauthorized"},
		{"not found", "not found error"},
		{"out of memory", "out of memory"},
		{"disk full", "disk full"},
		{"too many connections", "too many connections"},
		{"rate limit", "rate limit exceeded"},
		{"certificate expired", "certificate expired"},
		{"tls handshake", "tls handshake failed"},
		{"vector store unavailable", "vector store unavailable"},
		{"embedding failed", "embedding failed"},
		{"chunk failed", "document chunking failed"},
		{"parse failed", "document parsing failed"},
	}

	for _, p := range patterns {
		if strings.Contains(lower, p.match) {
			return p.query
		}
	}

	// No known pattern — extract the first meaningful phrase (up to 80 chars).
	// Remove leading noise like "conclusion: " or "document failed at node X: ".
	if idx := strings.Index(raw, ": "); idx > 0 && idx < 50 {
		raw = strings.TrimSpace(raw[idx+2:])
	}
	if len(raw) > 80 {
		if idx := strings.LastIndex(raw[:80], " "); idx > 0 {
			raw = raw[:idx]
		} else {
			raw = raw[:80]
		}
	}

	// Only return if it looks like a technical error (not a generic status).
	if looksLikeTechnicalError(raw) {
		return raw
	}
	return ""
}

func looksLikeTechnicalError(s string) bool {
	lower := strings.ToLower(s)
	techTerms := []string{
		"error", "failed", "refused", "timeout", "unavailable",
		"denied", "invalid", "missing", "exceeded", "wrong",
		"cannot", "could not", "unable",
	}
	for _, term := range techTerms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

func buildDiagnoseSearchSummary(state *diagnoseSearchState) string {
	parts := make([]string, 0, 2)
	if state.Diagnosis.Name != "" {
		parts = append(parts, fmt.Sprintf("diagnosis=%s", state.Diagnosis.Status))
	}
	if state.SearchQuery != "" {
		if state.SearchResult.Name != "" {
			parts = append(parts, fmt.Sprintf("web_search=%s(%q)", state.SearchResult.Status, state.SearchQuery))
		} else {
			parts = append(parts, fmt.Sprintf("web_search=attempted(%q)", state.SearchQuery))
		}
	} else {
		parts = append(parts, "web_search=skipped(no technical error found)")
	}
	return fmt.Sprintf("diagnose+search chain: %s", strings.Join(parts, " → "))
}
