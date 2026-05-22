package runtime

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

const DefaultMaxIterations = 3

const (
	planningSourceLLM            = "llm"
	planningSourceHintCalls      = "hint_calls"
	planningSourceResultFallback = "result_fallback"
	planningSourceBaseRules      = "base_rules"
)

type planningDecision struct {
	Calls             []Call
	Source            string
	LLMPlannerSkipped bool
}

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
		currentState := agentState.Normalize()
		planning := w.planCalls(ctx, round, input, agentState, allResults, executed)
		plannedCalls := planning.Calls
		log.Infof(
			"[agent] round %d planning: source=%s llmPlannerSkipped=%v nextHintCallCount=%d previousResults=%d plannedCalls=%d",
			round,
			strings.TrimSpace(planning.Source),
			planning.LLMPlannerSkipped,
			len(currentState.NextHintCalls),
			len(allResults),
			len(plannedCalls),
		)
		if len(plannedCalls) == 0 {
			if round > 1 && !agentState.Empty() {
				normalizedState := agentState.Normalize()
				log.Infof(
					"[agent] round %d: planner produced no new calls, stopping (source=%s llmPlannerSkipped=%v)",
					round,
					strings.TrimSpace(planning.Source),
					planning.LLMPlannerSkipped,
				)
				rounds = append(rounds, RoundSummary{
					Round:             round,
					Done:              true,
					Reasoning:         "planning produced no new tool calls, so the agent loop stops here.",
					NextHintCalls:     append([]HintCall(nil), normalizedState.NextHintCalls...),
					NextHint:          normalizedState.NextHint,
					Confidence:        normalizedState.Confidence,
					State:             normalizedState,
					PlanningSource:    strings.TrimSpace(planning.Source),
					LLMPlannerSkipped: planning.LLMPlannerSkipped,
					NextHintCallCount: len(normalizedState.NextHintCalls),
				})
			}
			break
		}

		toolNames := make([]string, len(plannedCalls))
		for i, c := range plannedCalls {
			toolNames[i] = strings.TrimSpace(c.Name)
		}
		levels := resolveExecutionLevels(plannedCalls, w.executor.registry)
		mode := w.roundExecutionMode(len(plannedCalls), levels)
		log.Infof("[agent] round %d: %d call(s) [%s] (%s)", round, len(plannedCalls), strings.Join(toolNames, ", "), mode)

		roundStartedAt := time.Now()
		roundResults, roundCalls, roundDegradeReasons := w.executeRoundCalls(ctx, input.EventSink, round, plannedCalls, levels)
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

		observation := ObserveResult{
			Done:  true,
			State: cloneAgentState(agentState),
		}
		if w.observer != nil {
			obs, err := w.observer.Observe(ctx, observeInput)
			if err != nil {
				degradeReasons = append(degradeReasons, err.Error())
				failedState := cloneAgentState(agentState)
				failedState.Phase = "complete"
				observation = ObserveResult{
					Done:      true,
					Reasoning: "observe phase failed, so the agent loop stops with the current evidence.",
					State:     failedState,
				}
			} else {
				observation = obs
			}
		}
		state := normalizeObservationState(observation, agentState)
		observation.State = state

		totalToolDurationMs := int64(0)
		for _, call := range roundCalls {
			totalToolDurationMs += maxInt64(call.DurationMs, 0)
		}

		roundSummary := RoundSummary{
			Round:               round,
			Calls:               roundCalls,
			Done:                observation.Done,
			Reasoning:           strings.TrimSpace(observation.Reasoning),
			NextHintCalls:       append([]HintCall(nil), state.NextHintCalls...),
			NextHint:            strings.TrimSpace(state.NextHint),
			Confidence:          state.Confidence,
			State:               state,
			ExecutionMode:       w.roundExecutionMode(len(plannedCalls), levels),
			PlanningSource:      strings.TrimSpace(planning.Source),
			LLMPlannerSkipped:   planning.LLMPlannerSkipped,
			NextHintCallCount:   len(state.NextHintCalls),
			WallClockDurationMs: roundWallClockDurationMs,
			ToolCallCount:       len(roundCalls),
			TotalToolDurationMs: totalToolDurationMs,
		}
		rounds = append(rounds, roundSummary)

		if observation.Done {
			log.Infof("[agent] round %d observer: DONE (confidence=%.2f, wallClock=%dms)", round, state.Confidence, roundWallClockDurationMs)
		} else {
			nextNames := make([]string, len(state.NextHintCalls))
			for i, h := range state.NextHintCalls {
				nextNames[i] = strings.TrimSpace(h.Name)
			}
			log.Infof("[agent] round %d observer: CONTINUE (confidence=%.2f) -> hint: %s", round, state.Confidence, strings.Join(nextNames, ", "))
		}

		if !observation.Done && strings.TrimSpace(observation.Reasoning) != "" && input.EventSink != nil {
			_ = input.EventSink.OnAgentThink(observation.Reasoning)
		}
		if observation.Done {
			break
		}
		agentState = state
	}

	workflowResult := WorkflowResult{
		Used:           len(allResults) > 0,
		Context:        RenderContextWithRegistry(w.executor.registry, allResults),
		AnswerGuidance: BuildAnswerGuidanceWithRegistry(w.executor.registry, allResults),
		Control:        deriveWorkflowControlWithRegistry(input, allResults, w.executor.registry),
		Calls:          allCalls,
		Rounds:         rounds,
	}
	workflowResult.TraceMeta = buildWorkflowTraceMetaWithRegistry(workflowResult.Control, input.RetrieveResult, allResults, w.executor.registry)
	if len(degradeReasons) > 0 {
		workflowResult.Degraded = true
		workflowResult.DegradeReason = strings.Join(UniqueTrimmedStrings(degradeReasons), "; ")
	}
	if len(rounds) == w.maxIterations && len(rounds) > 0 && !rounds[len(rounds)-1].Done {
		workflowResult.Degraded = true
		workflowResult.DegradeReason = strings.TrimSpace(firstNonEmpty(workflowResult.DegradeReason, "agent loop reached max iterations"))
	}
	logPlanningMetrics(rounds)
	log.Infof("[agent] done: %d round(s), %d call(s), degraded=%v", len(rounds), len(allCalls), workflowResult.Degraded)
	return workflowResult, nil
}

