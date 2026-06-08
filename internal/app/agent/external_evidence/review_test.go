package external_evidence

import (
	"reflect"
	"testing"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
)

func TestReviewSearchResultsPrioritizesAllowedSourcesAndRejectsDuplicates(t *testing.T) {
	review := reviewSearchResults(ReviewInput{
		Query: "golang",
		Results: []agentsearch.SearchResultItem{
			{Title: "Neutral", URL: "https://neutral.example/doc", Policy: "neutral"},
			{Title: "Allowed", URL: "https://allow.example/doc", Policy: "allow"},
			{Title: "Duplicate", URL: "https://allow.example/doc", Policy: "allow"},
			{Title: "Denied", URL: "https://deny.example/doc", Policy: "deny"},
			{Title: "Another Neutral", URL: "https://neutral-2.example/doc", Policy: "neutral"},
		},
		MaxCandidates: 2,
	})

	if got := selectedReviewURLs(review); !reflect.DeepEqual(got, []string{"https://allow.example/doc", "https://neutral.example/doc"}) {
		t.Fatalf("expected allowed source to be prioritized ahead of neutral source, got %v", got)
	}
	if len(review.Rejected) != 3 {
		t.Fatalf("expected duplicate, denied, and capped candidate to be rejected, got %+v", review.Rejected)
	}
	if review.Rejected[0].Reason != "duplicate_url" || review.Rejected[1].Reason != "policy_denied" || review.Rejected[2].Reason != "candidate_limit_reached" {
		t.Fatalf("unexpected rejection reasons: %+v", review.Rejected)
	}
}

func TestReviewSearchResultsReturnsInsufficientWhenNoFetchableSourcesRemain(t *testing.T) {
	review := reviewSearchResults(ReviewInput{
		Query: "no candidates",
		Results: []agentsearch.SearchResultItem{
			{Title: "Denied", URL: "https://deny.example/doc", Policy: "deny"},
			{Title: "Missing URL"},
		},
	})

	if len(review.Selected) != 0 {
		t.Fatalf("expected no selected sources, got %+v", review.Selected)
	}
	if review.Readiness != readinessInsufficient {
		t.Fatalf("expected insufficient readiness, got %+v", review)
	}
}

func TestFinalizeSourceReviewAssessesReadinessFromReadablePages(t *testing.T) {
	base := ReviewResult{
		Selected: []SelectedSource{
			{URL: "https://one.example/doc", Priority: 1},
			{URL: "https://two.example/doc", Priority: 2},
		},
	}

	tests := []struct {
		name       string
		output      agentfetch.Output
		readiness  string
		citedURLs  []string
	}{
		{
			name: "insufficient",
			output: agentfetch.Output{
				Pages: []agentfetch.PageResult{
					{URL: "https://one.example/doc", ErrorMessage: "fetch failed"},
				},
			},
			readiness: readinessInsufficient,
			citedURLs: nil,
		},
		{
			name: "partial",
			output: agentfetch.Output{
				Pages: []agentfetch.PageResult{
					{URL: "https://one.example/doc", Text: "one readable page"},
				},
			},
			readiness: readinessPartial,
			citedURLs: []string{"https://one.example/doc"},
		},
		{
			name: "ready",
			output: agentfetch.Output{
				Pages: []agentfetch.PageResult{
					{URL: "https://one.example/doc", Text: "one readable page"},
					{URL: "https://two.example/doc", Text: "two readable page"},
				},
			},
			readiness: readinessReady,
			citedURLs: []string{"https://one.example/doc", "https://two.example/doc"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			review := finalizeSourceReview(base, tc.output)
			if review.Readiness != tc.readiness {
				t.Fatalf("expected readiness %q, got %+v", tc.readiness, review)
			}
			if !reflect.DeepEqual(review.CitedURLs, tc.citedURLs) {
				t.Fatalf("expected cited urls %v, got %+v", tc.citedURLs, review)
			}
		})
	}
}
