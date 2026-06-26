package evaluation

import "testing"

func TestBuildSummaryStrategyThresholdAggregatesIncludesThresholdUnit(t *testing.T) {
	aggregates := buildSummaryStrategyThresholdAggregates([]SummaryStrategySampleResult{{
		ThresholdResults: []SummaryStrategyThresholdResult{{
			Threshold:     1200,
			ThresholdUnit: SummaryStrategyThresholdTokens,
			Passed:        true,
		}},
	}})
	if len(aggregates) != 1 {
		t.Fatalf("aggregates len = %d, want 1", len(aggregates))
	}
	if aggregates[0].ThresholdUnit != SummaryStrategyThresholdTokens {
		t.Fatalf("ThresholdUnit = %q, want %q", aggregates[0].ThresholdUnit, SummaryStrategyThresholdTokens)
	}
}

func TestBuildStrategySharedSampleResultIncludesThresholdUnit(t *testing.T) {
	result := buildStrategySharedSampleResult(SummarySample{Name: "sample"}, SummaryStrategySampleResult{
		ThresholdResults: []SummaryStrategyThresholdResult{{
			Threshold:     1200,
			ThresholdUnit: SummaryStrategyThresholdTokens,
			Passed:        true,
		}},
	})
	if got := result.RuleChecks["threshold_unit"]; got != "tokens" {
		t.Fatalf("threshold_unit = %#v, want tokens", got)
	}
}
