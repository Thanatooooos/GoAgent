// Package websource owns policy evaluation for public-web evidence.
package websource

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	PolicyAllow            = "allow"
	PolicyNeutral          = "neutral"
	PolicyDeny             = "deny"
	SourceTypeOfficialDocs = "official_docs"
	SourceTypeRepository   = "repository"
	SourceTypeForum        = "forum"
	SourceTypeBlog         = "blog"
	SourceTypeGovernment   = "government"
	SourceTypeAcademic     = "academic"
	SourceTypeUnknown      = "unknown"
)

type Config struct {
	AllowDomains  []string
	DenyDomains   []string
	AllowSuffixes []string
	DenySuffixes  []string
}

type Assessment struct {
	Domain     string   `json:"domain"`
	Policy     string   `json:"policy"`
	SourceType string   `json:"sourceType"`
	RiskFlags  []string `json:"riskFlags,omitempty"`
	Reasons    []string `json:"reasons,omitempty"`
}

type Policy interface{ Evaluate(string) Assessment }

type Engine struct{ allowDomains, denyDomains, allowSuffixes, denySuffixes []string }

func New(cfg Config) *Engine {
	return &Engine{allowDomains: hosts(cfg.AllowDomains), denyDomains: hosts(cfg.DenyDomains), allowSuffixes: suffixes(cfg.AllowSuffixes), denySuffixes: suffixes(cfg.DenySuffixes)}
}

func (e *Engine) Evaluate(rawURL string) Assessment {
	domain := domain(rawURL)
	if domain == "" {
		return Assessment{Policy: PolicyNeutral, SourceType: SourceTypeUnknown, RiskFlags: []string{"unknown_domain"}, Reasons: []string{"could not parse source domain"}}
	}
	assessment := Assessment{Domain: domain, Policy: PolicyNeutral, SourceType: sourceType(domain)}
	if assessment.SourceType == SourceTypeForum {
		assessment.RiskFlags = append(assessment.RiskFlags, "user_generated")
	}
	if assessment.SourceType == SourceTypeBlog {
		assessment.RiskFlags = append(assessment.RiskFlags, "opinionated_source")
	}
	if matches(domain, e.denyDomains) || matchesSuffix(domain, e.denySuffixes) {
		assessment.Policy = PolicyDeny
		assessment.RiskFlags = append(assessment.RiskFlags, "deny_listed_domain")
		assessment.Reasons = append(assessment.Reasons, fmt.Sprintf("domain %s matched deny policy", domain))
		return assessment
	}
	if matches(domain, e.allowDomains) || matchesSuffix(domain, e.allowSuffixes) {
		assessment.Policy = PolicyAllow
		assessment.Reasons = append(assessment.Reasons, fmt.Sprintf("domain %s matched allow policy", domain))
		return assessment
	}
	assessment.Reasons = append(assessment.Reasons, "domain did not match any allow or deny rule")
	return assessment
}

func sourceType(domain string) string {
	switch {
	case strings.HasSuffix(domain, ".gov"):
		return SourceTypeGovernment
	case strings.HasSuffix(domain, ".edu"):
		return SourceTypeAcademic
	case domain == "github.com" || domain == "gitlab.com" || strings.HasSuffix(domain, ".github.io"):
		return SourceTypeRepository
	case strings.Contains(domain, "stackoverflow.com") || strings.Contains(domain, "reddit.com") || strings.Contains(domain, "news.ycombinator.com") || strings.HasPrefix(domain, "forum.") || strings.HasPrefix(domain, "discuss."):
		return SourceTypeForum
	case strings.Contains(domain, "medium.com") || strings.Contains(domain, "substack.com") || strings.Contains(domain, "dev.to") || strings.HasPrefix(domain, "blog."):
		return SourceTypeBlog
	case strings.HasPrefix(domain, "docs.") || strings.HasPrefix(domain, "developer.") || strings.HasPrefix(domain, "support.") || domain == "go.dev" || domain == "pkg.go.dev":
		return SourceTypeOfficialDocs
	default:
		return SourceTypeUnknown
	}
}

func domain(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(parsed.Hostname())), "www.")
}

func matches(domain string, rules []string) bool {
	for _, rule := range rules {
		if domain == rule || strings.HasSuffix(domain, "."+rule) {
			return true
		}
	}
	return false
}

func matchesSuffix(domain string, rules []string) bool {
	for _, rule := range rules {
		if strings.HasSuffix(domain, rule) {
			return true
		}
	}
	return false
}

func hosts(values []string) []string {
	return normalize(values, func(value string) string {
		value = strings.TrimPrefix(strings.TrimPrefix(strings.ToLower(strings.TrimSpace(value)), "https://"), "http://")
		return strings.Trim(strings.TrimPrefix(value, "www."), "/")
	})
}

func suffixes(values []string) []string {
	return normalize(values, func(value string) string {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" && !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		return value
	})
}

func normalize(values []string, clean func(string) string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = clean(value)
		if value != "" {
			if _, ok := seen[value]; !ok {
				seen[value] = struct{}{}
				result = append(result, value)
			}
		}
	}
	return result
}
