package runtime

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

const DefaultMaxIterations = 3

// AgentLoop executes a multi-round Plan -> Act -> Observe tool loop.
type AgentLoop struct {
	executor             *Executor
	planner              Planner
	observer             Observer
	maxIterations        int
	parallelToolCalls    bool
	maxParallelToolCalls int
}

func NewAgentLoop(executor *Executor) *AgentLoop {
	return &AgentLoop{
		executor:             executor,
		observer:             NewRuleObserver(),
		maxIterations:        DefaultMaxIterations,
		maxParallelToolCalls: 1,
	}
}

func (w *AgentLoop) SetPlanner(planner Planner) {
	if w == nil {
		return
	}
	w.planner = planner
}

func (w *AgentLoop) SetObserver(observer Observer) {
	if w == nil || observer == nil {
		return
	}
	w.observer = observer
}

func (w *AgentLoop) SetMaxIterations(maxIterations int) {
	if w == nil {
		return
	}
	if maxIterations <= 0 {
		w.maxIterations = 1
		return
	}
	w.maxIterations = maxIterations
}

func (w *AgentLoop) SetParallelToolCalls(enabled bool, maxConcurrency int) {
	if w == nil {
		return
	}
	w.parallelToolCalls = enabled
	if maxConcurrency <= 1 {
		w.maxParallelToolCalls = 1
		return
	}
	w.maxParallelToolCalls = maxConcurrency
}

