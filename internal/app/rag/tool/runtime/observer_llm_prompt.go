package runtime

import (
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
)

var defaultObserverExamples = []string{
	`Evidence is already sufficient:
Current result: ingestion_task_node_query says node indexer failed with "connection refused: vector store unavailable".
Return: {"done":true,"reasoning":"Node-level error is already available.","state":{"phase":"complete","hypothesis":"indexer failed because vector store was unavailable","confidence":0.95,"openQuestions":[],"checkedTools":["ingestion_task_node_query"],"nextHintCalls":[]}}`,
	`Evidence is not deep enough yet:
Current result: document_ingestion_diagnose only shows latestTaskId and latestLogError, but no node-level error.
Return: {"done":false,"reasoning":"Task or chunk-log evidence is not enough; inspect the task detail next.","state":{"phase":"deep_dive","hypothesis":"the task failed but the concrete node is still unknown","confidence":0.62,"openQuestions":["Which task node actually failed?","Is there a node-level error message?"],"checkedTools":["document_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}`,
	`Task query shows a failed node but the concrete error is still unknown:
Current result: ingestion_task_query shows task-1 status=failed with taskNodeSummary=[indexer(status=failed,type=indexer)]. The node-level errorMessage is missing.
Return: {"done":false,"reasoning":"The task summary shows a failed indexer node, but the concrete node error is not yet available. Must inspect the node directly.","state":{"phase":"deep_dive","hypothesis":"indexer failed but node error is unknown","confidence":0.55,"openQuestions":["What is the concrete node-level error message?"],"checkedTools":["ingestion_task_query"],"nextHintCalls":[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}]}}`,
	`Running state should not be over-diagnosed:
Current result: task_ingestion_diagnose says the task is still running and no failed node is shown yet.
Return: {"done":false,"reasoning":"The task is still running; inspect the live task detail instead of guessing a failed node.","state":{"phase":"verification","hypothesis":"the task is still in progress rather than failed at a confirmed node","confidence":0.48,"openQuestions":["Which node is still running right now?"],"checkedTools":["task_ingestion_diagnose"],"nextHintCalls":[{"name":"ingestion_task_query","arguments":{"taskId":"task-1","includeNodes":true}}]}}`,
	`Knowledge base retrieval returned no or insufficient results, and the user asks a general knowledge question:
Retrieve context: searchChannels=vector_global, keyword, metadata_title, channelStats=vector_global(chunks=0) | keyword(chunks=0) | metadata_title(chunks=0). No chunks were found in the knowledge base.
Current result: (no previous tool results - this is the first round)
Return: {"done":false,"reasoning":"The knowledge base has no relevant content for this question. A web search is needed to find external information.","state":{"phase":"external_search","hypothesis":"the knowledge base does not cover this topic, external search is required","confidence":0.25,"openQuestions":["What does external search return?"],"checkedTools":[],"nextHintCalls":[{"name":"web_search","arguments":{"query":"<rewrite the user question as a concise search query>"}}]}}`,
	`Web page content has been fetched and is sufficient:
Current result: web_fetch: fetched 2 urls: 2 ok, 0 failed | data: combinedText="..."
Return: {"done":true,"reasoning":"Web page content has been fetched. The agent has enough information to answer with source attribution.","state":{"phase":"complete","hypothesis":"external sources provide sufficient information to answer the question","confidence":0.8,"openQuestions":[],"checkedTools":["web_search","web_fetch"],"nextHintCalls":[]}}`,
}

const observerSystemTemplate = `You are the observe phase of an agentic diagnostic workflow.

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

func (o *LLMObserver) buildSystemPrompt(input ObserveInput) string {
	return fmt.Sprintf(
		observerSystemTemplate,
		renderToolDefinitionsForPrompt(input.ToolDefinitions),
		renderObserverExamples(collectObserverExamples(input.ToolRegistry, input.ToolDefinitions)),
	)
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

func collectObserverExamples(registry *Registry, defs []Definition) []string {
	if registry == nil {
		return append([]string(nil), defaultObserverExamples...)
	}
	seen := map[string]struct{}{}
	examples := make([]string, 0)
	for _, def := range defs {
		behavior, ok := registry.GetBehavior(def.Name)
		if !ok || len(behavior.ObserverExamples) == 0 {
			continue
		}
		for _, example := range behavior.ObserverExamples {
			example = strings.TrimSpace(example)
			if example == "" {
				continue
			}
			if _, exists := seen[example]; exists {
				continue
			}
			seen[example] = struct{}{}
			examples = append(examples, example)
		}
	}
	if len(examples) == 0 {
		return append([]string(nil), defaultObserverExamples...)
	}
	return examples
}

func renderObserverExamples(examples []string) string {
	examples = UniqueTrimmedStrings(examples)
	if len(examples) == 0 {
		return ""
	}
	var builder strings.Builder
	builder.WriteString("Examples:\n")
	for idx, example := range examples {
		builder.WriteString(fmt.Sprintf("%d. %s", idx+1, strings.TrimSpace(example)))
		if idx < len(examples)-1 {
			builder.WriteString("\n\n")
		}
	}
	return builder.String()
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
