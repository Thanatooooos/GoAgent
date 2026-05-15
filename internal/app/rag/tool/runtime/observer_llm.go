package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	"local/rag-project/internal/framework/log"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	observerExamples = `Examples:
1. Evidence is already sufficient:
Current result: ingestion_task_node_query says node indexer failed with "connection refused: vector store unavailable".
Return: {"done":true,"reasoning":"Node-level error is already available.","state":{"phase":"complete","hypothesis":"indexer failed because vector store was unavailable","confidence":0.95,"openQuestions":[],"checkedTools":["ingestion_task_node_query"],"nextHintCalls":[]}}

2. Evidence is not deep enough yet:
Current result: document_ingestion_diagnose only shows latestTaskId and latestLogError, but no node-level error.
Return: {"done":false,"reasoning":"Task or chunk-log evidence is not enough; inspect the task detail next.","state":{"phase":"deep_dive","hypothesis":"the task failed but the concrete node is still unknown","confidence":0.62,"openQuestions":["Which task node actually failed?","Is there a node-level error message?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}

3. Task query shows a failed node but the concrete error is still unknown:
Current result: ingestion_task_query shows task-1 status=failed with taskNodeSummary=[indexer(status=failed,type=indexer)]. The node-level errorMessage is missing.
Return: {"done":false,"reasoning":"The task summary shows a failed indexer node, but the concrete node error is not yet available. Must inspect the node directly.","state":{"phase":"deep_dive","hypothesis":"indexer failed but node error is unknown","confidence":0.55,"openQuestions":["What is the concrete node-level error message?"],"checkedTools":["ingestion_task_query"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}]}}

4. Running state should not be over-diagnosed:
Current result: task_ingestion_diagnose says the task is still running and no failed node is shown yet.
Return: {"done":false,"reasoning":"The task is still running; inspect the live task detail instead of guessing a failed node.","state":{"phase":"verification","hypothesis":"the task is still in progress rather than failed at a confirmed node","confidence":0.48,"openQuestions":["Which node is still running right now?"],"checkedTools":["task_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}

5. Multiple independent lookups can be hinted in parallel:
Current result: document_ingestion_diagnose returns a failed document doc-1 with latestTaskId task-1 and latestNodeId indexer. The user also asked about the trace linked to this request.
Return: {"done":false,"reasoning":"Both the ingestion task node detail and the trace nodes are needed; they are independent and can run in parallel.","state":{"phase":"deep_dive","hypothesis":"indexer failed; trace context is also needed","confidence":0.65,"openQuestions":["What is the concrete node error?","What does the trace show?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}},{"name":"trace_node_query","arguments":{"traceId":"trace-abc"}}]}}

6. Knowledge base retrieval returned no or insufficient results, and the user asks a general knowledge question:
Retrieve context: searchChannels=vector_global, keyword, metadata_title, channelStats=vector_global(chunks=0) | keyword(chunks=0) | metadata_title(chunks=0). No chunks were found in the knowledge base.
Current result: (no previous tool results - this is the first round)
Return: {"done":false,"reasoning":"The knowledge base has no relevant content for this question. A web search is needed to find external information.","state":{"phase":"external_search","hypothesis":"the knowledge base does not cover this topic, external search is required","confidence":0.25,"openQuestions":["What does external search return?"],"checkedTools":[],"nextHintCalls":[{"name":"web_search","arguments":{"query":"<rewrite the user question as a concise search query>"}}]}}

7. Web search completed and returned several results:
Current result: web_search: found 5 web results | data: results=[{title:"...", url:"https://...", snippet:"..."}, ...]
Return: {"done":false,"reasoning":"Web search found several relevant results. Fetch the top results to get full content before synthesizing an answer.","state":{"phase":"fetching","hypothesis":"web results look relevant, need to read full content","confidence":0.4,"openQuestions":["What does each result page actually say?"],"checkedTools":["web_search"],"nextHintCalls":[{"name":"web_fetch","arguments":{"urls":["https://example.com/1","https://example.com/2"]}}]}}

8. Web page content has been fetched and is sufficient:
Current result: web_fetch: fetched 2 urls: 2 ok, 0 failed | data: combinedText="..."
Return: {"done":true,"reasoning":"Web page content has been fetched. The agent has enough information to answer with source attribution.","state":{"phase":"complete","hypothesis":"external sources provide sufficient information to answer the question","confidence":0.8,"openQuestions":[],"checkedTools":["web_search","web_fetch"],"nextHintCalls":[]}}`
	observerSystemTemplate = `You are the observe phase of an agentic diagnostic workflow.

Your job is to decide whether the agent already has enough evidence to answer, or whether it should continue with one or more next lookups. When multiple independent lookups are needed and they do not depend on each other, provide them together so they can run in parallel.

Available tools:
%s

Rules:
1. Never invent ids. Only use ids that already appear in the question, previous state, rewrite/retrieve context, or tool results.
2. If the current evidence is sufficient, set done=true and leave nextHintCalls empty.
3. If done=false, provide one or more valid nextHintCalls items. Use multiple items only when the calls are independent and can safely run in parallel.
4. When a task query returns a taskNodeSummary or nodes list that contains a failed or running node, you MUST NOT stop. Set done=false and hint ingestion_task_node_query with the specific nodeId to get the concrete error message.
5. Prefer deeper evidence only when it answers an open diagnostic question.
6. Avoid repeating a lookup that was already completed unless the current evidence explicitly requires it.
7. When the task/document is still running, prefer verification over failure-specific deep dives.
8. Preserve structured state with phase, hypothesis, confidence, openQuestions, checkedTools, and nextHintCalls.
9. The think tool result is for reasoning visibility only. Base your Done/hint decision on the last non-think result in the current round.
10. When the retrieve context shows zero chunks or very few/low-quality results, and the question does NOT mention specific document/task/trace IDs, the knowledge base likely lacks relevant content. In this case, consider suggesting web_search to find external information before answering.

%s

Return strict JSON only:
{"done":false,"reasoning":"...","state":{"phase":"deep_dive","hypothesis":"...","confidence":0.72,"openQuestions":["..."],"checkedTools":["..."],"nextHintCalls":[{"name":"tool_name","arguments":{"arg":"value"}}]}}`
)

