package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	inframcp "local/rag-project/internal/infra-mcp"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const defaultTavilyMCPToolName = "tavily-search"

type TavilyMCPProvider struct {
	client     inframcp.ToolClient
	serverName string
	toolName   string
}

func NewTavilyMCPProvider(client inframcp.ToolClient, serverName string, toolName string) *TavilyMCPProvider {
	return &TavilyMCPProvider{
		client:     client,
		serverName: strings.TrimSpace(serverName),
		toolName:   strings.TrimSpace(toolName),
	}
}

func (p *TavilyMCPProvider) ProviderName() string {
	return "tavily-mcp"
}

func (p *TavilyMCPProvider) Search(query string) ([]SearchResult, error) {
	if p.client == nil {
		return nil, fmt.Errorf("tavily mcp client is not configured")
	}
	serverName := strings.TrimSpace(p.serverName)
	if serverName == "" {
		return nil, fmt.Errorf("tavily mcp server is not configured")
	}
	toolName := strings.TrimSpace(p.toolName)
	if toolName == "" {
		toolName = defaultTavilyMCPToolName
	}

	result, err := p.client.CallTool(context.Background(), serverName, toolName, map[string]any{
		"query": query,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		return nil, fmt.Errorf("tavily mcp returned an empty result")
	}
	if result.IsError {
		return nil, fmt.Errorf("tavily mcp tool failed: %s", renderMCPError(result))
	}
	return normalizeTavilyMCPResult(result)
}

func normalizeTavilyMCPResult(result *mcp.CallToolResult) ([]SearchResult, error) {
	payload, err := structuredPayload(result)
	if err != nil {
		return nil, err
	}
	rawResults, ok := payload["results"].([]any)
	if !ok {
		return nil, fmt.Errorf("tavily mcp response missing results")
	}
	normalized := make([]SearchResult, 0, len(rawResults))
	for _, entry := range rawResults {
		item, ok := entry.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("tavily mcp result item has unexpected type %T", entry)
		}
		title := strings.TrimSpace(readAnyString(item["title"]))
		rawURL := strings.TrimSpace(readAnyString(item["url"]))
		content := strings.TrimSpace(readAnyString(item["content"]))
		if content == "" {
			content = strings.TrimSpace(readAnyString(item["snippet"]))
		}
		if content == "" {
			content = strings.TrimSpace(readAnyString(item["text"]))
		}
		score := readAnyFloat(item["score"])
		normalized = append(normalized, SearchResult{
			Title:         title,
			URL:           rawURL,
			Snippet:       truncateText(content, 300),
			Provider:      "tavily-mcp",
			ProviderScore: score,
		})
	}
	if len(rawResults) > 0 && len(normalized) == 0 {
		return nil, fmt.Errorf("tavily mcp response contained no valid results")
	}
	return normalized, nil
}

func structuredPayload(result *mcp.CallToolResult) (map[string]any, error) {
	if result == nil {
		return nil, fmt.Errorf("tavily mcp result is nil")
	}
	if payload, ok := result.StructuredContent.(map[string]any); ok {
		return payload, nil
	}
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if !ok {
			continue
		}
		raw := strings.TrimSpace(text.Text)
		if raw == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(raw), &payload); err == nil {
			return payload, nil
		}
	}
	return nil, fmt.Errorf("tavily mcp response missing structured content")
}

func renderMCPError(result *mcp.CallToolResult) string {
	if result == nil {
		return "unknown mcp error"
	}
	for _, content := range result.Content {
		text, ok := content.(*mcp.TextContent)
		if ok && strings.TrimSpace(text.Text) != "" {
			return strings.TrimSpace(text.Text)
		}
	}
	return "unknown mcp error"
}

func readAnyFloat(value any) *float64 {
	switch typed := value.(type) {
	case float64:
		result := typed
		return &result
	case float32:
		result := float64(typed)
		return &result
	case int:
		result := float64(typed)
		return &result
	case int32:
		result := float64(typed)
		return &result
	case int64:
		result := float64(typed)
		return &result
	case json.Number:
		parsed, err := typed.Float64()
		if err == nil {
			return &parsed
		}
	}
	return nil
}

func readAnyString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprintf("%v", value)
	}
}
