package handoff

import (
	"strings"

	agentruntime "local/rag-project/internal/app/agent/runtime"
)

func BuildAnswerGuidance(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	switch {
	case strings.TrimSpace(session.Snapshot.Answer.DegradeReason) != "":
		return "Answer conservatively. State that external evidence gathering was incomplete, mention the degrade reason, and avoid claims that are not supported by the evidence bundle."
	case session.Snapshot.Evidence.Sufficient && len(session.Snapshot.Evidence.Items) > 0:
		return "Lead with the answer, then support it with the accepted external evidence. Keep the answer grounded in the evidence bundle and avoid unsupported extrapolation."
	case len(session.Snapshot.Evidence.Items) > 0:
		return "Use the available evidence cautiously. Make uncertainty explicit and note any open questions that remain unresolved."
	default:
		return "No accepted external evidence was gathered. If you answer, clearly signal the lack of external support."
	}
}
