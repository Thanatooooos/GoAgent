package external_evidence

import (
	"fmt"
	"strings"

	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentsearch "local/rag-project/internal/app/agent/search"
	agentstate "local/rag-project/internal/app/agent/state"
)

func reviewSearchResults(input ReviewInput) ReviewResult {
	maxCandidates := input.MaxCandidates
	if maxCandidates <= 0 {
		maxCandidates = defaultMaxReviewedSources
	}

	seenURLs := make(map[string]struct{}, len(input.Results))
	allowed := make([]agentsearch.SearchResultItem, 0, len(input.Results))
	neutral := make([]agentsearch.SearchResultItem, 0, len(input.Results))
	review := ReviewResult{
		Readiness:           readinessInsufficient,
		ReadinessConfidence: 0.18,
		ReadinessReasoning:  "readable evidence has not been assessed yet",
	}

	for _, result := range input.Results {
		url := strings.TrimSpace(result.URL)
		if url == "" {
			review.Rejected = append(review.Rejected, RejectedSource{
				Title:  strings.TrimSpace(result.Title),
				Reason: "missing_url",
			})
			continue
		}
		if _, ok := seenURLs[url]; ok {
			review.Rejected = append(review.Rejected, RejectedSource{
				URL:    url,
				Title:  strings.TrimSpace(result.Title),
				Reason: "duplicate_url",
			})
			continue
		}
		seenURLs[url] = struct{}{}

		switch strings.TrimSpace(result.Policy) {
		case "deny":
			review.Rejected = append(review.Rejected, RejectedSource{
				URL:    url,
				Title:  strings.TrimSpace(result.Title),
				Reason: "policy_denied",
			})
		case "allow":
			allowed = append(allowed, result)
		default:
			neutral = append(neutral, result)
		}
	}

	candidates := append(allowed, neutral...)
	for _, result := range candidates {
		if len(review.Selected) >= maxCandidates {
			review.Rejected = append(review.Rejected, RejectedSource{
				URL:    strings.TrimSpace(result.URL),
				Title:  strings.TrimSpace(result.Title),
				Reason: "candidate_limit_reached",
			})
			continue
		}
		review.Selected = append(review.Selected, SelectedSource{
			URL:      strings.TrimSpace(result.URL),
			Title:    strings.TrimSpace(result.Title),
			Policy:   strings.TrimSpace(result.Policy),
			Reason:   selectedSourceReason(result),
			Priority: len(review.Selected) + 1,
		})
	}

	if len(review.Selected) == 0 {
		review.ReadinessReasoning = "source review found no fetchable candidates"
		return review
	}

	review.ReadinessConfidence = 0.28
	review.ReadinessReasoning = fmt.Sprintf("source review selected %d candidate source(s) for fetch", len(review.Selected))
	return review
}

func finalizeSourceReview(review ReviewResult, output agentfetch.Output) ReviewResult {
	citedURLs := make([]string, 0, len(output.Pages))
	seenCitations := make(map[string]struct{}, len(output.Pages))
	for _, page := range output.Pages {
		if strings.TrimSpace(page.Text) == "" || strings.TrimSpace(page.ErrorMessage) != "" {
			continue
		}
		url := strings.TrimSpace(page.URL)
		if url == "" {
			continue
		}
		if _, ok := seenCitations[url]; ok {
			continue
		}
		seenCitations[url] = struct{}{}
		citedURLs = append(citedURLs, url)
	}

	if len(citedURLs) == 0 {
		citedURLs = nil
	}
	review.CitedURLs = citedURLs
	switch len(citedURLs) {
	case 0:
		review.Readiness = readinessInsufficient
		review.ReadinessConfidence = 0.14
		review.ReadinessReasoning = "no selected sources produced readable page content"
	case 1:
		review.Readiness = readinessPartial
		review.ReadinessConfidence = 0.61
		review.ReadinessReasoning = "one selected source produced readable page content"
	default:
		review.Readiness = readinessReady
		review.ReadinessConfidence = 0.86
		review.ReadinessReasoning = "multiple selected sources produced readable page content"
	}
	return review
}

func selectedReviewURLs(review ReviewResult) []string {
	if len(review.Selected) == 0 {
		return nil
	}
	urls := make([]string, 0, len(review.Selected))
	for _, source := range review.Selected {
		if url := strings.TrimSpace(source.URL); url != "" {
			urls = append(urls, url)
		}
	}
	if len(urls) == 0 {
		return nil
	}
	return urls
}

func reviewStateDelta(review ReviewResult) agentstate.StateDelta {
	notes := []string{
		fmt.Sprintf("source review selected=%d rejected=%d", len(review.Selected), len(review.Rejected)),
		fmt.Sprintf("source review readiness=%s confidence=%.2f: %s", review.Readiness, review.ReadinessConfidence, strings.TrimSpace(review.ReadinessReasoning)),
	}
	return agentstate.StateDelta{
		Context: &agentstate.ContextDelta{
			Notes: notes,
		},
	}
}

func reviewObservationSummary(searchSummary string, fetchSummary string, review ReviewResult) string {
	base := strings.TrimSpace(fetchSummary)
	if base == "" {
		base = strings.TrimSpace(searchSummary)
	}
	suffix := fmt.Sprintf("selected=%d rejected=%d readiness=%s", len(review.Selected), len(review.Rejected), strings.TrimSpace(review.Readiness))
	if strings.TrimSpace(base) == "" {
		return suffix
	}
	return base + " | " + suffix
}

func selectedSourceReason(result agentsearch.SearchResultItem) string {
	if strings.TrimSpace(result.Policy) == "allow" {
		return "policy_allow"
	}
	return "policy_neutral"
}