func (w *AgentLoop) Run(ctx context.Context, input WorkflowInput) (WorkflowResult, error) {
	if w == nil || w.executor == nil {
		return WorkflowResult{}, fmt.Errorf("agent loop executor is required")
	}

	log.Infof("[agent] start: question=%q maxRounds=%d parallel=%v", TruncateForLog(input.Question), w.maxIterations, w.parallelToolCalls)

	allResults := make([]Result, 0)
	allCalls := make([]CallSummary, 0)
	rounds := make([]RoundSummary, 0, w.maxIterations)
	degradeReasons := make([]string, 0)
	executed := map[string]struct{}{}
	agentState := AgentState{}

	for round := 1; round <= w.maxIterations; round++ {
		plannedCalls := w.planCalls(ctx, input, agentState, allResults, executed)
		if len(plannedCalls) == 0 {
			if round > 1 && !agentState.Empty() {
				log.Infof("[agent] round %d: planner produced no new calls, stopping", round)
				rounds = append(rounds, RoundSummary{
					Round:         round,
					Done:          true,
					Reasoning:     "planning produced no new tool calls, so the agent loop stops here.",
					NextHintCalls: append([]HintCall(nil), agentState.NextHintCalls...),
					NextHint:      agentState.NextHint,
					Confidence:    agentState.Confidence,
					State:         agentState.Normalize(),
				})
			}
			break
		}

		toolNames := make([]string, len(plannedCalls))
		for i, c := range plannedCalls {
			toolNames[i] = strings.TrimSpace(c.Name)
		}
		mode := w.roundExecutionMode(len(plannedCalls))
		log.Infof("[agent] round %d: %d call(s) [%s] (%s)", round, len(plannedCalls), strings.Join(toolNames, ", "), mode)

		roundStartedAt := time.Now()
		roundResults, roundCalls, roundDegradeReasons := w.executeRoundCalls(ctx, input.EventSink, round, plannedCalls)
		roundWallClockDurationMs := time.Since(roundStartedAt).Milliseconds()
		allResults = append(allResults, roundResults...)
		allCalls = append(allCalls, roundCalls...)
		degradeReasons = append(degradeReasons, roundDegradeReasons...)
		for _, call := range plannedCalls {
			executed[callKey(call)] = struct{}{}
		}

		observeInput := ObserveInput{
			Question:         strings.TrimSpace(input.Question),
			Round:            round,
			Results:          append([]Result(nil), allResults...),
			RoundResults:     append([]Result(nil), roundResults...),
			PreviousState:    agentState.Normalize(),
			MaxIterations:    w.maxIterations,
			ToolDefinitions:  w.executor.registry.ListDefinitions(),
			ToolRegistry:     w.executor.registry,
			KnowledgeBaseIDs: append([]string(nil), input.KnowledgeBaseIDs...),
			RewriteResult:    input.RewriteResult,
			RetrieveResult:   input.RetrieveResult,
		}
		if round == w.maxIterations {
			observeInput.ReachedMaxLoop = true
		}

		observation := ObserveResult{Done: true}
		if w.observer != nil {
			obs, err := w.observer.Observe(ctx, observeInput)
			if err != nil {
				degradeReasons = append(degradeReasons, err.Error())
				observation = ObserveResult{
					Done:      true,
					Reasoning: "observe phase failed, so the agent loop stops with the current evidence.",
					State: AgentState{
						Phase:         "complete",
						Hypothesis:    agentState.Hypothesis,
						Confidence:    agentState.Confidence,
						OpenQuestions: append([]string(nil), agentState.OpenQuestions...),
						CheckedTools:  append([]string(nil), agentState.CheckedTools...),
						NextHintCalls: append([]HintCall(nil), agentState.NextHintCalls...),
						NextHint:      agentState.NextHint,
					}.Normalize(),
				}
			} else {
				observation = obs
			}
		}

		if observation.State.Empty() {
			observation.State = AgentState{
				Confidence:    observation.Confidence,
				NextHintCalls: append([]HintCall(nil), observation.NextHintCalls...),
				NextHint:      observation.NextHint,
			}.Normalize()
		}
		if len(observation.NextHintCalls) == 0 && strings.TrimSpace(observation.NextHint) == "" {
			observation.NextHintCalls = append([]HintCall(nil), observation.State.NextHintCalls...)
		}
		if strings.TrimSpace(observation.NextHint) == "" {
			observation.NextHint = observation.State.NextHint
		}
		observation.Confidence = clampConfidence(firstNonZeroFloat(observation.Confidence, observation.State.Confidence))
		observation.State.Confidence = observation.Confidence
		if len(observation.State.NextHintCalls) == 0 && len(observation.NextHintCalls) > 0 {
			observation.State.NextHintCalls = append([]HintCall(nil), observation.NextHintCalls...)
		}
		observation.State.NextHint = strings.TrimSpace(firstNonEmpty(observation.State.NextHint, observation.NextHint))
		observation.State = observation.State.Normalize()
		observation.NextHintCalls = append([]HintCall(nil), observation.State.NextHintCalls...)
		observation.NextHint = observation.State.NextHint

		totalToolDurationMs := int64(0)
		for _, call := range roundCalls {
			totalToolDurationMs += maxInt64(call.DurationMs, 0)
		}

		roundSummary := RoundSummary{
			Round:               round,
			Calls:               roundCalls,
			Done:                observation.Done,
			Reasoning:           strings.TrimSpace(observation.Reasoning),
			NextHintCalls:       append([]HintCall(nil), observation.NextHintCalls...),
			NextHint:            strings.TrimSpace(observation.NextHint),
			Confidence:          observation.Confidence,
			State:               observation.State,
			ExecutionMode:       w.roundExecutionMode(len(plannedCalls)),
			WallClockDurationMs: roundWallClockDurationMs,
			ToolCallCount:       len(roundCalls),
			TotalToolDurationMs: totalToolDurationMs,
		}
		rounds = append(rounds, roundSummary)

		if observation.Done {
			log.Infof("[agent] round %d observer: DONE (confidence=%.2f, wallClock=%dms)", round, observation.Confidence, roundWallClockDurationMs)
		} else {
			nextNames := make([]string, len(observation.NextHintCalls))
			for i, h := range observation.NextHintCalls {
				nextNames[i] = strings.TrimSpace(h.Name)
			}
			log.Infof("[agent] round %d observer: CONTINUE (confidence=%.2f) -> hint: %s", round, observation.Confidence, strings.Join(nextNames, ", "))
		}

		if !observation.Done && strings.TrimSpace(observation.Reasoning) != "" && input.EventSink != nil {
			_ = input.EventSink.OnAgentThink(observation.Reasoning)
		}
		if observation.Done {
			break
		}
		agentState = observation.State
	}

	workflowResult := WorkflowResult{
		Used:           len(allResults) > 0,
		Context:        RenderContextWithRegistry(w.executor.registry, allResults),
		AnswerGuidance: BuildAnswerGuidanceWithRegistry(w.executor.registry, allResults),
		Control:        deriveWorkflowControl(input, allResults),
		Calls:          allCalls,
		Rounds:         rounds,
	}
	workflowResult.TraceMeta = buildWorkflowTraceMeta(workflowResult.Control, input.RetrieveResult, allResults)
	if len(degradeReasons) > 0 {
		workflowResult.Degraded = true
		workflowResult.DegradeReason = strings.Join(uniqueStrings(degradeReasons), "; ")
	}
	if len(rounds) == w.maxIterations && len(rounds) > 0 && !rounds[len(rounds)-1].Done {
		workflowResult.Degraded = true
		workflowResult.DegradeReason = strings.TrimSpace(firstNonEmpty(workflowResult.DegradeReason, "agent loop reached max iterations"))
	}
	log.Infof("[agent] done: %d round(s), %d call(s), degraded=%v", len(rounds), len(allCalls), workflowResult.Degraded)
	return workflowResult, nil
}

