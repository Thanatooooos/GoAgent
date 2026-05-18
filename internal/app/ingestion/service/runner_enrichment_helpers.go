package service

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type enrichmentTask struct {
	Type               string
	SystemPrompt       string
	UserPromptTemplate string
}

func readEnrichmentTasks(settings map[string]any) []enrichmentTask {
	if len(settings) == 0 {
		return nil
	}
	raw, ok := settings["tasks"]
	if !ok || raw == nil {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		if direct, ok := raw.([]map[string]any); ok {
			list = make([]any, 0, len(direct))
			for _, item := range direct {
				list = append(list, item)
			}
		} else {
			return nil
		}
	}
	result := make([]enrichmentTask, 0, len(list))
	for _, item := range list {
		mapped, ok := item.(map[string]any)
		if !ok {
			continue
		}
		taskType := strings.ToLower(readStringSetting(mapped, "type"))
		if taskType == "" {
			continue
		}
		result = append(result, enrichmentTask{
			Type:               taskType,
			SystemPrompt:       readStringSetting(mapped, "systemPrompt"),
			UserPromptTemplate: readStringSetting(mapped, "userPromptTemplate"),
		})
	}
	return result
}

func ensureMetadata(values map[string]any) map[string]any {
	if values == nil {
		return map[string]any{}
	}
	return values
}

func cloneMetadata(values map[string]any) map[string]any {
	if len(values) == 0 {
		return map[string]any{}
	}
	cloned := make(map[string]any, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func mergeMetadata(dst map[string]any, src map[string]any, overwrite bool) map[string]any {
	dst = ensureMetadata(dst)
	for key, value := range src {
		if _, exists := dst[key]; exists && !overwrite {
			continue
		}
		dst[key] = value
	}
	return dst
}

func normalizeWhitespace(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func truncateText(text string, maxRunes int) string {
	text = normalizeWhitespace(text)
	if text == "" || maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxRunes {
		return text
	}
	runes := []rune(text)
	return strings.TrimSpace(string(runes[:maxRunes])) + "..."
}

func splitSentences(text string) []string {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		switch r {
		case '\n', '\r', '.', '!', '?', ';', ',', '。', '！', '？', '；', '，', '：', ':':
			return true
		default:
			return false
		}
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = normalizeWhitespace(part)
		if utf8.RuneCountInString(part) < 2 {
			continue
		}
		result = append(result, part)
	}
	return result
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func tokenizeText(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	parts := strings.FieldsFunc(text, func(r rune) bool {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || isCJK(r) {
			return false
		}
		return true
	})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		result = append(result, part)
	}
	return result
}

var keywordStopwords = map[string]bool{
	"the": true, "and": true, "for": true, "that": true, "this": true, "with": true,
	"from": true, "have": true, "has": true, "are": true, "was": true, "were": true,
	"you": true, "your": true, "what": true, "when": true, "where": true, "which": true,
	"如何": true, "什么": true, "哪些": true, "以及": true, "或者": true, "因为": true,
}

func extractKeywords(title string, text string, max int) []string {
	if max <= 0 {
		max = 5
	}
	seen := map[string]bool{}
	result := make([]string, 0, max)
	add := func(value string) {
		value = normalizeWhitespace(value)
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if keywordStopwords[key] || seen[key] {
			return
		}
		runeCount := utf8.RuneCountInString(value)
		if runeCount < 2 || runeCount > 24 {
			return
		}
		seen[key] = true
		result = append(result, value)
	}

	add(truncateText(title, 20))
	for _, token := range tokenizeText(title + "\n" + text) {
		add(token)
		if len(result) >= max {
			return result
		}
	}
	for _, sentence := range splitSentences(text) {
		add(truncateText(sentence, 16))
		if len(result) >= max {
			return result
		}
	}
	return result
}

func generateQuestions(title string, text string, keywords []string, max int) []string {
	if max <= 0 {
		max = 3
	}
	seen := map[string]bool{}
	result := make([]string, 0, max)
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			return
		}
		seen[value] = true
		result = append(result, value)
	}

	title = strings.TrimSpace(title)
	if title != "" {
		add(fmt.Sprintf("《%s》主要讲了什么？", truncateText(title, 24)))
	}
	for _, keyword := range keywords {
		add(fmt.Sprintf("%s有哪些关键信息？", keyword))
		if len(result) >= max {
			return result
		}
	}
	if len(result) == 0 && strings.TrimSpace(text) != "" {
		add("这段内容的核心信息是什么？")
	}
	return result
}

func inferLanguage(text string) string {
	var cjkCount int
	var latinCount int
	for _, r := range text {
		switch {
		case isCJK(r):
			cjkCount++
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			latinCount++
		}
	}
	switch {
	case cjkCount > latinCount:
		return "zh"
	case latinCount > 0:
		return "en"
	default:
		return "unknown"
	}
}

func buildHeuristicMetadata(text string, title string) map[string]any {
	normalized := normalizeWhitespace(text)
	return map[string]any{
		"title":       strings.TrimSpace(title),
		"language":    inferLanguage(normalized),
		"char_count":  utf8.RuneCountInString(normalized),
		"token_count": len(strings.Fields(normalized)),
	}
}

func buildDocumentContextHeader(state ExecutionState) string {
	lines := make([]string, 0, 4)
	if title := strings.TrimSpace(state.Parsed.Title); title != "" {
		lines = append(lines, "标题: "+title)
	}
	if fileName := strings.TrimSpace(state.Source.FileName); fileName != "" {
		lines = append(lines, "文件: "+fileName)
	}
	if sourceType := strings.TrimSpace(state.Source.Type); sourceType != "" {
		lines = append(lines, "来源类型: "+sourceType)
	}
	if contentType := strings.TrimSpace(state.Source.ContentType); contentType != "" {
		lines = append(lines, "内容类型: "+contentType)
	}
	return strings.Join(lines, "\n")
}

func prependIfMissing(text string, prefix string) string {
	text = strings.TrimSpace(text)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" || text == "" || strings.HasPrefix(text, prefix) {
		return text
	}
	return prefix + "\n\n" + text
}

func buildChunkDocumentMetadata(state ExecutionState) map[string]any {
	metadata := cloneMetadata(state.Parsed.Metadata)
	metadata["document_title"] = strings.TrimSpace(state.Parsed.Title)
	metadata["source_type"] = strings.TrimSpace(state.Source.Type)
	metadata["source_file_name"] = strings.TrimSpace(state.Source.FileName)
	metadata["source_content_type"] = strings.TrimSpace(state.Source.ContentType)
	metadata["document_summary"] = truncateText(state.Parsed.Content, 180)
	return metadata
}
