package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	defaultMaxTools = 3
	plannerExamples = `Examples:
1. If the current agent state contains nextHintCalls=[{"name":"document_ingestion_diagnose","arguments":{"documentId":"doc-1"}}], prefer that tool call next instead of going back to document_query.
2. If previous results already show "document_ingestion_diagnose: document ingestion failed at node indexer" and nextHintCalls=[{"name":"ingestion_task_node_query","arguments":{"taskId":"task-1","nodeId":"indexer"}}], plan only ingestion_task_node_query with the exact ids.
3. If previous results already contain the same tool call and no new hint or new id is available, return {"tools":[]} instead of repeating the same call.
4. If the current evidence is already enough to answer and no extra lookup is needed, return {"tools":[]}.
5. If one tool call depends on the output of another call on the same entity, do not plan them together. For example, do not plan both ingestion_task_query(task-1) and ingestion_task_node_query(task-1, node-x) in the same round unless node-x is already confirmed by previous evidence or nextHintCalls.
6. Only plan multiple tool calls in parallel when they are independent. For example, document_query(doc-1) and trace_node_query(trace-1) may run together, but document_query(doc-1) and document_ingestion_diagnose(doc-1) should stay serial because the second depends on the first.`
	plannerSystemTemplate = `You are a tool planning assistant for an agentic diagnostic workflow.

Available tools:
%s

Rules:
1. Only plan tool calls when the user question or the previous structured nextHintCalls requires a concrete lookup.
2. Never invent ids. Use only ids that appear in the user question or the previous structured nextHintCalls.
3. Copy argument values exactly from the question, nextHintCalls, or previous results.
4. Do not repeat an equivalent tool call that was already executed.
5. Prefer following the structured nextHintCalls when it is present.
6. If the evidence is already sufficient, return an empty tools array.
7. Only plan multiple tool calls in the same round when they are independent and can be executed in parallel safely.
8. Do not plan multiple drill-down calls for the same entity in one round. Prefer the shallowest missing lookup first, then wait for the next round.
9. For the same task/document/trace, avoid combinations like document_query + document_ingestion_diagnose, ingestion_task_query + ingestion_task_node_query, or task_ingestion_diagnose + ingestion_task_node_query in the same round unless previous evidence already makes the deeper call independently valid.
10. Plan at most %d tool calls.

%s

Return strict JSON only:
{"tools":[{"name":"tool_name","arguments":{"arg":"value"}}]}`
)

type plannerResponse struct {
	Tools []plannerToolCall `json:"tools"`
}

type plannerToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// LLMPlanner uses an LLM to decide which tools should be called next.
type LLMPlanner struct {
	chatService aichat.LLMService
	maxTools    int
}

func NewLLMPlanner(chatService aichat.LLMService) *LLMPlanner {
	return &LLMPlanner{
		chatService: chatService,
		maxTools:    defaultMaxTools,
	}
}

func (p *LLMPlanner) Plan(ctx context.Context, input ragcore.PlanInput) (ragcore.PlanResult, error) {
	if p == nil || p.chatService == nil {
		return ragcore.PlanResult{}, nil
	}

	question := strings.TrimSpace(input.Question)
	if question == "" || len(input.ToolDefinitions) == 0 {
		return ragcore.PlanResult{}, nil
	}

	systemPrompt := p.buildSystemPrompt(input.ToolDefinitions)
	userPrompt := p.buildUserPrompt(input)
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(systemPrompt),
			convention.UserMessage(userPrompt),
		},
	}
	jsonMode := true
	request.JSONMode = &jsonMode

	response, err := p.chatService.ChatWithRequest(request)
	if err != nil {
		return ragcore.PlanResult{}, fmt.Errorf("planner llm call: %w", err)
	}

	return p.parseResponse(response), nil
}

func (p *LLMPlanner) buildSystemPrompt(defs []ragcore.Definition) string {
	var toolList strings.Builder
	for _, def := range defs {
		fmt.Fprintf(&toolList, "- %s: %s\n", def.Name, def.Description)
		for _, param := range def.Parameters {
			req := ""
			if param.Required {
				req = ", required"
			}
			desc := ""
			if param.Description != "" {
				desc = " - " + param.Description
			}
			fmt.Fprintf(&toolList, "  parameter: %s (%s%s)%s\n", param.Name, param.Type, req, desc)
		}
	}
	return fmt.Sprintf(
		plannerSystemTemplate,
		strings.TrimRight(toolList.String(), "\n"),
		p.maxTools,
		plannerExamples,
	)
}

func (p *LLMPlanner) buildUserPrompt(input ragcore.PlanInput) string {
	var builder strings.Builder
	builder.WriteString("User question:\n")
	builder.WriteString(strings.TrimSpace(input.Question))
	if rewriteSummary := ragcore.SummarizeRewriteResultForLLM(input.RewriteResult); rewriteSummary != "" {
		builder.WriteString("\n\nRewrite context:\n")
		builder.WriteString(rewriteSummary)
	}
	if retrieveSummary := ragcore.SummarizeRetrieveResultForLLM(input.RetrieveResult); retrieveSummary != "" {
		builder.WriteString("\n\nRetrieve context:\n")
		builder.WriteString(retrieveSummary)
	}
	if state := input.AgentState.Normalize(); !state.Empty() {
		builder.WriteString("\n\nCurrent agent state (follow nextHint first when valid):\n")
		builder.WriteString(state.PromptString())
	}
	if len(input.PreviousResults) > 0 {
		builder.WriteString("\n\nPrevious tool results:\n")
		for _, result := range input.PreviousResults {
			name := strings.TrimSpace(result.Name)
			summary := strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage))
			if name == "" || summary == "" {
				continue
			}
			builder.WriteString("- ")
			builder.WriteString(name)
			builder.WriteString(": ")
			builder.WriteString(summary)
			if dataSummary := ragcore.SummarizeResultDataForLLM(result.Data); dataSummary != "" {
				builder.WriteString(" | data: ")
				builder.WriteString(dataSummary)
			}
			builder.WriteString("\n")
		}
	}
	if len(input.KnowledgeBaseIDs) > 0 {
		builder.WriteString("\nKnowledge base scope:\n")
		builder.WriteString("- ")
		builder.WriteString(strings.Join(input.KnowledgeBaseIDs, ", "))
		builder.WriteString("\n")
	}
	builder.WriteString("\nParallel planning guidance:\n")
	builder.WriteString("- Only group calls that target different independent entities or domains.\n")
	builder.WriteString("- For the same entity, prefer one incremental lookup per round instead of batching a whole drill-down chain.\n")
	builder.WriteString("- If one call needs another call to reveal a missing id or status, keep them serial across rounds.\n")
	builder.WriteString("\nReturn only the next tool calls that are still necessary.")
	return builder.String()
}

func (p *LLMPlanner) parseResponse(raw string) ragcore.PlanResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ragcore.PlanResult{}
	}

	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}

	var parsed plannerResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return ragcore.PlanResult{}
	}

	calls := make([]ragcore.Call, 0, len(parsed.Tools))
	for _, tc := range parsed.Tools {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			continue
		}
		calls = append(calls, ragcore.Call{
			Name:      name,
			Arguments: tc.Arguments,
		})
	}

	return ragcore.PlanResult{Calls: calls}
}

func extractJSONBlock(raw string) string {
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
