package tool

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	documentIDPattern = regexp.MustCompile(`(?i)\b(doc(?:ument)?[-_a-z0-9]+)\b`)
	taskIDPattern     = regexp.MustCompile(`(?i)\b(task[-_a-z0-9]+)\b`)
	traceIDPattern    = regexp.MustCompile(`(?i)\b(trace[-_a-z0-9]+)\b`)
)

type LocalWorkflow struct {
	executor *Executor
	planner  Planner
	maxCalls int
}

func NewLocalWorkflow(executor *Executor) *LocalWorkflow {
	return &LocalWorkflow{
		executor: executor,
		maxCalls: 3,
	}
}

func (w *LocalWorkflow) SetMaxCalls(maxCalls int) {
	if w == nil {
		return
	}
	if maxCalls <= 0 {
		w.maxCalls = 1
		return
	}
	w.maxCalls = maxCalls
}

func (w *LocalWorkflow) SetPlanner(planner Planner) {
	if w == nil {
		return
	}
	w.planner = planner
}

func (w *LocalWorkflow) Run(ctx context.Context, input WorkflowInput) (WorkflowResult, error) {
	if w == nil || w.executor == nil {
		return WorkflowResult{}, fmt.Errorf("local tool workflow executor is required")
	}

	plannedCalls := w.planCalls(ctx, input)
	if len(plannedCalls) == 0 {
		return WorkflowResult{}, nil
	}

	results := make([]Result, 0, len(plannedCalls))
	callSummaries := make([]CallSummary, 0, len(plannedCalls))
	var degradeReasons []string
	for _, call := range plannedCalls {
		startedAt := time.Now()
		result, err := w.executor.Execute(ctx, call)
		durationMs := time.Since(startedAt).Milliseconds()
		results = append(results, result)
		callSummaries = append(callSummaries, CallSummary{
			Name:       strings.TrimSpace(result.Name),
			Status:     strings.TrimSpace(result.Status),
			Summary:    strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage)),
			DurationMs: durationMs,
		})
		if err != nil {
			degradeReasons = append(degradeReasons, strings.TrimSpace(firstNonEmpty(result.ErrorMessage, err.Error())))
		}
	}

	workflowResult := WorkflowResult{
		Used:           len(results) > 0,
		Context:        RenderContext(results),
		AnswerGuidance: BuildAnswerGuidance(results),
		Calls:          callSummaries,
	}
	if len(degradeReasons) > 0 {
		workflowResult.Degraded = true
		workflowResult.DegradeReason = strings.Join(degradeReasons, "; ")
	}
	return workflowResult, nil
}

func (w *LocalWorkflow) planCalls(ctx context.Context, input WorkflowInput) []Call {
	if w.planner != nil {
		calls := w.planWithLLM(ctx, input)
		if len(calls) > 0 {
			return calls
		}
	}
	return w.planWithRules(input)
}

func (w *LocalWorkflow) planWithLLM(ctx context.Context, input WorkflowInput) []Call {
	planInput := PlanInput{
		Question:        strings.TrimSpace(input.Question),
		ToolDefinitions: w.executor.registry.ListDefinitions(),
	}
	if planInput.Question == "" || len(planInput.ToolDefinitions) == 0 {
		return nil
	}

	result, err := w.planner.Plan(ctx, planInput)
	if err != nil || !result.HasTools() {
		return nil
	}
	if len(result.Calls) > w.maxCalls {
		result.Calls = result.Calls[:w.maxCalls]
	}
	return result.Calls
}

func (w *LocalWorkflow) planWithRules(input WorkflowInput) []Call {
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return nil
	}
	lowered := strings.ToLower(question)
	calls := make([]Call, 0, w.maxCalls)
	seen := map[string]struct{}{}

	appendCall := func(call Call) {
		if len(calls) >= w.maxCalls {
			return
		}
		key := callKey(call)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		calls = append(calls, call)
	}

	if documentID := firstMatchedID(documentIDPattern, question); documentID != "" && containsAny(lowered, "document", "doc", "文档") {
		if containsAny(lowered, "diagnose", "failure", "why", "failed", "排查", "诊断", "失败", "原因") {
			appendCall(Call{
				Name: "document_ingestion_diagnose",
				Arguments: map[string]any{
					"documentId": documentID,
				},
			})
		}
		if containsAny(lowered, "chunk log", "chunklog", "chunk", "ingestion", "pipeline", "diagnose", "failure", "排查", "诊断", "失败") {
			appendCall(Call{
				Name: "document_chunk_log_query",
				Arguments: map[string]any{
					"documentId": documentID,
				},
			})
		}
		appendCall(Call{
			Name: "document_query",
			Arguments: map[string]any{
				"documentId": documentID,
			},
		})
	}

	if taskID := firstMatchedID(taskIDPattern, question); taskID != "" && containsAny(lowered, "ingestion", "task", "任务", "导入") {
		if containsAny(lowered, "diagnose", "failure", "why", "failed", "排查", "诊断", "失败", "原因") {
			appendCall(Call{
				Name: "task_ingestion_diagnose",
				Arguments: map[string]any{
					"taskId": taskID,
				},
			})
		}
		if containsAny(lowered, "node", "节点", "步骤") {
			appendCall(Call{
				Name: "ingestion_task_node_query",
				Arguments: map[string]any{
					"taskId": taskID,
				},
			})
		}
		appendCall(Call{
			Name: "ingestion_task_query",
			Arguments: map[string]any{
				"taskId":       taskID,
				"includeNodes": true,
			},
		})
	}

	traceID := firstMatchedID(traceIDPattern, question)
	if traceID == "" && containsAny(lowered, "本次", "当前", "this", "current") && containsAny(lowered, "trace", "chain", "retrieval", "链路", "检索", "召回") {
		traceID = strings.TrimSpace(input.TraceID)
	}
	if traceID != "" && containsAny(lowered, "trace", "chain", "retrieval", "链路", "检索", "召回") {
		if containsAny(lowered, "diagnose", "failure", "why", "bad", "poor", "failed", "排查", "诊断", "失败", "原因", "效果差", "召回差") {
			appendCall(Call{
				Name: "trace_retrieval_diagnose",
				Arguments: map[string]any{
					"traceId": traceID,
				},
			})
		}
		appendCall(Call{
			Name: "trace_node_query",
			Arguments: map[string]any{
				"traceId": traceID,
			},
		})
	}

	return calls
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, strings.ToLower(strings.TrimSpace(value))) {
			return true
		}
	}
	return false
}

func firstMatchedID(pattern *regexp.Regexp, text string) string {
	if pattern == nil {
		return ""
	}
	match := pattern.FindStringSubmatch(strings.TrimSpace(text))
	if len(match) < 2 {
		return ""
	}
	return strings.TrimSpace(match[1])
}

func callKey(call Call) string {
	var builder strings.Builder
	builder.WriteString(strings.TrimSpace(call.Name))
	builder.WriteString("|")
	if len(call.Arguments) == 0 {
		return builder.String()
	}
	for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
		value := strings.TrimSpace(readStringArg(call.Arguments, key))
		if value == "" {
			continue
		}
		builder.WriteString(key)
		builder.WriteString("=")
		builder.WriteString(value)
		builder.WriteString("|")
	}
	return builder.String()
}
