package llmgen

import "strings"

type RejectReason string

const (
	ReasonOK                RejectReason = ""
	ReasonNotInCandidateSet RejectReason = "reference_not_in_candidate_set"
	ReasonMalformed         RejectReason = "malformed_reference"
	ReasonEmpty             RejectReason = "empty_reference"
)

// RefValidator deterministically checks that a reference resolves to an
// allowed candidate id. Rules are rule-based (never LLM-judged).
type RefValidator struct {
	allowed map[string]struct{}
}

func NewRefValidator(candidates []string) *RefValidator {
	allowed := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		allowed[candidate] = struct{}{}
	}
	return &RefValidator{allowed: allowed}
}

// Validate returns the canonical candidate id (ReasonOK) or a rejection reason.
func (v *RefValidator) Validate(ref string) (string, RejectReason) {
	if v == nil {
		return "", ReasonMalformed
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", ReasonEmpty
	}
	if _, ok := v.allowed[ref]; ok {
		return ref, ReasonOK
	}
	return "", ReasonNotInCandidateSet
}

type RefResult struct {
	Ref    string
	ID     string
	Reason RejectReason
}

func (v *RefValidator) ValidateMany(refs []string) []RefResult {
	results := make([]RefResult, 0, len(refs))
	for _, ref := range refs {
		id, reason := v.Validate(ref)
		results = append(results, RefResult{Ref: ref, ID: id, Reason: reason})
	}
	return results
}
