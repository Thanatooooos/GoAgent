package runtime

import (
	"encoding/json"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
)

type llmObserverResponse struct {
	Done      bool                  `json:"done"`
	Reasoning string                `json:"reasoning"`
	State     llmObserverStateBlock `json:"state"`
}

type llmObserverStateBlock struct {
	Phase         string     `json:"phase"`
	Hypothesis    string     `json:"hypothesis"`
	Confidence    float64    `json:"confidence"`
	OpenQuestions []string   `json:"openQuestions"`
	CheckedTools  []string   `json:"checkedTools"`
	NextHintCalls []HintCall `json:"nextHintCalls"`
	NextHint      string     `json:"nextHint"`
}

func (o *LLMObserver) parseResponse(raw string, input ObserveInput) (ObserveResult, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ObserveResult{}, false
	}
	if extracted := extractObserverJSONBlock(raw); extracted != "" {
		raw = extracted
	}

	var parsed llmObserverResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ObserveResult{}, false
	}

	state := AgentState{
		Phase:         parsed.State.Phase,
		Hypothesis:    parsed.State.Hypothesis,
		Confidence:    parsed.State.Confidence,
		OpenQuestions: parsed.State.OpenQuestions,
		CheckedTools:  parsed.State.CheckedTools,
		NextHintCalls: firstNonEmptyHintCalls(parsed.State.NextHintCalls),
	}.Normalize()

	state.CheckedTools = mergeCheckedTools(input.PreviousState.CheckedTools, state.CheckedTools, toolNames(input.RoundResults))
	if strings.TrimSpace(state.Hypothesis) == "" {
		state.Hypothesis = strings.TrimSpace(input.PreviousState.Hypothesis)
	}
	if !validateHintAgainstEvidence(state.NextHintCalls, input) {
		return ObserveResult{}, false
	}
	if parsed.Done {
		state.NextHintCalls = nil
		state.NextHint = ""
		if state.Phase == "" {
			state.Phase = "complete"
		}
	} else {
		if len(state.NextHintCalls) == 0 {
			return ObserveResult{}, false
		}
		for _, hintCall := range state.NextHintCalls {
			if strings.TrimSpace(hintCall.Name) == "" {
				return ObserveResult{}, false
			}
		}
		if state.Phase == "" {
			state.Phase = "deep_dive"
		}
		if len(state.OpenQuestions) == 0 {
			state.OpenQuestions = append([]string(nil), input.PreviousState.OpenQuestions...)
		}
	}
	decision := ObserveResult{
		Done:      parsed.Done,
		Reasoning: strings.TrimSpace(parsed.Reasoning),
		State:     state,
	}
	decision.State = normalizeObservationState(decision, input.PreviousState)
	return decision, true
}

func extractObserverJSONBlock(raw string) string {
	marker := "```json"
	start := strings.Index(raw, marker)
	if start == -1 {
		marker = "```"
		start = strings.Index(raw, marker)
	}
	if start == -1 {
		return ""
	}
	contentStart := strings.IndexByte(raw[start:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += start + 1
	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}