func (w *AgentLoop) planCalls(ctx context.Context, input WorkflowInput, agentState AgentState, previousResults []Result, executed map[string]struct{}) []Call {
	calls := []Call{}
	if w.planner != nil {
		calls = w.planWithLLM(ctx, input, agentState, previousResults, executed)
	}
	if len(calls) == 0 {
		calls = w.planWithRules(input, agentState, previousResults, executed)
	}
	return calls
}

func (w *AgentLoop) planWithLLM(ctx context.Context, input WorkflowInput, agentState AgentState, previousResults []Result, executed map[string]struct{}) []Call {
	planInput := PlanInput{
		Question:         strings.TrimSpace(input.Question),
		ToolDefinitions:  w.executor.registry.ListDefinitions(),
		AgentState:       agentState.Normalize(),
		PreviousResults:  append([]Result(nil), previousResults...),
		KnowledgeBaseIDs: append([]string(nil), input.KnowledgeBaseIDs...),
		RewriteResult:    input.RewriteResult,
		RetrieveResult:   input.RetrieveResult,
	}
	if planInput.Question == "" || len(planInput.ToolDefinitions) == 0 {
		return nil
	}

	result, err := w.planner.Plan(ctx, planInput)
	if err != nil || !result.HasTools() {
		return nil
	}

	available := make([]Call, 0, len(result.Calls))
	for _, call := range result.Calls {
		if !validateCallAgainstEvidence(call, planInput.Question, planInput.AgentState.NextHintCalls, previousResults) {
			continue
		}
		key := callKey(call)
		if _, exists := executed[key]; exists {
			continue
		}
		available = append(available, call)
	}
	return available
}

func (w *AgentLoop) planWithRules(input WorkflowInput, agentState AgentState, previousResults []Result, executed map[string]struct{}) []Call {
	if agentState.Empty() || len(agentState.NextHintCalls) == 0 {
		return filterNewCalls(PlanWithBaseRules(input, DefaultMaxIterations), executed)
	}
	hintCalls := PlanCallsFromHintCalls(agentState.NextHintCalls, w.executor.registry.ListDefinitions())
	if len(hintCalls) > 0 {
		return filterNewCalls(hintCalls, executed)
	}
	return filterNewCalls(planCallsFromResultsWithRegistry(previousResults, input, w.executor.registry), executed)
}

func PlanWithBaseRules(input WorkflowInput, maxCalls int) []Call {
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return nil
	}
	lowered := strings.ToLower(question)
	calls := make([]Call, 0, maxCalls)
	seen := map[string]struct{}{}

	appendCall := func(call Call) {
		if len(calls) >= maxCalls {
			return
		}
		key := callKey(call)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		calls = append(calls, call)
	}

	appendDocumentCalls(lowered, question, &calls, appendCall)
	appendTaskCalls(lowered, question, &calls, appendCall)
	appendTraceCalls(lowered, question, input.TraceID, &calls, appendCall)

	if len(calls) == 0 {
		appendOpenEndedCalls(lowered, input, &calls, appendCall)
	}

	if len(calls) == 0 && KnowledgeBaseInsufficient(input.RetrieveResult) {
		log.Infof("[agent] kb insufficient (chunks=%d), triggering external_evidence_workflow for %q", len(input.RetrieveResult.Chunks), TruncateForLog(question))
		appendCall(Call{Name: "external_evidence_workflow", Arguments: map[string]any{"question": question}})
	}

	return calls
}

