package builtin

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	SourcePolicyAllow   = "allow"
	SourcePolicyNeutral = "neutral"
	SourcePolicyDeny    = "deny"

	SourceTypeOfficialDocs = "official_docs"
	SourceTypeRepository   = "repository"
	SourceTypeForum        = "forum"
	SourceTypeBlog         = "blog"
	SourceTypeGovernment   = "government"
	SourceTypeAcademic     = "academic"
	SourceTypeUnknown      = "unknown"
)

type SourcePolicyConfig struct {
	AllowDomains  []string
	DenyDomains   []string
	AllowSuffixes []string
	DenySuffixes  []string
}

type SourceAssessment struct {
	Domain     string
	Policy     string
	SourceType string
	RiskFlags  []string
	Reasons    []string
}

type SourcePolicyEngine struct {
	allowDomains  []string
	denyDomains   []string
	allowSuffixes []string
	denySuffixes  []string
}

func NewSourcePolicyEngine(cfg SourcePolicyConfig) *SourcePolicyEngine {
	return &SourcePolicyEngine{
		allowDomains:  normalizeHostRules(cfg.AllowDomains),
		denyDomains:   normalizeHostRules(cfg.DenyDomains),
		allowSuffixes: normalizeSuffixRules(cfg.AllowSuffixes),
		denySuffixes:  normalizeSuffixRules(cfg.DenySuffixes),
	}
}

func (e *SourcePolicyEngine) Evaluate(rawURL string) SourceAssessment {
	domain := extractSourceDomain(rawURL)
	assessment := SourceAssessment{
		Domain:     domain,
		Policy:     SourcePolicyNeutral,
		SourceType: inferSourceType(domain),
	}
	if domain == "" {
		assessment.RiskFlags = append(assessment.RiskFlags, "unknown_domain")
		assessment.Reasons = append(assessment.Reasons, "could not parse source domain")
		return assessment
	}

	switch assessment.SourceType {
	case SourceTypeForum:
		assessment.RiskFlags = append(assessment.RiskFlags, "user_generated")
		assessment.Reasons = append(assessment.Reasons, "source type inferred as forum")
	case SourceTypeBlog:
		assessment.RiskFlags = append(assessment.RiskFlags, "opinionated_source")
		assessment.Reasons = append(assessment.Reasons, "source type inferred as blog")
	}

	if e.matchesDomain(domain, e.denyDomains) {
		assessment.Policy = SourcePolicyDeny
		assessment.RiskFlags = append(assessment.RiskFlags, "deny_listed_domain")
		assessment.Reasons = append(assessment.Reasons, fmt.Sprintf("domain %s matched deny list", domain))
		return assessment.normalize()
	}
	if e.matchesSuffix(domain, e.denySuffixes) {
		assessment.Policy = SourcePolicyDeny
		assessment.RiskFlags = append(assessment.RiskFlags, "deny_listed_suffix")
		assessment.Reasons = append(assessment.Reasons, fmt.Sprintf("domain %s matched deny suffix rule", domain))
		return assessment.normalize()
	}
	if e.matchesDomain(domain, e.allowDomains) {
		assessment.Policy = SourcePolicyAllow
		assessment.Reasons = append(assessment.Reasons, fmt.Sprintf("domain %s matched allow list", domain))
		return assessment.normalize()
	}
	if e.matchesSuffix(domain, e.allowSuffixes) {
		assessment.Policy = SourcePolicyAllow
		assessment.Reasons = append(assessment.Reasons, fmt.Sprintf("domain %s matched allow suffix rule", domain))
		return assessment.normalize()
	}

	assessment.Reasons = append(assessment.Reasons, "domain did not match any allow/deny rule")
	return assessment.normalize()
}

func (e *SourcePolicyEngine) matchesDomain(domain string, rules []string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if domain == rule || strings.HasSuffix(domain, "."+rule) {
			return true
		}
	}
	return false
}

func (e *SourcePolicyEngine) matchesSuffix(domain string, rules []string) bool {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" || len(rules) == 0 {
		return false
	}
	for _, rule := range rules {
		if strings.HasSuffix(domain, rule) {
			return true
		}
	}
	return false
}

func (a SourceAssessment) normalize() SourceAssessment {
	a.Policy = strings.TrimSpace(strings.ToLower(a.Policy))
	if a.Policy == "" {
		a.Policy = SourcePolicyNeutral
	}
	a.SourceType = strings.TrimSpace(strings.ToLower(a.SourceType))
	if a.SourceType == "" {
		a.SourceType = SourceTypeUnknown
	}
	a.RiskFlags = uniqueTrimmedValues(a.RiskFlags)
	a.Reasons = uniqueTrimmedValues(a.Reasons)
	return a
}

func extractSourceDomain(rawURL string) string {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(strings.ToLower(parsed.Hostname()))
	host = strings.TrimPrefix(host, "www.")
	return host
}

func inferSourceType(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	switch {
	case domain == "":
		return SourceTypeUnknown
	case strings.HasSuffix(domain, ".gov"):
		return SourceTypeGovernment
	case strings.HasSuffix(domain, ".edu"):
		return SourceTypeAcademic
	case domain == "github.com" || domain == "gitlab.com" || strings.HasSuffix(domain, ".github.io"):
		return SourceTypeRepository
	case strings.Contains(domain, "stackoverflow.com") || strings.Contains(domain, "reddit.com") ||
		strings.Contains(domain, "news.ycombinator.com") || strings.HasPrefix(domain, "forum.") ||
		strings.HasPrefix(domain, "discuss."):
		return SourceTypeForum
	case strings.Contains(domain, "medium.com") || strings.Contains(domain, "substack.com") ||
		strings.Contains(domain, "dev.to") || strings.HasPrefix(domain, "blog."):
		return SourceTypeBlog
	case strings.HasPrefix(domain, "docs.") || strings.HasPrefix(domain, "developer.") ||
		strings.HasPrefix(domain, "support.") || domain == "go.dev" || domain == "pkg.go.dev":
		return SourceTypeOfficialDocs
	default:
		return SourceTypeUnknown
	}
}

func normalizeHostRules(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(strings.ToLower(item))
		item = strings.TrimPrefix(item, "https://")
		item = strings.TrimPrefix(item, "http://")
		item = strings.TrimPrefix(item, "www.")
		item = strings.Trim(item, "/")
		if item == "" {
			continue
		}
		normalized = append(normalized, item)
	}
	return uniqueTrimmedValues(normalized)
}

func normalizeSuffixRules(items []string) []string {
	normalized := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(strings.ToLower(item))
		if item == "" {
			continue
		}
		if !strings.HasPrefix(item, ".") {
			item = "." + item
		}
		normalized = append(normalized, item)
	}
	return uniqueTrimmedValues(normalized)
}

func uniqueTrimmedValues(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	values := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		values = append(values, item)
	}
	return values
}