type llmObserverResponse struct {
	Done      bool                  `json:"done"`
	Reasoning string                `json:"reasoning"`
	State     llmObserverStateBlock `json:"state"`
}

type llmObserverStateBlock struct {
	Phase         string   `json:"phase"`
	Hypothesis    string   `json:"hypothesis"`
	Confidence    float64  `json:"confidence"`
	OpenQuestions []string `json:"openQuestions"`
	CheckedTools  []string `json:"checkedTools"`
	NextHintCalls []HintCall `json:"nextHintCalls"`
	NextHint      string   `json:"nextHint"`
}

// LLMObserver lets the LLM decide whether the agent loop should continue.
// RuleObserver remains as a guardrail fallback when the model output is missing or invalid.
type LLMObserver struct {
	chatService aichat.LLMService
	fallback    Observer
}

func NewLLMObserver(chatService aichat.LLMService) *LLMObserver {
	return &LLMObserver{
		chatService: chatService,
		fallback:    NewRuleObserver(),
	}
}

func (o *LLMObserver) SetFallback(observer Observer) {
	if o == nil || observer == nil {
		return
	}
	o.fallback = observer
}

func (o *LLMObserver) Observe(ctx context.Context, input ObserveInput) (ObserveResult, error) {
	if o == nil || o.chatService == nil {
		return o.observeWithFallback(ctx, input)
	}
	if len(input.RoundResults) == 0 || input.ReachedMaxLoop {
		return o.observeWithFallback(ctx, input)
	}

	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(o.buildSystemPrompt(input.ToolDefinitions)),
			convention.UserMessage(o.BuildUserPrompt(input)),
		},
	}
	jsonMode := true
	request.JSONMode = &jsonMode

	response, err := o.chatService.ChatWithRequest(request)
	if err != nil {
		log.Warnf("llm observer call failed, falling back to rule observer: %v", err)
		fallback, fbErr := o.observeWithFallback(ctx, input)
		if fbErr != nil {
			return ObserveResult{}, fmt.Errorf("llm observer call: %w; fallback: %v", err, fbErr)
		}
		return fallback, nil
	}

	decision, ok := o.parseResponse(response, input)
	if !ok {
		log.Warnf("llm observer parse failed, falling back to rule observer: response=%s", truncateForLog(response))
		return o.observeWithFallback(ctx, input)
	}
	return decision, nil
}