// baseRouteRule defines a keyword-driven routing rule. Rules in a family are
// checked in order; the first match wins.
type baseRouteRule struct {
	requireAll [][]string // at least one keyword from each inner slice must match
	exclude    []string   // none of these must match
	buildCall  func(id string) Call
}

var (
	diagnosisKeywords = []string{
		"diagnose", "failure", "why", "failed", "排查", "诊断", "失败", "原因",
		"running", "processing", "progress", "slow", "stuck", "node", "status",
		"运行", "处理中", "进度", "慢", "卡", "节点", "状态", "还在", "完成",
	}
	solutionKeywords = []string{
		"解决", "怎么办", "修复", "方案", "办法", "如何处理", "怎么修复",
		"solution", "fix", "how to fix", "resolve", "troubleshoot",
	}
	chunkLogKeywords = []string{
		"chunk log", "chunklog", "chunk", "ingestion", "pipeline",
		"diagnose", "failure", "排查", "诊断", "失败",
	}
	docKeywords  = []string{"document", "doc", "文档"}
	taskKeywords = []string{"ingestion", "task", "任务", "导入任务"}
	traceKeywords = []string{"trace", "chain", "retrieval", "链路", "检索", "召回"}
	traceDiagnosisKeywords = []string{
		"diagnose", "failure", "why", "bad", "poor", "failed",
		"排查", "诊断", "失败", "原因", "效果差", "召回差",
	}
)

var documentBaseRules = []baseRouteRule{
	{
		requireAll: [][]string{docKeywords, diagnosisKeywords, solutionKeywords},
		buildCall:  func(id string) Call { return Call{Name: "document_diagnose_with_search", Arguments: map[string]any{"documentId": id}} },
	},
	{
		requireAll: [][]string{docKeywords, diagnosisKeywords},
		exclude:    solutionKeywords,
		buildCall:  func(id string) Call { return Call{Name: "document_root_cause_diagnosis", Arguments: map[string]any{"documentId": id}} },
	},
	{
		requireAll: [][]string{docKeywords, chunkLogKeywords},
		exclude:    diagnosisKeywords,
		buildCall:  func(id string) Call { return Call{Name: "document_chunk_log_query", Arguments: map[string]any{"documentId": id}} },
	},
	{
		requireAll: [][]string{docKeywords},
		buildCall:  func(id string) Call { return Call{Name: "document_query", Arguments: map[string]any{"documentId": id}} },
	},
}

