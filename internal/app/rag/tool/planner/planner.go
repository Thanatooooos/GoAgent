package planner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/app/rag/tool"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	defaultMaxTools       = 3
	plannerSystemTemplate = `你是工具调用规划助手。根据用户问题，决定需要调用哪些工具。

可用工具：
%s

规则：
1. 仅当用户问题明确需要查询具体信息且提供了对应ID时，才规划工具调用，不要编造ID
2. 严格按用户提供的参数值填充 arguments
3. 如果用户问题不需要任何工具，返回空 tools 数组
4. 最多规划 %d 个工具调用

严格按 JSON 格式输出，不要输出其他内容：
{"tools":[{"name":"工具名","arguments":{"参数名":"值"}}]}`
)

type plannerResponse struct {
	Tools []plannerToolCall `json:"tools"`
}

type plannerToolCall struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
}

// LLMPlanner 使用 LLM 决定需要调用哪些 tool。
type LLMPlanner struct {
	chatService aichat.LLMService
	maxTools    int
}

// NewLLMPlanner 创建 LLM tool planner。
func NewLLMPlanner(chatService aichat.LLMService) *LLMPlanner {
	return &LLMPlanner{
		chatService: chatService,
		maxTools:    defaultMaxTools,
	}
}

// Plan 根据用户问题规划 tool 调用列表。
func (p *LLMPlanner) Plan(ctx context.Context, input tool.PlanInput) (tool.PlanResult, error) {
	if p == nil || p.chatService == nil {
		return tool.PlanResult{}, nil
	}

	question := strings.TrimSpace(input.Question)
	if question == "" || len(input.ToolDefinitions) == 0 {
		return tool.PlanResult{}, nil
	}

	systemPrompt := p.buildSystemPrompt(input.ToolDefinitions)
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(systemPrompt),
			convention.UserMessage(question),
		},
	}
	jsonMode := true
	request.JSONMode = &jsonMode

	response, err := p.chatService.ChatWithRequest(request)
	if err != nil {
		return tool.PlanResult{}, fmt.Errorf("planner llm call: %w", err)
	}

	return p.parseResponse(response), nil
}

func (p *LLMPlanner) buildSystemPrompt(defs []tool.Definition) string {
	var toolList strings.Builder
	for _, def := range defs {
		fmt.Fprintf(&toolList, "- %s: %s\n", def.Name, def.Description)
		for _, param := range def.Parameters {
			req := ""
			if param.Required {
				req = ", 必填"
			}
			desc := ""
			if param.Description != "" {
				desc = " - " + param.Description
			}
			fmt.Fprintf(&toolList, "  参数：%s (%s%s)%s\n", param.Name, param.Type, req, desc)
		}
	}
	return fmt.Sprintf(plannerSystemTemplate, strings.TrimRight(toolList.String(), "\n"), p.maxTools)
}

func (p *LLMPlanner) parseResponse(raw string) tool.PlanResult {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return tool.PlanResult{}
	}

	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}

	var parsed plannerResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return tool.PlanResult{}
	}

	calls := make([]tool.Call, 0, len(parsed.Tools))
	for _, tc := range parsed.Tools {
		name := strings.TrimSpace(tc.Name)
		if name == "" {
			continue
		}
		calls = append(calls, tool.Call{
			Name:      name,
			Arguments: tc.Arguments,
		})
	}

	return tool.PlanResult{Calls: calls}
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
