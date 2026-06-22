package evaluation

import (
	"context"
	"fmt"
)

type Judge interface {
	Evaluate(ctx context.Context, req JudgeRequest) (JudgeResult, error)
}

type JudgeRequest struct {
	PromptRef string         `json:"prompt_ref"`
	RubricRef string         `json:"rubric_ref"`
	Payload   map[string]any `json:"payload"`
	Config    JudgeConfig    `json:"config"`
}

type JudgeConfig struct {
	Model       string  `json:"model"`
	Temperature float64 `json:"temperature"`
	MaxTokens   int     `json:"max_tokens"`
}

type JudgeResult struct {
	Passed          bool     `json:"passed"`
	Score           float64  `json:"score"`
	MissedItems     []string `json:"missed_items,omitempty"`
	IncorrectClaims []string `json:"incorrect_claims,omitempty"`
	Reason          string   `json:"reason,omitempty"`
	Details         map[string]any `json:"details,omitempty"`
}

type JudgeParseError struct {
	Raw   string
	Cause error
}

func (e *JudgeParseError) Error() string {
	if e == nil {
		return ""
	}
	return fmt.Sprintf("parse judge output: %v", e.Cause)
}

func (e *JudgeParseError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}
