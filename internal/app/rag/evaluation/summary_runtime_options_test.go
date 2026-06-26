package evaluation

import (
	"reflect"
	"strings"
	"testing"
)

func TestSummaryEvaluatorRuntimeOptionsNormalizeTokenStrategy(t *testing.T) {
	options, err := (SummaryEvaluatorRuntimeOptions{
		Mode:                  SummaryEvalModeStrategy,
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{1600, 800, 1600, -1},
		MessageOverheadTokens: 4,
	}).normalizedStrategy()
	if err != nil {
		t.Fatalf("normalizedStrategy() error = %v", err)
	}
	if options.ThresholdUnit != SummaryStrategyThresholdTokens {
		t.Fatalf("ThresholdUnit = %q", options.ThresholdUnit)
	}
	if !reflect.DeepEqual(options.Thresholds, []int{800, 1600}) {
		t.Fatalf("Thresholds = %#v", options.Thresholds)
	}
}

func TestSummaryEvaluatorRuntimeOptionsRejectStrategyWithoutThresholds(t *testing.T) {
	_, err := (SummaryEvaluatorRuntimeOptions{
		Mode:          SummaryEvalModeStrategy,
		ThresholdUnit: SummaryStrategyThresholdTokens,
	}).normalizedStrategy()
	if err == nil || !strings.Contains(err.Error(), "summary strategy thresholds are required") {
		t.Fatalf("error = %v", err)
	}
}

func TestSummaryEvaluatorRuntimeOptionsRejectUnsupportedUnit(t *testing.T) {
	_, err := (SummaryEvaluatorRuntimeOptions{
		Mode:          SummaryEvalModeStrategy,
		ThresholdUnit: "characters",
		Thresholds:    []int{1000},
	}).normalizedStrategy()
	if err == nil || !strings.Contains(err.Error(), "unsupported summary strategy threshold unit") {
		t.Fatalf("error = %v", err)
	}
}