func (o *LLMObserver) observeWithFallback(ctx context.Context, input ObserveInput) (ObserveResult, error) {
	if o != nil && o.fallback != nil {
		return o.fallback.Observe(ctx, input)
	}
	return NewRuleObserver().Observe(ctx, input)
}

func (o *LLMObserver) buildSystemPrompt(defs []Definition) string {
	return fmt.Sprintf(observerSystemTemplate, renderToolDefinitionsForPrompt(defs), observerExamples)
}

func (o *LLMObserver) BuildUserPrompt(input ObserveInput) string {
	var builder strings.Builder
	builder.WriteString("User question:\n")
	builder.WriteString(strings.TrimSpace(input.Question))

	if rewriteSummary := SummarizeRewriteResultForLLM(input.RewriteResult); rewriteSummary != "" {
		builder.WriteString("\n\nRewrite context:\n")
		builder.WriteString(rewriteSummary)
	}

	if retrieveSummary := SummarizeRetrieveResultForLLM(input.RetrieveResult); retrieveSummary != "" {
		builder.WriteString("\n\nRetrieve context:\n")
		builder.WriteString(retrieveSummary)
	}

	if state := input.PreviousState.Normalize(); !state.Empty() {
		builder.WriteString("\n\nPrevious agent state:\n")
		builder.WriteString(state.PromptString())
	}

	if len(input.KnowledgeBaseIDs) > 0 {
		builder.WriteString("\n\nKnowledge base scope:\n- ")
		builder.WriteString(strings.Join(input.KnowledgeBaseIDs, ", "))
	}

	builder.WriteString("\n\nCurrent round tool results:\n")
	for _, result := range input.RoundResults {
		builder.WriteString("- ")
		builder.WriteString(strings.TrimSpace(result.Name))
		builder.WriteString(": ")
		builder.WriteString(strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage)))
		dataSummary := SummarizeResultDataForLLM(result.Data)
		if dataSummary != "" {
			builder.WriteString(" | data: ")
			builder.WriteString(dataSummary)
		}
		builder.WriteString("\n")
	}

	if len(input.Results) > len(input.RoundResults) {
		builder.WriteString("\nEarlier tool history:\n")
		for _, result := range input.Results[:len(input.Results)-len(input.RoundResults)] {
			name := strings.TrimSpace(result.Name)
			summary := strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage))
			if name == "" || summary == "" {
				continue
			}
			builder.WriteString("- ")
			builder.WriteString(name)
			builder.WriteString(": ")
			builder.WriteString(summary)
			if dataSummary := SummarizeResultDataForLLM(result.Data); dataSummary != "" {
				builder.WriteString(" | data: ")
				builder.WriteString(dataSummary)
			}
			builder.WriteString("\n")
		}
	}

	builder.WriteString("\nDecide whether the agent should stop now or continue with one or more nextHintCalls items.")
	return builder.String()
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
		NextHintCalls: firstNonEmptyHintCalls(parsed.State.NextHintCalls, parseHintCallsFromLegacyString(parsed.State.NextHint)),
		NextHint:      parsed.State.NextHint,
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
	state = state.Normalize()

	return ObserveResult{
		Done:          parsed.Done,
		Reasoning:     strings.TrimSpace(parsed.Reasoning),
		NextHintCalls: append([]HintCall(nil), state.NextHintCalls...),
		NextHint:      state.NextHint,
		Confidence:    state.Confidence,
		State:         state,
	}, true
}

func mergeCheckedTools(groups ...[]string) []string {
	merged := make([]string, 0)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return uniqueTrimmedStrings(merged)
}

func toolNames(results []Result) []string {
	if len(results) == 0 {
		return nil
	}
	names := make([]string, 0, len(results))
	for _, result := range results {
		if name := strings.TrimSpace(result.Name); name != "" {
			names = append(names, name)
		}
	}
	return uniqueTrimmedStrings(names)
}

