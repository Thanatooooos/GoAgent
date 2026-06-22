package rewrite

import (
	"regexp"
	"strings"
)

const (
	ConstraintErrorCode = "error_code"
	ConstraintHTTPCode  = "http_code"
	ConstraintID        = "id"
	ConstraintQuoted    = "quoted"
	ConstraintTimeRange = "time_range"
	ConstraintLimit     = "limit"
)

var (
	quotedBacktickPattern = regexp.MustCompile("`([^`]+)`")
	quotedDoublePattern   = regexp.MustCompile(`"([^"]+)"`)
	quotedSinglePattern   = regexp.MustCompile(`'([^']+)'`)
	idPattern             = regexp.MustCompile(`(?i)\b(?:doc|task|trace|kb)[-_][a-z0-9][a-z0-9_-]*\b`)
	httpCodePattern       = regexp.MustCompile(`(?i)\b(?:http\s*)?[1-5][0-9]{2}\b`)
	errorCodePattern      = regexp.MustCompile(`\b[A-Z][A-Z0-9_-]{1,20}[0-9][A-Z0-9_-]*\b`)
	httpProtocolPattern   = regexp.MustCompile(`(?i)^HTTP\d+$`)
)

var timeRangeMarkers = []string{
	"最近一次",
	"昨天",
	"今天",
	"近 7 天",
	"近7天",
	"last run",
	"latest",
}

var limitMarkers = []string{
	"only",
	"不要查外部",
	"只看",
}

type RewriteConstraint struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

type RewriteValidationReport struct {
	Accepted            bool                `json:"accepted"`
	Reasons             []string            `json:"reasons,omitempty"`
	OriginalConstraints []RewriteConstraint `json:"originalConstraints,omitempty"`
	MissingConstraints  []RewriteConstraint `json:"missingConstraints,omitempty"`
}

func ExtractRewriteConstraints(question string) []RewriteConstraint {
	question = strings.TrimSpace(question)
	if question == "" {
		return nil
	}

	constraints := make([]RewriteConstraint, 0)
	seen := make(map[string]struct{})

	addQuotedConstraints(&constraints, seen, quotedBacktickPattern, question)
	addQuotedConstraints(&constraints, seen, quotedDoublePattern, question)
	addQuotedConstraints(&constraints, seen, quotedSinglePattern, question)

	for _, value := range idPattern.FindAllString(question, -1) {
		addConstraint(&constraints, seen, RewriteConstraint{Type: ConstraintID, Value: value})
	}
	for _, value := range httpCodePattern.FindAllString(question, -1) {
		addConstraint(&constraints, seen, RewriteConstraint{Type: ConstraintHTTPCode, Value: value})
	}
	for _, value := range errorCodePattern.FindAllString(question, -1) {
		if httpProtocolPattern.MatchString(value) {
			continue
		}
		addConstraint(&constraints, seen, RewriteConstraint{Type: ConstraintErrorCode, Value: value})
	}

	lower := strings.ToLower(question)
	for _, marker := range timeRangeMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) || strings.Contains(question, marker) {
			addConstraint(&constraints, seen, RewriteConstraint{Type: ConstraintTimeRange, Value: marker})
		}
	}
	for _, marker := range limitMarkers {
		if strings.Contains(lower, strings.ToLower(marker)) || strings.Contains(question, marker) {
			addConstraint(&constraints, seen, RewriteConstraint{Type: ConstraintLimit, Value: marker})
		}
	}

	return constraints
}

func GuardRewriteResult(original string, result Result) (Result, RewriteValidationReport) {
	original = strings.TrimSpace(original)
	constraints := ExtractRewriteConstraints(original)
	if len(constraints) == 0 {
		report := RewriteValidationReport{Accepted: true}
		return result, report
	}

	candidate := buildRewriteCandidateText(result)
	missing := make([]RewriteConstraint, 0)
	for _, constraint := range constraints {
		if !constraintInCandidate(candidate, constraint) {
			missing = append(missing, constraint)
		}
	}
	if len(missing) == 0 {
		report := RewriteValidationReport{
			Accepted:            true,
			OriginalConstraints: constraints,
		}
		metadata := cloneMetadata(result.Metadata)
		metadata = setMetadataValue(metadata, "rewriteValidation", report)
		result.Metadata = metadata
		return result, report
	}

	report := RewriteValidationReport{
		Accepted:            false,
		Reasons:             []string{"rewritten result dropped hard constraints"},
		OriginalConstraints: constraints,
		MissingConstraints:  missing,
	}
	fallback := fallbackResult(original)
	metadata := cloneMetadata(result.Metadata)
	metadata = setMetadataValue(metadata, "rewriteValidation", report)
	fallback.Metadata = metadata
	return fallback, report
}

func buildRewriteCandidateText(result Result) string {
	parts := make([]string, 0, 1+len(result.SubQuestions))
	if rewritten := strings.TrimSpace(result.RewrittenQuestion); rewritten != "" {
		parts = append(parts, rewritten)
	}
	for _, question := range result.SubQuestions {
		if question = strings.TrimSpace(question); question != "" {
			parts = append(parts, question)
		}
	}
	return strings.Join(parts, " ")
}

func constraintInCandidate(candidate string, constraint RewriteConstraint) bool {
	value := strings.TrimSpace(constraint.Value)
	if value == "" {
		return true
	}
	switch constraint.Type {
	case ConstraintQuoted, ConstraintID, ConstraintHTTPCode, ConstraintErrorCode:
		return strings.Contains(strings.ToLower(candidate), strings.ToLower(value))
	default:
		return strings.Contains(candidate, value) || strings.Contains(strings.ToLower(candidate), strings.ToLower(value))
	}
}

func addQuotedConstraints(constraints *[]RewriteConstraint, seen map[string]struct{}, pattern *regexp.Regexp, text string) {
	for _, match := range pattern.FindAllStringSubmatch(text, -1) {
		if len(match) < 2 {
			continue
		}
		value := strings.TrimSpace(match[1])
		if value == "" {
			continue
		}
		addConstraint(constraints, seen, RewriteConstraint{Type: ConstraintQuoted, Value: value})
	}
}

func addConstraint(constraints *[]RewriteConstraint, seen map[string]struct{}, constraint RewriteConstraint) {
	constraint.Type = strings.TrimSpace(constraint.Type)
	constraint.Value = strings.TrimSpace(constraint.Value)
	if constraint.Type == "" || constraint.Value == "" {
		return
	}
	key := constraint.Type + ":" + strings.ToLower(constraint.Value)
	if _, ok := seen[key]; ok {
		return
	}
	seen[key] = struct{}{}
	*constraints = append(*constraints, constraint)
}
