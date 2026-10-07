package capability

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"local/rag-project/internal/app/rag/core/citation"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/app/runtime/websource"
)

const (
	WebSearchID = "web_search"
	WebFetchID  = "web_fetch"
)

type WebSearchService interface {
	Search(context.Context, string) ([]WebSearchResult, error)
}
type WebSearchResult struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Domain  string `json:"domain,omitempty"`
	Policy  string `json:"policy,omitempty"`
}
type WebFetchService interface {
	Fetch(context.Context, []string) ([]WebPage, error)
}
type ScopedWebFetchService interface {
	FetchScoped(context.Context, []string, []string) ([]WebPage, error)
}
type WebPage struct {
	URL   string `json:"url"`
	Text  string `json:"text"`
	Error string `json:"error,omitempty"`
}

type webSearchContextItem struct {
	WebSearchResult
	CitationID string `json:"citation_id,omitempty"`
}

type webFetchContextPage struct {
	WebPage
	CitationID string `json:"citation_id,omitempty"`
}

func WebSearch(service WebSearchService, policies ...websource.Policy) Def {
	policy := websource.Policy(websource.New(websource.Config{}))
	if len(policies) > 0 && policies[0] != nil {
		policy = policies[0]
	}
	return Def{ID: WebSearchID, Description: webSearchDescription, JSONSchema: json.RawMessage(webSearchSchema),
		Validate: func(v Value) error {
			var x struct {
				Query string `json:"query"`
			}
			if err := json.Unmarshal(v, &x); err != nil || strings.TrimSpace(x.Query) == "" {
				return fmt.Errorf("query is required")
			}
			return nil
		},
		Describe: func(v Value, c Context) (Operation, error) {
			if !c.AllowWebSearch {
				return Operation{}, Deny("web search is not permitted")
			}
			return Operation{ID: WebSearchID, Input: v}, nil
		},
		Execute: func(v Value, c Context) (Result, error) {
			var x struct {
				Query string `json:"query"`
			}
			_ = json.Unmarshal(v, &x)
			items, err := service.Search(c.Context, x.Query)
			if err != nil {
				return Result{}, err
			}
			accepted := items[:0]
			for _, item := range items {
				assessment := policy.Evaluate(item.URL)
				if assessment.Policy == websource.PolicyDeny || !taskWebDomainAllowed(item.URL, c.AllowedWebDomains) {
					continue
				}
				item.Domain, item.Policy = assessment.Domain, assessment.Policy
				accepted = append(accepted, item)
			}
			items = accepted
			contextItems := make([]webSearchContextItem, 0, len(items))
			refs := make([]persistence.EvidenceRef, 0, len(items))
			for _, i := range items {
				handle := c.Citations.RegisterWeb(citation.WebReference{URL: i.URL, Title: i.Title})
				contextItems = append(contextItems, webSearchContextItem{WebSearchResult: i, CitationID: handle})
				refs = append(refs, persistence.EvidenceRef{Kind: "web", URL: i.URL})
			}
			raw, _ := json.Marshal(contextItems)
			return Result{Content: string(raw), Value: raw, Evidence: refs}, nil
		}}
}

func WebFetch(service WebFetchService, policies ...websource.Policy) Def {
	policy := websource.Policy(websource.New(websource.Config{}))
	if len(policies) > 0 && policies[0] != nil {
		policy = policies[0]
	}
	return Def{ID: WebFetchID, Description: webFetchDescription, JSONSchema: json.RawMessage(webFetchSchema),
		Validate: func(v Value) error {
			var x struct {
				URLs []string `json:"urls"`
			}
			if err := json.Unmarshal(v, &x); err != nil || len(x.URLs) == 0 || len(x.URLs) > 3 {
				return fmt.Errorf("one to three urls are required")
			}
			return nil
		},
		Describe: func(v Value, c Context) (Operation, error) {
			if !c.AllowWebSearch {
				return Operation{}, Deny("web fetch is not permitted")
			}
			var x struct {
				URLs []string `json:"urls"`
			}
			if err := json.Unmarshal(v, &x); err != nil {
				return Operation{}, err
			}
			for _, rawURL := range x.URLs {
				if policy.Evaluate(rawURL).Policy == websource.PolicyDeny || !taskWebDomainAllowed(rawURL, c.AllowedWebDomains) {
					return Operation{}, Deny("web fetch source is not permitted")
				}
			}
			return Operation{ID: WebFetchID, Input: v}, nil
		},
		Execute: func(v Value, c Context) (Result, error) {
			var x struct {
				URLs []string `json:"urls"`
			}
			_ = json.Unmarshal(v, &x)
			for _, rawURL := range x.URLs {
				if policy.Evaluate(rawURL).Policy == websource.PolicyDeny || !taskWebDomainAllowed(rawURL, c.AllowedWebDomains) {
					return Result{}, Deny("web fetch source is not permitted")
				}
			}
			var pages []WebPage
			var err error
			if len(c.AllowedWebDomains) > 0 {
				scoped, ok := service.(ScopedWebFetchService)
				if !ok {
					return Result{}, fmt.Errorf("scoped web fetch service is required")
				}
				pages, err = scoped.FetchScoped(c.Context, x.URLs, c.AllowedWebDomains)
			} else {
				pages, err = service.Fetch(c.Context, x.URLs)
			}
			if err != nil {
				return Result{}, err
			}
			contextPages := make([]webFetchContextPage, 0, len(pages))
			refs := make([]persistence.EvidenceRef, 0, len(pages))
			for _, p := range pages {
				if policy.Evaluate(p.URL).Policy == websource.PolicyDeny || !taskWebDomainAllowed(p.URL, c.AllowedWebDomains) {
					continue
				}
				handle := c.Citations.RegisterWeb(citation.WebReference{URL: p.URL})
				contextPages = append(contextPages, webFetchContextPage{WebPage: p, CitationID: handle})
				refs = append(refs, persistence.EvidenceRef{Kind: "web", URL: p.URL})
			}
			raw, _ := json.Marshal(contextPages)
			return Result{Content: string(raw), Value: raw, Evidence: refs}, nil
		}}
}

func taskWebDomainAllowed(rawURL string, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return false
	}
	host := strings.TrimPrefix(strings.ToLower(parsed.Hostname()), "www.")
	for _, item := range allowed {
		domain := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(item)), "www.")
		if domain != "" && (host == domain || strings.HasSuffix(host, "."+domain)) {
			return true
		}
	}
	return false
}

// WebDomainAllowed is also used by the fetch adapter before each redirect.
func WebDomainAllowed(rawURL string, allowed []string) bool {
	return taskWebDomainAllowed(rawURL, allowed)
}
