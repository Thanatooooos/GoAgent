package evaluation

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/core/history"
)

type recordingStrategyGenerator struct {
	inputs []SummaryGenerationInput
}

type nonEmptyFixedEstimator int

func (e nonEmptyFixedEstimator) EstimateTokens(text string) int {
	if strings.TrimSpace(text) == "" {
		return 0
	}
	return int(e)
}

func (g *recordingStrategyGenerator) Generate(
	_ context.Context,
	input SummaryGenerationInput,
) (SummaryGenerationOutput, error) {
	g.inputs = append(g.inputs, input)
	call := len(g.inputs)
	return SummaryGenerationOutput{
		Structured: history.StructuredSummary{
			SchemaVersion: 1,
			Goal:          fmt.Sprintf("summary-%d", call),
		},
		Rendered: fmt.Sprintf("summary-%d", call),
	}, nil
}

func TestRunSummaryStrategyTokenThresholdDoesNotTriggerBelowBudget(t *testing.T) {
	result := runTokenThresholdStrategy(t, twoTurnTokenStrategySample(), 21, 0)
	if result.SummaryCallCount != 0 {
		t.Fatalf("SummaryCallCount = %d, want 0", result.SummaryCallCount)
	}
}

func TestRunSummaryStrategyTokenThresholdTriggersAtEqualBudget(t *testing.T) {
	result := runTokenThresholdStrategy(t, twoTurnTokenStrategySample(), 10, 0)
	if result.SummaryCallCount != 2 {
		t.Fatalf("SummaryCallCount = %d, want 2", result.SummaryCallCount)
	}
}

func TestRunSummaryStrategyTokenThresholdUsesCommittedSummaryPlusTail(t *testing.T) {
	result := runTokenThresholdStrategy(t, threeTurnTokenStrategySample(), 15, 0)
	if result.SummaryCallCount != 2 {
		t.Fatalf("SummaryCallCount = %d, want 2", result.SummaryCallCount)
	}
}

func TestRunSummaryStrategyTokenThresholdCountsMessageOverhead(t *testing.T) {
	result := runTokenThresholdStrategy(t, oneTurnTokenStrategySample(), 18, 4)
	if result.SummaryCallCount != 1 {
		t.Fatalf("SummaryCallCount = %d, want 1", result.SummaryCallCount)
	}
}

func runTokenThresholdStrategy(
	t *testing.T,
	sample SummarySample,
	threshold int,
	messageOverheadTokens int,
) SummaryStrategyThresholdResult {
	t.Helper()
	result, err := runSummaryStrategySweep(context.Background(), summaryStrategyDependencies{
		Generator:             &recordingStrategyGenerator{},
		ThresholdUnit:         SummaryStrategyThresholdTokens,
		Thresholds:            []int{threshold},
		Estimator:             nonEmptyFixedEstimator(5),
		MessageOverheadTokens: messageOverheadTokens,
	}, sample)
	if err != nil {
		t.Fatalf("runSummaryStrategySweep() error = %v", err)
	}
	return result.ThresholdResults[0]
}

func oneTurnTokenStrategySample() SummarySample {
	return tokenStrategySample(1)
}

func twoTurnTokenStrategySample() SummarySample {
	return tokenStrategySample(2)
}

func threeTurnTokenStrategySample() SummarySample {
	return tokenStrategySample(3)
}

func tokenStrategySample(turns int) SummarySample {
	messages := make([]SummaryMessage, 0, turns*2)
	for turn := 1; turn <= turns; turn++ {
		messages = append(messages,
			SummaryMessage{Role: "user", Content: fmt.Sprintf("q%d", turn)},
			SummaryMessage{Role: "assistant", Content: fmt.Sprintf("a%d", turn)},
		)
	}
	return SummarySample{
		Name:  "token-strategy",
		Input: SummaryInput{SourceMessages: messages},
		StrategyEval: &SummaryStrategyEval{
			Checkpoints: []SummaryStrategyCheckpoint{{
				AfterTurn:       turns,
				ExpectedSummary: SummaryExpectedSummary{},
			}},
		},
	}
}
