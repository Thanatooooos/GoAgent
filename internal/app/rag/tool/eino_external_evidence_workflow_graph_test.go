package tool_test

import (
	"context"
	"strings"
	"testing"

	. "local/rag-project/internal/app/rag/tool"
	raggraph "local/rag-project/internal/app/rag/tool/invokers/graph"
)

func TestExternalEvidenceWorkflowGraphBuildsSourceReviewAndQuality(t *testing.T) {
	registry := NewRegistry()
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:       "web_search",
			ReadOnly:   true,
			Parameters: []ParameterDefinition{{Name: "query", Type: ParamTypeString, Required: true}},
		},
		result: Result{
			Name:    "web_search",
			Status:  CallStatusSuccess,
			Summary: "found 4 web results",
			Data: map[string]any{
				"query":        "go generics overview",
				"provider":     "tavily",
				"resultCount":  4,
				"allowedCount": 2,
				"neutralCount": 1,
				"deniedCount":  1,
				"results": []any{
					map[string]any{
						"title":      "Generics tutorial",
						"url":        "https://go.dev/doc/tutorial/generics",
						"domain":     "go.dev",
						"policy":     "allow",
						"sourceType": "official_docs",
					},
					map[string]any{
						"title":      "Proposal",
						"url":        "https://github.com/golang/go/issues/43651",
						"domain":     "github.com",
						"policy":     "allow",
						"sourceType": "repository",
					},
					map[string]any{
						"title":      "Forum discussion",
						"url":        "https://forum.example.com/go-generics",
						"domain":     "forum.example.com",
						"policy":     "neutral",
						"sourceType": "forum",
					},
					map[string]any{
						"title":      "Spam article",
						"url":        "https://spam.example.com/go-generics",
						"domain":     "spam.example.com",
						"policy":     "deny",
						"sourceType": "blog",
					},
				},
			},
		},
	})
	registry.MustRegister(staticTool{
		definition: Definition{
			Name:       "web_fetch",
			ReadOnly:   true,
			Parameters: []ParameterDefinition{{Name: "urls", Type: ParamTypeArray, Required: true}},
		},
		result: Result{
			Name:    "web_fetch",
			Status:  CallStatusSuccess,
			Summary: "fetched 3 urls: 2 ok, 1 failed",
			Data: map[string]any{
				"urls":         []string{"https://go.dev/doc/tutorial/generics", "https://github.com/golang/go/issues/43651", "https://forum.example.com/go-generics"},
				"successCount": 2,
				"failCount":    1,
				"combinedText": "[https://go.dev/doc/tutorial/generics]\nGenerics let you write reusable functions and types.\n\n---\n\n[https://github.com/golang/go/issues/43651]\nThe proposal introduced type parameters in Go 1.18.",
				"pages": []any{
					map[string]any{
						"url":  "https://go.dev/doc/tutorial/generics",
						"text": "Generics let you write reusable functions and types.",
					},
					map[string]any{
						"url":  "https://github.com/golang/go/issues/43651",
						"text": "The proposal introduced type parameters in Go 1.18.",
					},
					map[string]any{
						"url":   "https://forum.example.com/go-generics",
						"error": "http status 500",
					},
				},
			},
		},
	})

	tool, err := raggraph.NewExternalEvidenceWorkflowTool(NewExecutor(registry), nil)
	if err != nil {
		t.Fatalf("create external evidence workflow tool: %v", err)
	}

	result, err := tool.Invoke(context.Background(), Call{
		Name:      "external_evidence_workflow",
		Arguments: map[string]any{"question": "What is Go generics?"},
	})
	if err != nil {
		t.Fatalf("invoke workflow: %v", err)
	}
	if result.Status != CallStatusSuccess {
		t.Fatalf("expected success, got %q: %s", result.Status, result.ErrorMessage)
	}
	if !strings.Contains(result.Summary, "quality=") || !strings.Contains(result.Summary, "readiness=") {
		t.Fatalf("expected quality and readiness in summary, got %q", result.Summary)
	}

	view, ok := ViewExternalEvidenceWorkflowResult(result)
	if !ok {
		t.Fatal("expected external evidence workflow view")
	}
	if len(view.SelectedURLs) != 3 {
		t.Fatalf("expected 3 selected URLs, got %#v", view.SelectedURLs)
	}
	if view.SourceReview.Coverage != "mixed" {
		t.Fatalf("expected mixed coverage, got %+v", view.SourceReview)
	}
	if view.Quality.Quality == "" || view.Quality.Corroboration != "corroborated" {
		t.Fatalf("unexpected quality view: %+v", view.Quality)
	}
	if len(view.CitedURLs) == 0 {
		t.Fatalf("expected cited URLs, got %+v", view)
	}
	for _, blocked := range view.CitedURLs {
		if blocked == "https://spam.example.com/go-generics" {
			t.Fatalf("deny-listed source should not be cited: %+v", view.CitedURLs)
		}
	}
}
