package core

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/framework/convention"
)

const (
	maxLLMSummaryItems    = 3
	maxLLMSummaryTextLen  = 160
	maxLLMSummaryParts    = 18
	maxLLMSummaryJSONKeys = 6
)

func SummarizeResultDataForLLM(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}

	parts := make([]string, 0, maxLLMSummaryParts)
	seen := make(map[string]struct{}, len(data))
	appendPart := func(key string, value string) {
		value = strings.TrimSpace(value)
		if key == "" || value == "" || len(parts) >= maxLLMSummaryParts {
			return
		}
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		parts = append(parts, fmt.Sprintf("%s=%s", key, value))
	}

	for _, key := range []string{
		"documentId",
		"taskId",
		"nodeId",
		"traceId",
		"searchQuery",
		"status",
		"processMode",
		"latestTaskId",
		"latestNodeId",
		"latestNodeError",
		"latestLogError",
		"conclusion",
		"confidence",
		"quality",
		"qualityConfidence",
		"qualityReasoning",
		"sourceCoverage",
		"sourceDiversity",
		"corroboration",
		"readiness",
		"readinessConfidence",
		"readinessReasoning",
		"answerStrategy",
		"errorMessage",
		"diagnosisScope",
		"diagnosisDepth",
	} {
		appendPart(key, readSummarizedDataValue(data, key))
	}

	if nodeSummary := summarizeTaskNodeSummary(data["taskNodeSummary"]); nodeSummary != "" {
		appendPart("taskNodeSummary", nodeSummary)
	}
	if nodes := summarizeTaskNodes(data["nodes"]); nodes != "" {
		appendPart("nodes", nodes)
	}

	for _, key := range []string{
		"suggestions",
		"riskHints",
		"facts",
		"inferences",
		"nextActions",
		"evidence",
		"rawEvidence",
		"missingInformation",
		"selectedUrls",
		"selectedDomains",
		"selectedSourceTypes",
		"citedUrls",
	} {
		appendPart(key, summarizeGenericValue(data[key]))
	}

	remainingKeys := make([]string, 0, len(data))
	for key := range data {
		if _, exists := seen[key]; exists {
			continue
		}
		if isLLMSummaryNoiseKey(key) {
			continue
		}
		remainingKeys = append(remainingKeys, key)
	}
	sort.Strings(remainingKeys)
	for _, key := range remainingKeys {
		appendPart(key, summarizeGenericValue(data[key]))
	}

	return strings.Join(parts, ", ")
}

func isLLMSummaryNoiseKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "", "rawBody", "fullText", "rawText", "rawContent", "originalText":
		return true
	default:
		return false
	}
}

func readSummarizedDataValue(data map[string]any, key string) string {
	if value := ReadDataString(data, key); value != "" {
		return trimSummaryText(value)
	}
	if count := ReadDataInt(data, key); count > 0 {
		return fmt.Sprintf("%d", count)
	}
	if raw, ok := data[key]; ok {
		return summarizeGenericScalar(raw)
	}
	return ""
}

func summarizeTaskNodeSummary(raw any) string {
	switch typed := raw.(type) {
	case []map[string]any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if summary := summarizeTaskNodeItem(item); summary != "" {
				items = append(items, summary)
			}
		}
		return strings.Join(limitStrings(items, maxLLMSummaryItems), "; ")
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if summary := summarizeTaskNodeItem(mapped); summary != "" {
				items = append(items, summary)
			}
		}
		return strings.Join(limitStrings(items, maxLLMSummaryItems), "; ")
	default:
		return ""
	}
}

func summarizeTaskNodes(raw any) string {
	switch typed := raw.(type) {
	case []map[string]any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if summary := summarizeTaskNodeItem(item); summary != "" {
				items = append(items, summary)
			}
		}
		return strings.Join(limitStrings(items, maxLLMSummaryItems), "; ")
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			mapped, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if summary := summarizeTaskNodeItem(mapped); summary != "" {
				items = append(items, summary)
			}
		}
		return strings.Join(limitStrings(items, maxLLMSummaryItems), "; ")
	default:
		return ""
	}
}

func summarizeTaskNodeItem(item map[string]any) string {
	if len(item) == 0 {
		return ""
	}
	nodeID := strings.TrimSpace(ReadStringArg(item, "nodeId"))
	status := strings.TrimSpace(ReadStringArg(item, "status"))
	nodeType := strings.TrimSpace(ReadStringArg(item, "nodeType"))
	errMsg := strings.TrimSpace(ReadStringArg(item, "errorMessage"))
	if nodeID == "" {
		return ""
	}
	parts := []string{nodeID}
	if status != "" {
		parts = append(parts, "status="+status)
	}
	if nodeType != "" {
		parts = append(parts, "type="+nodeType)
	}
	if errMsg != "" {
		parts = append(parts, "error="+trimSummaryText(errMsg))
	}
	return strings.Join(parts, "|")
}