func logPlanningMetrics(rounds []RoundSummary) {
	if len(rounds) == 0 {
		return
	}
	sourceCounts := map[string]int{
		planningSourceLLM:            0,
		planningSourceHintCalls:      0,
		planningSourceResultFallback: 0,
		planningSourceBaseRules:      0,
	}
	llmPlannerSkippedRounds := 0
	for _, round := range rounds {
		source := strings.TrimSpace(round.PlanningSource)
		if source != "" {
			sourceCounts[source]++
		}
		if round.LLMPlannerSkipped {
			llmPlannerSkippedRounds++
		}
	}
	log.Infof(
		"[agent] planning metrics: llm=%d hint_calls=%d result_fallback=%d base_rules=%d llmPlannerSkippedRounds=%d totalRounds=%d",
		sourceCounts[planningSourceLLM],
		sourceCounts[planningSourceHintCalls],
		sourceCounts[planningSourceResultFallback],
		sourceCounts[planningSourceBaseRules],
		llmPlannerSkippedRounds,
		len(rounds),
	)
}

func (w *AgentLoop) planCalls(ctx context.Context, round int, input WorkflowInput, agentState AgentState, previousResults []Result, executed map[string]struct{}) planningDecision {
	normalizedState := agentState.Normalize()
	if round > 1 && len(normalizedState.NextHintCalls) > 0 {
		if calls := filterNewCalls(PlanCallsFromHintCalls(normalizedState.NextHintCalls, w.executor.registry.ListDefinitions()), executed); len(calls) > 0 {
			return planningDecision{Calls: calls, Source: planningSourceHintCalls, LLMPlannerSkipped: true}
		}
		fallback := w.planWithRules(input, previousResults, executed)
		fallback.LLMPlannerSkipped = true
		if strings.TrimSpace(fallback.Source) == "" {
			fallback.Source = planningSourceHintCalls
		}
		return fallback
	}

	if w.planner != nil {
		if calls := w.planWithLLM(ctx, input, normalizedState, previousResults, executed); len(calls) > 0 {
			return planningDecision{Calls: calls, Source: planningSourceLLM}
		}
	}
	return w.planWithRules(input, previousResults, executed)
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

func (w *AgentLoop) planWithRules(input WorkflowInput, previousResults []Result, executed map[string]struct{}) planningDecision {
	if calls := filterNewCalls(PlanCallsFromResultsWithRegistry(previousResults, input, w.executor.registry), executed); len(calls) > 0 {
		return planningDecision{Calls: calls, Source: planningSourceResultFallback}
	}
	if calls := filterNewCalls(PlanWithBaseRules(input, DefaultMaxIterations), executed); len(calls) > 0 {
		return planningDecision{Calls: calls, Source: planningSourceBaseRules}
	}
	return planningDecision{Source: planningSourceBaseRules}
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
		arguments := CloneMap(hintCall.Arguments)
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
		coerced, ok := CoerceHintArgument(value, param.Type)
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

func PlanCallsFromResults(results []Result) []Call {
	return PlanCallsFromResultsWithRegistry(results, WorkflowInput{}, nil)
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

func (w *AgentLoop) executeRoundCalls(ctx context.Context, sink WorkflowEventSink, round int, plannedCalls []Call, levels [][]int) ([]Result, []CallSummary, []string) {
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
				Arguments: CloneMap(call.Arguments),
			})
		}
	}

	if !w.parallelToolCalls || len(plannedCalls) == 1 || w.maxParallelToolCalls <= 1 {
		for idx := range items {
			items[idx] = w.executeSingleCall(ctx, round, items[idx])
		}
	} else if levels != nil {
		w.executeCallsByLevel(ctx, round, items, levels)
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
				Arguments:  CloneMap(item.call.Arguments),
				Data:       CloneMap(item.result.Data),
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

// resolveExecutionLevels partitions planned calls into dependency-ordered levels
// using ToolSpec.After declarations from the registry. Returns nil when there are
// no ordering constraints (single level or no dependency data), allowing the
// caller to use the flat-parallel fast path.
func resolveExecutionLevels(calls []Call, registry *Registry) [][]int {
	if registry == nil || len(calls) <= 1 {
		return nil
	}

	planned := make(map[string]int, len(calls))
	for i, c := range calls {
		name := strings.TrimSpace(c.Name)
		if name != "" {
			planned[name] = i
		}
	}

	graph := make([][]int, len(calls))
	inDegree := make([]int, len(calls))

	for i, c := range calls {
		spec, ok := registry.GetSpec(c.Name)
		if !ok || len(spec.After) == 0 {
			continue
		}
		for _, depName := range spec.After {
			if depIdx, found := planned[depName]; found && depIdx != i {
				graph[depIdx] = append(graph[depIdx], i)
				inDegree[i]++
			}
		}
	}

	hasDeps := false
	for _, d := range inDegree {
		if d > 0 {
			hasDeps = true
			break
		}
	}
	if !hasDeps {
		return nil
	}

	type queueEntry struct {
		index int
		level int
	}
	queue := make([]queueEntry, 0, len(calls))

	for i, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, queueEntry{index: i, level: 0})
		}
	}

	levels := make([][]int, 0)
	visited := 0
	for len(queue) > 0 {
		curr := queue[0]
		queue = queue[1:]
		visited++

		for curr.level >= len(levels) {
			levels = append(levels, nil)
		}
		levels[curr.level] = append(levels[curr.level], curr.index)

		for _, dependent := range graph[curr.index] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, queueEntry{index: dependent, level: curr.level + 1})
			}
		}
	}

	if visited != len(calls) {
		log.Warnf("[agent] cycle detected in tool dependency graph: visited %d/%d, falling back to flat parallel", visited, len(calls))
		return nil
	}

	if len(levels) <= 1 {
		return nil
	}

	return levels
}