func firstNonEmptyHintCalls(groups ...[]HintCall) []HintCall {
	for _, group := range groups {
		group = normalizeHintCalls(group)
		if len(group) > 0 {
			return group
		}
	}
	return nil
}

func renderToolDefinitionsForPrompt(defs []Definition) string {
	if len(defs) == 0 {
		return "- No tool definitions available."
	}
	var builder strings.Builder
	for _, def := range defs {
		fmt.Fprintf(&builder, "- %s: %s\n", def.Name, def.Description)
		for _, param := range def.Parameters {
			req := ""
			if param.Required {
				req = ", required"
			}
			desc := ""
			if param.Description != "" {
				desc = " - " + param.Description
			}
			fmt.Fprintf(&builder, "  parameter: %s (%s%s)%s\n", param.Name, param.Type, req, desc)
		}
	}
	return strings.TrimRight(builder.String(), "\n")
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

func validateHintAgainstEvidence(nextHintCalls []HintCall, input ObserveInput) bool {
	nextHintCalls = normalizeHintCalls(nextHintCalls)
	if len(nextHintCalls) == 0 {
		return true
	}
	results := append([]Result(nil), input.Results...)
	results = append(results, input.RoundResults...)
	allowed := collectEvidenceIDs(input.Question, input.PreviousState.NextHintCalls, results)
	for _, hintCall := range nextHintCalls {
		if len(hintCall.Arguments) == 0 {
			return false
		}
		for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
			value := strings.TrimSpace(readStringArg(hintCall.Arguments, key))
			if value == "" {
				continue
			}
			if _, ok := allowed[value]; !ok {
				return false
			}
		}
	}
	return true
}

func validateCallAgainstEvidence(call Call, question string, previousHintCalls []HintCall, results []Result) bool {
	if strings.TrimSpace(call.Name) == "" {
		return false
	}
	allowed := collectEvidenceIDs(question, previousHintCalls, results)
	for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
		value := strings.TrimSpace(readStringArg(call.Arguments, key))
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func collectEvidenceIDs(question string, previousHintCalls []HintCall, results []Result) map[string]struct{} {
	allowed := map[string]struct{}{}
	for _, id := range []string{
		firstMatchedID(documentIDPattern, question),
		firstMatchedID(taskIDPattern, question),
		firstMatchedID(traceIDPattern, question),
	} {
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	previousHintCalls = normalizeHintCalls(previousHintCalls)
	for _, hintCall := range previousHintCalls {
		for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
			if value := strings.TrimSpace(readStringArg(hintCall.Arguments, key)); value != "" {
				allowed[value] = struct{}{}
			}
		}
	}
	for _, result := range results {
		collectEvidenceIDsFromData(allowed, result.Data)
	}
	return allowed
}

func collectEvidenceIDsFromData(allowed map[string]struct{}, data map[string]any) {
	if len(data) == 0 {
		return
	}
	for _, key := range []string{"documentId", "taskId", "nodeId", "traceId", "latestTaskId", "latestNodeId"} {
		if value := readDataString(data, key); value != "" {
			allowed[value] = struct{}{}
		}
	}
	switch typed := data["taskNodeSummary"].(type) {
	case []map[string]any:
		for _, item := range typed {
			if nodeID := strings.TrimSpace(readStringArg(item, "nodeId")); nodeID != "" {
				allowed[nodeID] = struct{}{}
			}
		}
	case []any:
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if nodeID := strings.TrimSpace(readStringArg(mapped, "nodeId")); nodeID != "" {
				allowed[nodeID] = struct{}{}
			}
		}
	}
	switch typed := data["nodes"].(type) {
	case []map[string]any:
		for _, item := range typed {
			if nodeID := strings.TrimSpace(readStringArg(item, "nodeId")); nodeID != "" {
				allowed[nodeID] = struct{}{}
			}
		}
	case []any:
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if nodeID := strings.TrimSpace(readStringArg(mapped, "nodeId")); nodeID != "" {
				allowed[nodeID] = struct{}{}
			}
		}
	}
}

func truncateForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= 300 {
		return raw
	}
	return raw[:300] + "..."
}
