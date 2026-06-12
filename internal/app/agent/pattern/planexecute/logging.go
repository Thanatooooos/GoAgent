package planexecute

import (
	"encoding/json"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"
	"local/rag-project/internal/framework/log"
)

const (
	colorReset   = "\x1b[0m"
	colorBold    = "\x1b[1m"
	colorBlue    = "\x1b[34m"
	colorCyan    = "\x1b[36m"
	colorGreen   = "\x1b[32m"
	colorYellow  = "\x1b[33m"
	colorMagenta = "\x1b[35m"
	colorRed     = "\x1b[31m"
)

func logPlanBuilt(session *agentruntime.RuntimeSession, plan agentstate.PlanState, reasoning string) {
	log.Infof(
		"%s traceID=%s sessionID=%s goal=%s reasoning=%s%s",
		colorLabel(colorCyan, "PLAN"),
		sessionTraceID(session),
		sessionID(session),
		compactText(plan.Goal, 160),
		compactText(reasoning, 160),
		renderPlanSteps(plan.Steps),
	)
}

func logStepSelected(session *agentruntime.RuntimeSession, step agentstate.PlanStep, branch string, reason string) {
	log.Infof(
		"%s traceID=%s sessionID=%s step=%s title=%s branch=%s reason=%s query=%s urls=%s dependsOn=%s",
		colorLabel(colorBlue, "STEP"),
		sessionTraceID(session),
		sessionID(session),
		step.StepID,
		compactText(step.Title, 120),
		branch,
		compactText(reason, 120),
		compactText(step.Query, 120),
		renderStringSlice(step.URLs, 3),
		renderStringSlice(step.DependsOn, 4),
	)
}

func logStepExecutionStart(session *agentruntime.RuntimeSession, step agentstate.PlanStep, spec agentcapability.Spec, input any) {
	log.Infof(
		"%s traceID=%s sessionID=%s step=%s capability=%s kind=%s family=%s approval=%t input=%s",
		colorLabel(colorMagenta, "TOOL"),
		sessionTraceID(session),
		sessionID(session),
		step.StepID,
		firstNonEmpty(spec.Name, step.CapabilityName),
		firstNonEmpty(spec.Kind, step.CapabilityKind),
		firstNonEmpty(spec.Family, step.CapabilityFamily),
		spec.RequiresApproval || step.RequiresApproval,
		compactJSON(input, 320),
	)
}

func logStepExecutionResult(session *agentruntime.RuntimeSession, step agentstate.PlanStep, result agentcapability.InvocationResult, resultState agentstate.PlanStepResult) {
	labelColor := colorGreen
	if result.Status != agentcapability.StatusSucceeded {
		labelColor = colorYellow
	}
	if strings.TrimSpace(resultState.ErrorClass) != "" {
		labelColor = colorRed
	}
	log.Infof(
		"%s traceID=%s sessionID=%s step=%s capability=%s status=%s errorClass=%s durationMs=%d summary=%s artifacts=%s urls=%s output=%s",
		colorLabel(labelColor, "RESULT"),
		sessionTraceID(session),
		sessionID(session),
		step.StepID,
		step.CapabilityName,
		firstNonEmpty(result.Status, resultState.Status),
		firstNonEmpty(result.ErrorClass, resultState.ErrorClass),
		resultState.DurationMs,
		compactText(firstNonEmpty(result.Observation.Summary, result.Action.Summary, resultState.Summary), 180),
		renderArtifacts(resultState.Artifacts),
		renderStringSlice(resultState.URLs, 3),
		compactJSON(result.Output, 320),
	)
}

