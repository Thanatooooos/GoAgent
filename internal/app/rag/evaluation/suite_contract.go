package evaluation

import (
	"fmt"
	"strings"
)

type SuiteName string

const (
	SuiteSummary SuiteName = "summary"
	SuiteRewrite SuiteName = "rewrite"
	SuiteAll     SuiteName = "all"
)

func ParseSuiteName(raw string) (SuiteName, error) {
	name := SuiteName(strings.ToLower(strings.TrimSpace(raw)))
	switch name {
	case SuiteSummary, SuiteRewrite, SuiteAll:
		return name, nil
	default:
		return "", fmt.Errorf("unsupported suite %q", strings.TrimSpace(raw))
	}
}

func (s SuiteName) IsExecutableSuite() bool {
	return s == SuiteSummary || s == SuiteRewrite
}

func (s SuiteName) IsAggregateSuite() bool {
	return s == SuiteAll
}
