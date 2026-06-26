package reactive

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentfetch "local/rag-project/internal/app/agent/fetch"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
)

func newFetchNode(fetchCapability agentcapability.Handle) (agentkernel.Node, error) {
	if fetchCapability == nil {
		return nil, fmt.Errorf("fetch capability is required")
	}
	return agentkernel.NewNodeFunc("fetch", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		urls := fetchURLs(session)
		execution, err := agentruntime.ExecuteScheduledCapability(ctx, agentruntime.CapabilityExecutionRequest{
			Session:         session,
			Node:            "fetch",
			PatternAction:   "reactive_fetch",
			Handle:          fetchCapability,
			Input:           agentfetch.CapabilityInput{URLs: urls},
			StartSummary:    strings.Join(urls, ", "),
			ResultSummary:   strings.Join(urls, ", "),
			EmitStartOnSkip: false,
		})
		if err != nil {
			return agentruntime.NodeResult{}, err
		}
		if _, ok := execution.Invocation.Output.(agentfetch.Output); !ok && execution.Invocation.Status != agentcapability.StatusSkipped {
			return agentruntime.NodeResult{}, fmt.Errorf("fetch capability returned unexpected output type %T", execution.Invocation.Output)
		}

		return agentruntime.NodeResult{
			Events: execution.Events,
			Delta:  withExecutionNodeDelta(execution.Invocation.Delta, "fetch"),
		}, nil
	})
}

func fetchURLs(session *agentruntime.RuntimeSession) []string {
	if session == nil {
		return nil
	}
	return buildFetchURLList(
		session.Snapshot.Context.SearchResults,
		session.Snapshot.Context.SeenURLs,
		session.Snapshot.Context.FetchResults,
		session.Snapshot.Context.PreferredURLs,
		session.Snapshot.Context.AvoidURLs,
	)
}

func buildFetchURLList(results []agentstate.SearchResultRef, seen []string, fetchResults []agentstate.FetchResultRef, preferred []string, avoid []string) []string {
	searchURLs := collectSearchURLs(results)
	if len(searchURLs) == 0 {
		return nil
	}

	known := make(map[string]struct{}, len(searchURLs))
	for _, item := range searchURLs {
		known[item] = struct{}{}
	}
	seenSet := make(map[string]struct{}, len(seen))
	for _, item := range seen {
		seenSet[item] = struct{}{}
	}
	avoidSet := make(map[string]struct{}, len(avoid))
	for _, item := range avoid {
		avoidSet[item] = struct{}{}
	}
	retryable := make(map[string]struct{}, len(fetchResults))
	for _, item := range fetchResults {
		if item.Degraded && item.URL != "" {
			retryable[item.URL] = struct{}{}
		}
	}

	result := make([]string, 0, len(searchURLs))
	added := make(map[string]struct{}, len(searchURLs))
	appendURL := func(url string, allowSeen bool) {
		if url == "" {
			return
		}
		if _, ok := known[url]; !ok {
			return
		}
		if _, ok := avoidSet[url]; ok {
			return
		}
		if _, ok := added[url]; ok {
			return
		}
		if !allowSeen {
			if _, ok := seenSet[url]; ok {
				if _, retry := retryable[url]; !retry {
					return
				}
			}
		}
		added[url] = struct{}{}
		result = append(result, url)
	}

	for _, item := range preferred {
		appendURL(strings.TrimSpace(item), true)
	}
	for _, item := range searchURLs {
		appendURL(item, false)
	}
	return result
}

func collectSearchURLs(results []agentstate.SearchResultRef) []string {
	urls := make([]string, 0, len(results))
	seen := make(map[string]struct{}, len(results))
	for _, item := range results {
		url := strings.TrimSpace(item.URL)
		if url == "" {
			continue
		}
		if _, ok := seen[url]; ok {
			continue
		}
		seen[url] = struct{}{}
		urls = append(urls, url)
	}
	return urls
}