func logStepAssessment(session *agentruntime.RuntimeSession, step agentstate.PlanStep, branch string, reason string, sufficient bool, evidenceItems []agentstate.EvidenceItem) {
	log.Infof(
		"%s traceID=%s sessionID=%s step=%s branch=%s reason=%s sufficient=%t newEvidence=%d evidence=%s",
		colorLabel(colorYellow, "ASSESS"),
		sessionTraceID(session),
		sessionID(session),
		step.StepID,
		branch,
		compactText(reason, 140),
		sufficient,
		len(evidenceItems),
		renderEvidenceItems(evidenceItems),
	)
}

func logFinalizedOutput(session *agentruntime.RuntimeSession, mode string, final string, degradeReason string) {
	labelColor := colorGreen
	if strings.TrimSpace(degradeReason) != "" {
		labelColor = colorRed
	}
	log.Infof(
		"%s traceID=%s sessionID=%s mode=%s degraded=%t reason=%s output=%s",
		colorLabel(labelColor, "FINAL"),
		sessionTraceID(session),
		sessionID(session),
		mode,
		strings.TrimSpace(degradeReason) != "",
		compactText(degradeReason, 160),
		compactText(final, 320),
	)
}

func colorLabel(color string, label string) string {
	return fmt.Sprintf("%s%s[%s]%s", colorBold, color, strings.TrimSpace(label), colorReset)
}

func renderPlanSteps(steps []agentstate.PlanStep) string {
	if len(steps) == 0 {
		return " steps=[]"
	}
	parts := make([]string, 0, len(steps))
	for _, step := range steps {
		parts = append(parts, fmt.Sprintf(
			"{id=%s capability=%s goal=%s policy=%s query=%s urls=%s}",
			step.StepID,
			firstNonEmpty(step.CapabilityName, step.CapabilityRole),
			compactText(step.Goal, 80),
			firstNonEmpty(step.CompletionPolicy, step.FailurePolicy),
			compactText(step.Query, 80),
			renderStringSlice(step.URLs, 2),
		))
	}
	return " steps=" + strings.Join(parts, " -> ")
}

func renderArtifacts(artifacts []agentstate.PlanStepArtifact) string {
	if len(artifacts) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		parts = append(parts, fmt.Sprintf(
			"{kind=%s summary=%s values=%s refs=%s}",
			artifact.Kind,
			compactText(artifact.Summary, 80),
			renderStringSlice(artifact.StringValues, 2),
			renderStringSlice(artifact.Refs, 2),
		))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func renderEvidenceItems(items []agentstate.EvidenceItem) string {
	if len(items) == 0 {
		return "[]"
	}
	parts := make([]string, 0, len(items))
	for _, item := range items {
		parts = append(parts, fmt.Sprintf(
			"{source=%s ref=%s content=%s}",
			item.Source,
			compactText(item.SourceRef, 60),
			compactText(item.Content, 80),
		))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

func renderStringSlice(values []string, limit int) string {
	if len(values) == 0 {
		return "[]"
	}
	if limit <= 0 || len(values) <= limit {
		return "[" + strings.Join(trimmedValues(values), ", ") + "]"
	}
	trimmed := trimmedValues(values[:limit])
	return fmt.Sprintf("[%s, +%d more]", strings.Join(trimmed, ", "), len(values)-limit)
}

func trimmedValues(values []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := compactText(value, 80)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{""}
	}
	return result
}

func compactJSON(value any, limit int) string {
	if value == nil {
		return "{}"
	}
	data, err := json.Marshal(value)
	if err != nil {
		return compactText(fmt.Sprintf("%v", value), limit)
	}
	return compactText(string(data), limit)
}

func compactText(value string, limit int) string {
	trimmed := strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if trimmed == "" {
		return ""
	}
	if limit > 0 && len([]rune(trimmed)) > limit {
		runes := []rune(trimmed)
		return strings.TrimSpace(string(runes[:limit-3])) + "..."
	}
	return trimmed
}

func sessionTraceID(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.Request.TraceID)
}

func sessionID(session *agentruntime.RuntimeSession) string {
	if session == nil {
		return ""
	}
	return strings.TrimSpace(session.SessionID)
}