var taskBaseRules = []baseRouteRule{
	{
		requireAll: [][]string{taskKeywords, diagnosisKeywords},
		buildCall:  func(id string) Call { return Call{Name: "task_ingestion_diagnose", Arguments: map[string]any{"taskId": id}} },
	},
	{
		requireAll: [][]string{taskKeywords},
		buildCall:  func(id string) Call { return Call{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": id, "includeNodes": true}} },
	},
}

func applyFirstMatchingRule(lowered string, id string, rules []baseRouteRule) *Call {
	for _, rule := range rules {
		if !matchKeywordGroups(lowered, rule.requireAll) {
			continue
		}
		if len(rule.exclude) > 0 && containsAny(lowered, rule.exclude...) {
			continue
		}
		call := rule.buildCall(id)
		return &call
	}
	return nil
}

func matchKeywordGroups(text string, groups [][]string) bool {
	for _, group := range groups {
		if !containsAny(text, group...) {
			return false
		}
	}
	return true
}

func appendDocumentCalls(lowered, question string, calls *[]Call, appendCall func(Call)) {
	id := firstMatchedID(documentIDPattern, question)
	if id == "" || !containsAny(lowered, docKeywords...) {
		return
	}
	call := applyFirstMatchingRule(lowered, id, documentBaseRules)
	if call != nil {
		appendCall(*call)
	}
}

func appendTaskCalls(lowered, question string, calls *[]Call, appendCall func(Call)) {
	id := firstMatchedID(taskIDPattern, question)
	if id == "" || !containsAny(lowered, taskKeywords...) {
		return
	}
	call := applyFirstMatchingRule(lowered, id, taskBaseRules)
	if call != nil {
		appendCall(*call)
	}
}

func appendTraceCalls(lowered, question, traceID string, calls *[]Call, appendCall func(Call)) {
	id := firstMatchedID(traceIDPattern, question)
	if id == "" && containsAny(lowered, "本次", "当前", "this", "current") && containsAny(lowered, traceKeywords...) {
		id = strings.TrimSpace(traceID)
	}
	if id == "" || !containsAny(lowered, traceKeywords...) {
		return
	}
	if containsAny(lowered, traceDiagnosisKeywords...) {
		appendCall(Call{Name: "trace_retrieval_diagnose", Arguments: map[string]any{"traceId": id}})
	}
	appendCall(Call{Name: "trace_node_query", Arguments: map[string]any{"traceId": id}})
}

func appendOpenEndedCalls(lowered string, input WorkflowInput, calls *[]Call, appendCall func(Call)) {
	isOpenEnded := containsAny(lowered,
		"哪些", "最近", "所有", "列表", "哪个", "哪个文档", "哪些文档",
		"which", "list", "recent", "all", "any",
		"失败", "运行中", "处理中",
	)
	if !isOpenEnded {
		return
	}
	defaultKB := ""
	if len(input.KnowledgeBaseIDs) > 0 {
		defaultKB = strings.TrimSpace(input.KnowledgeBaseIDs[0])
	}
	if containsAny(lowered, docKeywords...) {
		callArgs := map[string]any{}
		if defaultKB != "" {
			callArgs["knowledgeBaseId"] = defaultKB
		}
		if containsAny(lowered, "失败", "failed") {
			callArgs["status"] = "failed"
		} else if containsAny(lowered, "运行", "处理中", "running") {
			callArgs["status"] = "running"
		}
		appendCall(Call{Name: "document_list", Arguments: callArgs})
	}
	if containsAny(lowered, taskKeywords...) {
		callArgs := map[string]any{}
		if containsAny(lowered, "失败", "failed") {
			callArgs["status"] = "failed"
		} else if containsAny(lowered, "运行", "处理中", "running") {
			callArgs["status"] = "running"
		}
		appendCall(Call{Name: "task_list", Arguments: callArgs})
	}
}

func PlanCallsFromHint(agentState string, defs []Definition) []Call {
	return PlanCallsFromHintCalls(parseHintCallsFromLegacyString(agentState), defs)
}

func PlanCallsFromHintCalls(hintCalls []HintCall, defs []Definition) []Call {
	hintCalls = normalizeHintCalls(hintCalls)
	if len(hintCalls) == 0 {
		return nil
	}
	calls := make([]Call, 0, len(hintCalls))
	for _, hintCall := range hintCalls {
		call, ok := buildCallFromHintCall(hintCall, defs)
		if !ok {
			continue
		}
		calls = append(calls, call)
	}
	if len(calls) == 0 {
		return nil
	}
	return calls
}

func buildCallFromHintCall(hintCall HintCall, defs []Definition) (Call, bool) {
	name := strings.TrimSpace(hintCall.Name)
	if name == "" {
		return Call{}, false
	}
	def, ok := findDefinitionByName(defs, name)
	if !ok {
		arguments := cloneMap(hintCall.Arguments)
		if len(arguments) == 0 {
			return Call{}, false
		}
		return Call{Name: name, Arguments: arguments}, true
	}

	callArgs := make(map[string]any, len(def.Parameters))
	for _, param := range def.Parameters {
		value, exists := hintCall.Arguments[param.Name]
		if !exists || value == nil {
			if param.Name == "includeNodes" && param.Type == ParamTypeBoolean {
				callArgs[param.Name] = true
				continue
			}
			if param.Required {
				return Call{}, false
			}
			continue
		}
		coerced, ok := coerceHintArgument(value, param.Type)
		if !ok {
			return Call{}, false
		}
		callArgs[param.Name] = coerced
	}
	if len(callArgs) == 0 {
		return Call{}, false
	}
	return Call{Name: name, Arguments: callArgs}, true
}

func findDefinitionByName(defs []Definition, name string) (Definition, bool) {
	name = strings.TrimSpace(name)
	for _, def := range defs {
		if strings.TrimSpace(def.Name) == name {
			return def, true
		}
	}
	return Definition{}, false
}

func coerceHintArgument(value any, paramType string) (any, bool) {
	switch strings.TrimSpace(paramType) {
	case "", ParamTypeString:
		switch typed := value.(type) {
		case string:
			typed = strings.TrimSpace(typed)
			return typed, typed != ""
		default:
			return "", false
		}
	case ParamTypeBoolean:
		switch typed := value.(type) {
		case bool:
			return typed, true
		case string:
			typed = strings.TrimSpace(typed)
			if typed == "" {
				return false, false
			}
			parsed, err := strconv.ParseBool(typed)
			if err != nil {
				return false, false
			}
			return parsed, true
		default:
			return false, false
		}
	case ParamTypeInteger:
		switch typed := value.(type) {
		case int:
			return typed, true
		case int32:
			return int(typed), true
		case int64:
			return int(typed), true
		case float64:
			return int(typed), true
		case string:
			typed = strings.TrimSpace(typed)
			if typed == "" {
				return 0, false
			}
			parsed, err := strconv.Atoi(typed)
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	case ParamTypeNumber:
		switch typed := value.(type) {
		case float64:
			return typed, true
		case float32:
			return float64(typed), true
		case int:
			return float64(typed), true
		case int32:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case string:
			typed = strings.TrimSpace(typed)
			if typed == "" {
				return 0, false
			}
			parsed, err := strconv.ParseFloat(typed, 64)
			if err != nil {
				return 0, false
			}
			return parsed, true
		default:
			return 0, false
		}
	case ParamTypeObject, ParamTypeArray:
		return value, true
	default:
		return value, true
	}
}

func PlanCallsFromResults(results []Result) []Call {
	if len(results) == 0 {
		return nil
	}
	latest := results[len(results)-1]
	hintCall, done, _ := nextAction(latest)
	if done || hintCall == nil || strings.TrimSpace(hintCall.Name) == "" {
		return nil
	}
	return []Call{{Name: strings.TrimSpace(hintCall.Name), Arguments: cloneMap(hintCall.Arguments)}}
}

func latestInterestingTaskNode(data map[string]any) (string, string, bool) {
	if len(data) == 0 {
		return "", "", false
	}
	raw, ok := data["taskNodeSummary"]
	if !ok || raw == nil {
		return "", "", false
	}

	readFromMap := func(item map[string]any) (string, string, bool) {
		nodeID := strings.TrimSpace(readStringArg(item, "nodeId"))
		status := strings.ToLower(strings.TrimSpace(readStringArg(item, "status")))
		if nodeID == "" {
			return "", "", false
		}
		if status == "failed" || status == "running" {
			return nodeID, status, true
		}
		return "", "", false
	}

	switch typed := raw.(type) {
	case []map[string]any:
		for _, item := range typed {
			if nodeID, status, ok := readFromMap(item); ok {
				return nodeID, status, true
			}
		}
	case []any:
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if nodeID, status, ok := readFromMap(mapped); ok {
				return nodeID, status, true
			}
		}
	}
	return "", "", false
}

func filterNewCalls(calls []Call, executed map[string]struct{}) []Call {
	if len(calls) == 0 {
		return nil
	}
	filtered := make([]Call, 0, len(calls))
	for _, call := range calls {
		if _, exists := executed[callKey(call)]; exists {
			continue
		}
		filtered = append(filtered, call)
	}
	return filtered
}

func cloneMap(input map[string]any) map[string]any {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]any, len(input))
	for key, value := range input {
		cloned[key] = value
	}
	return cloned
}

func uniqueStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func parseNextHint(hint string) (string, map[string]any) {
	hint = strings.TrimSpace(hint)
	if hint == "" || !strings.HasPrefix(hint, "tool:") {
		return "", nil
	}
	parts := strings.Split(hint, "|")
	if len(parts) == 0 {
		return "", nil
	}
	name := strings.TrimSpace(strings.TrimPrefix(parts[0], "tool:"))
	if name == "" {
		return "", nil
	}
	arguments := map[string]any{}
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		arguments[key] = value
	}
	return name, arguments
}

func firstNonZeroFloat(values ...float64) float64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func maxInt64(a int64, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

type roundExecutionItem struct {
	index       int
	call        Call
	callID      string
	result      Result
	callSummary CallSummary
	err         error
}

func (w *AgentLoop) executeRoundCalls(ctx context.Context, sink WorkflowEventSink, round int, plannedCalls []Call) ([]Result, []CallSummary, []string) {
	if len(plannedCalls) == 0 {
		return nil, nil, nil
	}

	items := make([]roundExecutionItem, len(plannedCalls))
	for idx, call := range plannedCalls {
		callID := fmt.Sprintf("round_%d_call_%02d", round, idx+1)
		items[idx] = roundExecutionItem{
			index:  idx,
			call:   call,
			callID: callID,
		}
		if sink != nil {
			_ = sink.OnToolStart(ToolCallEvent{
				CallID:    callID,
				Round:     round,
				Sequence:  idx + 1,
				Name:      strings.TrimSpace(call.Name),
				Status:    "running",
				Arguments: cloneMap(call.Arguments),
			})
		}
	}

	if !w.parallelToolCalls || len(plannedCalls) == 1 || w.maxParallelToolCalls <= 1 {
		for idx := range items {
			items[idx] = w.executeSingleCall(ctx, round, items[idx])
		}
	} else {
		w.executeCallsInParallel(ctx, round, items)
	}

	roundResults := make([]Result, 0, len(items))
	roundCalls := make([]CallSummary, 0, len(items))
	degradeReasons := make([]string, 0)
	for idx := range items {
		item := items[idx]
		roundResults = append(roundResults, item.result)
		roundCalls = append(roundCalls, item.callSummary)
		if item.err != nil {
			degradeReasons = append(degradeReasons, strings.TrimSpace(firstNonEmpty(item.result.ErrorMessage, item.err.Error())))
		}
		if sink != nil {
			_ = sink.OnToolResult(ToolCallEvent{
				CallID:     item.callID,
				Round:      round,
				Sequence:   idx + 1,
				Name:       item.callSummary.Name,
				Status:     item.callSummary.Status,
				Summary:    item.callSummary.Summary,
				DurationMs: item.callSummary.DurationMs,
				Arguments:  cloneMap(item.call.Arguments),
				Data:       cloneMap(item.result.Data),
			})
		}
	}
	return roundResults, roundCalls, degradeReasons
}

func (w *AgentLoop) executeCallsInParallel(ctx context.Context, round int, items []roundExecutionItem) {
	maxConcurrency := w.maxParallelToolCalls
	if maxConcurrency <= 1 {
		maxConcurrency = 1
	}
	if maxConcurrency > len(items) {
		maxConcurrency = len(items)
	}

	sem := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup
	for idx := range items {
		wg.Add(1)
		sem <- struct{}{}
		go func(itemIndex int) {
			defer wg.Done()
			defer func() { <-sem }()
			items[itemIndex] = w.executeSingleCall(ctx, round, items[itemIndex])
		}(idx)
	}
	wg.Wait()
}

func (w *AgentLoop) executeSingleCall(ctx context.Context, round int, item roundExecutionItem) roundExecutionItem {
	startedAt := time.Now()
	result, err := w.executor.Execute(ctx, item.call)
	durationMs := time.Since(startedAt).Milliseconds()
	summary := strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage))
	item.result = result
	item.err = err
	item.callSummary = CallSummary{
		CallID:     item.callID,
		Round:      round,
		Sequence:   item.index + 1,
		Name:       strings.TrimSpace(result.Name),
		Status:     strings.TrimSpace(result.Status),
		Summary:    summary,
		DurationMs: durationMs,
		Arguments:  cloneMap(item.call.Arguments),
		Data:       cloneMap(result.Data),
	}
	return item
}

func (w *AgentLoop) roundExecutionMode(callCount int) string {
	if w == nil {
		return "serial"
	}
	if w.parallelToolCalls && w.maxParallelToolCalls > 1 && callCount > 1 {
		return "parallel"
	}
	return "serial"
}
