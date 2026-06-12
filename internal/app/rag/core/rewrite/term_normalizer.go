package rewrite

import (
	"regexp"
	"sort"
	"strings"

	"local/rag-project/internal/framework/convention"
)

type TermNormalizationRule struct {
	Canonical string
	Aliases   []string
	Category  string
	Version   int
	Enabled   *bool
}

type TermNormalizationOptions struct {
	Enabled bool
	Rules   []TermNormalizationRule
}

type TermNormalizationMatch struct {
	Alias     string `json:"alias"`
	Canonical string `json:"canonical"`
	Category  string `json:"category,omitempty"`
	Version   int    `json:"version,omitempty"`
}

type TermNormalizationReport struct {
	Changed bool                     `json:"changed"`
	Matches []TermNormalizationMatch `json:"matches,omitempty"`
}

type TermNormalizingService struct {
	base       Service
	normalizer *TermNormalizer
}

type TermNormalizer struct {
	entries []termNormalizationEntry
}

type termNormalizationEntry struct {
	canonical string
	alias     string
	category  string
	version   int
	pattern   *regexp.Regexp
}

func NewTermNormalizingService(base Service, options TermNormalizationOptions) Service {
	if base == nil {
		return nil
	}
	normalizer := NewTermNormalizer(options)
	if normalizer == nil {
		return base
	}
	return &TermNormalizingService{
		base:       base,
		normalizer: normalizer,
	}
}

func NewTermNormalizer(options TermNormalizationOptions) *TermNormalizer {
	if !options.Enabled {
		return nil
	}

	type candidate struct {
		canonical string
		alias     string
		category  string
		version   int
	}

	candidates := make([]candidate, 0)
	seen := make(map[string]struct{})
	for _, rule := range options.Rules {
		if rule.Enabled != nil && !*rule.Enabled {
			continue
		}
		canonical := strings.TrimSpace(rule.Canonical)
		if canonical == "" {
			continue
		}
		for _, alias := range rule.Aliases {
			alias = strings.TrimSpace(alias)
			if alias == "" {
				continue
			}
			key := strings.ToLower(alias) + "->" + strings.ToLower(canonical)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, candidate{
				canonical: canonical,
				alias:     alias,
				category:  strings.TrimSpace(rule.Category),
				version:   rule.Version,
			})
		}
	}
	if len(candidates) == 0 {
		return nil
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if len([]rune(candidates[i].alias)) != len([]rune(candidates[j].alias)) {
			return len([]rune(candidates[i].alias)) > len([]rune(candidates[j].alias))
		}
		return candidates[i].alias < candidates[j].alias
	})

	entries := make([]termNormalizationEntry, 0, len(candidates))
	for _, item := range candidates {
		pattern := compileAliasPattern(item.alias)
		if pattern == nil {
			continue
		}
		entries = append(entries, termNormalizationEntry{
			canonical: item.canonical,
			alias:     item.alias,
			category:  item.category,
			version:   item.version,
			pattern:   pattern,
		})
	}
	if len(entries) == 0 {
		return nil
	}
	return &TermNormalizer{entries: entries}
}

func (s *TermNormalizingService) Rewrite(question string) string {
	if s == nil || s.base == nil {
		return normalize(question)
	}
	if s.normalizer == nil {
		return s.base.Rewrite(question)
	}
	return s.normalizer.NormalizeText(s.base.Rewrite(question))
}

func (s *TermNormalizingService) RewriteWithSplit(question string) Result {
	if s == nil || s.base == nil {
		return fallbackResult(question)
	}
	result := s.base.RewriteWithSplit(question)
	if s.normalizer == nil {
		return result
	}
	return s.normalizer.Apply(result)
}

func (s *TermNormalizingService) RewriteWithHistory(question string, history []convention.ChatMessage) Result {
	if s == nil || s.base == nil {
		return fallbackResult(question)
	}
	result := s.base.RewriteWithHistory(question, history)
	if s.normalizer == nil {
		return result
	}
	return s.normalizer.Apply(result)
}

