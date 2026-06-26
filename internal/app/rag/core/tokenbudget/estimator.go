package tokenbudget

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/infinigence/tokenestimate"

	"local/rag-project/internal/framework/convention"
)

type Estimator interface {
	EstimateTokens(text string) int
}

const (
	DefaultEstimatorName    = "tokenestimate"
	DefaultEstimatorVersion = "v0.1.0"
)

type DefaultEstimator struct {
	base *tokenestimate.Estimator
}

func NewDefaultEstimator() *DefaultEstimator {
	return &DefaultEstimator{base: tokenestimate.NewEstimator()}
}

func (e *DefaultEstimator) EstimateTokens(text string) int {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0
	}
	if e == nil || e.base == nil {
		return tokenestimate.NewEstimator().Estimate(text)
	}
	return e.base.Estimate(text)
}

type FixedEstimator int

func (e FixedEstimator) EstimateTokens(string) int {
	return int(e)
}

type RuneEstimator struct{}

func (RuneEstimator) EstimateTokens(text string) int {
	return utf8.RuneCountInString(strings.TrimSpace(text))
}

func EstimateMessages(messages []convention.ChatMessage, estimator Estimator, overhead int) int {
	if estimator == nil {
		estimator = NewDefaultEstimator()
	}
	if overhead < 0 {
		overhead = 0
	}
	total := 0
	for _, message := range messages {
		total += estimator.EstimateTokens(message.Content) + overhead
	}
	return total
}

func ApplySafetyFactor(tokens int, factor float64) int {
	if tokens <= 0 {
		return 0
	}
	if factor < 1 {
		factor = 1
	}
	return int(math.Ceil(float64(tokens) * factor))
}

func DescribeEstimator(estimator Estimator) (string, string) {
	switch estimator.(type) {
	case *DefaultEstimator:
		return DefaultEstimatorName, DefaultEstimatorVersion
	case nil:
		return "none", ""
	default:
		return fmt.Sprintf("%T", estimator), ""
	}
}