func summarizeGenericValue(raw any) string {
	switch typed := raw.(type) {
	case nil:
		return ""
	case string:
		return trimSummaryText(typed)
	case []string:
		return strings.Join(limitStrings(trimAndFilterStrings(typed), maxLLMSummaryItems), " | ")
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if summary := summarizeGenericValue(item); summary != "" {
				items = append(items, summary)
			}
		}
		return strings.Join(limitStrings(items, maxLLMSummaryItems), " | ")
	case map[string]any:
		return summarizeMapValue(typed)
	default:
		return summarizeGenericScalar(raw)
	}
}

func summarizeMapValue(data map[string]any) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range limitStrings(keys, maxLLMSummaryJSONKeys) {
		if value := summarizeGenericValue(data[key]); value != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	return strings.Join(parts, "; ")
}

func summarizeGenericScalar(raw any) string {
	switch typed := raw.(type) {
	case bool:
		if typed {
			return "true"
		}
		return "false"
	case int:
		return fmt.Sprintf("%d", typed)
	case int32:
		return fmt.Sprintf("%d", typed)
	case int64:
		return fmt.Sprintf("%d", typed)
	case float32:
		return trimSummaryText(fmt.Sprintf("%.4g", typed))
	case float64:
		return trimSummaryText(fmt.Sprintf("%.4g", typed))
	default:
		encoded, err := json.Marshal(raw)
		if err != nil {
			return ""
		}
		return trimSummaryText(string(encoded))
	}
}

func trimSummaryText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.Join(strings.Fields(value), " ")
	if len(value) <= maxLLMSummaryTextLen {
		return value
	}
	return strings.TrimSpace(value[:maxLLMSummaryTextLen-3]) + "..."
}

func limitStrings(items []string, max int) []string {
	if len(items) == 0 {
		return nil
	}
	if max <= 0 || len(items) <= max {
		return items
	}
	return items[:max]
}

func trimAndFilterStrings(items []string) []string {
	filtered := make([]string, 0, len(items))
	for _, item := range items {
		if trimmed := trimSummaryText(item); trimmed != "" {
			filtered = append(filtered, trimmed)
		}
	}
	return filtered
}


func SummarizeRewriteResultForLLM(result ragrewrite.Result) string {
	parts := make([]string, 0, 3)
	if question := strings.TrimSpace(result.RewrittenQuestion); question != "" {
		parts = append(parts, "rewrittenQuestion="+trimSummaryText(question))
	}
	if len(result.SubQuestions) > 0 {
		parts = append(parts, "subQuestions="+strings.Join(limitStrings(trimAndFilterStrings(result.SubQuestions), 3), " | "))
	}
	if mode := strings.TrimSpace(result.PreferredSearchMode); mode != "" {
		parts = append(parts, "preferredSearchMode="+mode)
	}
	return strings.Join(parts, ", ")
}

func SummarizeRetrieveResultForLLM(result ragretrieve.Result) string {
	parts := make([]string, 0, 4)
	if len(result.SearchChannels) > 0 {
		parts = append(parts, "searchChannels="+strings.Join(limitStrings(result.SearchChannels, 4), ", "))
	}
	if len(result.ChannelStats) > 0 {
		stats := make([]string, 0, len(result.ChannelStats))
		for _, stat := range limitChannelStats(result.ChannelStats, 3) {
			item := stat.Name
			if stat.ChunkCount > 0 {
				item += fmt.Sprintf("(chunks=%d", stat.ChunkCount)
				if stat.Error != "" {
					item += ", error=" + trimSummaryText(stat.Error)
				}
				item += ")"
			} else if stat.Error != "" {
				item += "(error=" + trimSummaryText(stat.Error) + ")"
			}
			stats = append(stats, item)
		}
		if len(stats) > 0 {
			parts = append(parts, "channelStats="+strings.Join(stats, " | "))
		}
	}
	if len(result.Chunks) > 0 {
		chunks := make([]string, 0, len(result.Chunks))
		for idx, chunk := range limitChunks(result.Chunks, 3) {
			chunks = append(chunks, fmt.Sprintf("[%d] %s", idx+1, summarizeRetrievedChunk(chunk)))
		}
		if len(chunks) > 0 {
			parts = append(parts, "topChunks="+strings.Join(chunks, " | "))
		}
	}
	return strings.Join(parts, ", ")
}

func limitChannelStats(stats []ragretrieve.ChannelStat, max int) []ragretrieve.ChannelStat {
	if len(stats) == 0 || max <= 0 || len(stats) <= max {
		return stats
	}
	return stats[:max]
}

func limitChunks[T any](items []T, max int) []T {
	if len(items) == 0 || max <= 0 || len(items) <= max {
		return items
	}
	return items[:max]
}

func summarizeRetrievedChunk(chunk convention.RetrievedChunk) string {
	parts := make([]string, 0, 3)
	if section, ok := chunk.Metadata["section"].(string); ok && strings.TrimSpace(section) != "" {
		parts = append(parts, "section="+trimSummaryText(section))
	}
	if fileName, ok := chunk.Metadata["source_file_name"].(string); ok && strings.TrimSpace(fileName) != "" {
		parts = append(parts, "file="+trimSummaryText(fileName))
	}
	parts = append(parts, "text="+trimSummaryText(chunk.Text))
	return strings.Join(parts, ", ")
}