func (n *TermNormalizer) Apply(result Result) Result {
	if n == nil {
		return result
	}

	rewrittenOriginal := strings.TrimSpace(result.RewrittenQuestion)
	normalizedText, rewrittenReport := n.NormalizeTextWithReport(rewrittenOriginal)
	rewritten := normalizedText
	if rewritten == "" {
		rewritten = rewrittenOriginal
	}

	normalizedSubs := make([]string, 0, len(result.SubQuestions))
	mergedReport := rewrittenReport
	for _, question := range result.SubQuestions {
		original := strings.TrimSpace(question)
		if original == "" {
			continue
		}
		normalized, subReport := n.NormalizeTextWithReport(original)
		if normalized == "" {
			normalized = original
		}
		normalizedSubs = append(normalizedSubs, normalized)
		mergedReport = mergeTermNormalizationReports(mergedReport, subReport)
	}
	if len(normalizedSubs) == 0 && rewritten != "" {
		normalizedSubs = []string{rewritten}
	}

	metadata := cloneMetadata(result.Metadata)
	if mergedReport.Changed {
		metadata = setMetadataValue(metadata, "termNormalization", mergedReport)
	}

	return Result{
		RewrittenQuestion: rewritten,
		SubQuestions:      normalizeSubQuestions(normalizedSubs, rewritten),
		NeedRetrieval:     result.NeedRetrieval,
		Metadata:          metadata,
	}
}

func (n *TermNormalizer) NormalizeText(text string) string {
	normalized, _ := n.NormalizeTextWithReport(text)
	return normalized
}

func (n *TermNormalizer) NormalizeTextWithReport(text string) (string, TermNormalizationReport) {
	text = strings.TrimSpace(text)
	if n == nil || text == "" {
		return text, TermNormalizationReport{}
	}

	original := text
	matches := make([]TermNormalizationMatch, 0)
	seen := make(map[string]struct{})
	for _, entry := range n.entries {
		if !entry.pattern.MatchString(text) {
			continue
		}
		text = entry.pattern.ReplaceAllString(text, entry.canonical)
		key := strings.ToLower(entry.alias) + "->" + strings.ToLower(entry.canonical)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		matches = append(matches, TermNormalizationMatch{
			Alias:     entry.alias,
			Canonical: entry.canonical,
			Category:  entry.category,
			Version:   entry.version,
		})
	}

	normalized := strings.TrimSpace(text)
	return normalized, TermNormalizationReport{
		Changed: normalized != original,
		Matches: matches,
	}
}

func mergeTermNormalizationReports(left, right TermNormalizationReport) TermNormalizationReport {
	if !left.Changed && !right.Changed {
		return TermNormalizationReport{}
	}
	merged := TermNormalizationReport{
		Changed: left.Changed || right.Changed,
		Matches: append([]TermNormalizationMatch(nil), left.Matches...),
	}
	seen := make(map[string]struct{}, len(merged.Matches))
	for _, match := range merged.Matches {
		seen[termNormalizationMatchKey(match)] = struct{}{}
	}
	for _, match := range right.Matches {
		key := termNormalizationMatchKey(match)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		merged.Matches = append(merged.Matches, match)
	}
	return merged
}

func termNormalizationMatchKey(match TermNormalizationMatch) string {
	return strings.ToLower(match.Alias) + "->" + strings.ToLower(match.Canonical)
}

func cloneMetadata(metadata map[string]any) map[string]any {
	if len(metadata) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(metadata))
	for key, value := range metadata {
		cloned[key] = value
	}
	return cloned
}

func setMetadataValue(metadata map[string]any, key string, value any) map[string]any {
	if metadata == nil {
		metadata = make(map[string]any, 1)
	}
	metadata[key] = value
	return metadata
}

func compileAliasPattern(alias string) *regexp.Regexp {
	alias = strings.TrimSpace(alias)
	if alias == "" {
		return nil
	}
	quoted := regexp.QuoteMeta(alias)
	pattern := "(?i)" + quoted
	if shouldUseWordBoundary(alias) {
		pattern = "(?i)\\b" + quoted + "\\b"
	}
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil
	}
	return compiled
}

func shouldUseWordBoundary(alias string) bool {
	if alias == "" {
		return false
	}
	for _, r := range alias {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ' ' || r == '-' || r == '_':
		default:
			return false
		}
	}
	return true
}