// executeCallsByLevel executes tool call levels sequentially: level N waits for
// level N-1 to complete. Calls within a single level run in parallel bounded by
// the semaphore (same pattern as executeCallsInParallel).
func (w *AgentLoop) executeCallsByLevel(ctx context.Context, round int, items []roundExecutionItem, levels [][]int) {
	for _, level := range levels {
		if len(level) == 1 {
			items[level[0]] = w.executeSingleCall(ctx, round, items[level[0]])
			continue
		}

		maxConcurrency := w.maxParallelToolCalls
		if maxConcurrency <= 1 {
			maxConcurrency = 1
		}
		if maxConcurrency > len(level) {
			maxConcurrency = len(level)
		}

		sem := make(chan struct{}, maxConcurrency)
		var wg sync.WaitGroup
		for _, idx := range level {
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
		Arguments:  CloneMap(item.call.Arguments),
		Data:       CloneMap(result.Data),
	}
	return item
}

func (w *AgentLoop) roundExecutionMode(callCount int, levels [][]int) string {
	if w == nil {
		return "serial"
	}
	if w.parallelToolCalls && w.maxParallelToolCalls > 1 && callCount > 1 {
		if len(levels) > 1 {
			return fmt.Sprintf("parallel_levels=%d", len(levels))
		}
		return "parallel"
	}
	return "serial"
}
