package system

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

func nextDecisionFromHint(hintCall *ragcore.HintCall, done bool, reason string) ragcore.NextDecision {
	decision := ragcore.NextDecision{Done: done, Reason: reason, Terminal: done}
	if hintCall != nil {
		decision.HintCalls = []ragcore.HintCall{*hintCall}
		decision.Done = false
		decision.Terminal = false
	}
	return decision
}

func hintCallToSlice(hintCall *ragcore.HintCall) []ragcore.HintCall {
	if hintCall == nil {
		return nil
	}
	return []ragcore.HintCall{*hintCall}
}

func renderDiagnosisContext(result ragcore.Result) string {
	view, ok := ViewDiagnosisResult(result)
	if !ok {
		return ""
	}
	lines := make([]string, 0, 4)
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Conclusion: "+conclusion)
	}
	if confidence := strings.TrimSpace(view.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if len(view.Facts) > 0 {
		lines = append(lines, "Facts:\n- "+strings.Join(view.Facts, "\n- "))
	}
	if len(view.NextActions) > 0 {
		lines = append(lines, "Suggested next actions:\n- "+strings.Join(view.NextActions, "\n- "))
	}
	return strings.Join(lines, "\n")
}
