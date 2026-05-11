package retrieve

import (
	"strings"
	"unicode"
)

const (
	modeSourceExplicit = "explicit"
	modeSourceAuto     = "auto"
)

type SearchModeDecision struct {
	RequestedMode string
	ResolvedMode  string
	Source        string
	Reason        string
	Signals       []string
}

type searchModePattern struct {
	label    string
	mode     string
	weight   int
	contains []string
}

var searchModePatterns = []searchModePattern{
	{
		label:    "path_or_code_token",
		mode:     SearchModeHybrid,
		weight:   5,
		contains: []string{"`", "/", "\\", ".go", ".java", ".py", ".sql", ".yaml", ".yml", ".json", ".md", "::", "->"},
	},
	{
		label:    "error_or_runtime_signal",
		mode:     SearchModeHybrid,
		weight:   5,
		contains: []string{"报错", "异常", "错误", "error", "stack trace", "panic", "nil pointer", "404", "500", "timeout", "refused"},
	},
	{
		label:    "config_or_api_locator",
		mode:     SearchModeHybrid,
		weight:   4,
		contains: []string{"配置", "参数", "字段", "函数", "接口", "命令", "sql", "http", "api", "nginx", "docker", "k8s", "kubectl", "redis", "mysql", "postgres"},
	},
	{
		label:    "event_or_protocol_locator",
		mode:     SearchModeHybrid,
		weight:   4,
		contains: []string{"sse", "event", "事件", "fallback", "下发"},
	},
	{
		label:    "exact_match_intent",
		mode:     SearchModeKeyword,
		weight:   4,
		contains: []string{"包含", "出现", "叫做", "名称", "标题", "匹配", "搜索词", "关键字", "章节", "小节", "段落", "contains", "match", "keyword", "named"},
	},
	{
		label:    "quoted_phrase_lookup",
		mode:     SearchModeKeyword,
		weight:   4,
		contains: []string{"\"", "'", "《", "》"},
	},
	{
		label:    "concept_question",
		mode:     SearchModeSemantic,
		weight:   4,
		contains: []string{"什么是", "含义", "定义", "原理", "作用", "为什么", "区别", "优点", "缺点", "场景", "how", "why", "what is", "difference", "principle", "overview"},
	},
	{
		label:    "architecture_or_flow_question",
		mode:     SearchModeSemantic,
		weight:   3,
		contains: []string{"架构", "流程", "链路", "主链路", "步骤", "分几步", "整体", "原始流程", "代表什么", "说明什么"},
	},
}

func AnalyzeSearchMode(request Request) SearchModeDecision {
	requestedMode := strings.TrimSpace(strings.ToLower(request.SearchMode))
	switch requestedMode {
	case SearchModeSemantic, SearchModeKeyword, SearchModeHybrid:
		return SearchModeDecision{
			RequestedMode: requestedMode,
			ResolvedMode:  requestedMode,
			Source:        modeSourceExplicit,
			Reason:        "explicit search mode requested",
			Signals:       []string{"request.search_mode"},
		}
	}

	query := normalizeDecisionQuery(request.Query)
	if query == "" {
		return SearchModeDecision{
			RequestedMode: requestedMode,
			ResolvedMode:  SearchModeSemantic,
			Source:        modeSourceAuto,
			Reason:        "empty query fallback",
			Signals:       []string{"empty_query"},
		}
	}

	modeScores := map[string]int{
		SearchModeSemantic: 0,
		SearchModeKeyword:  0,
		SearchModeHybrid:   0,
	}
	modeSignals := map[string][]string{
		SearchModeSemantic: {},
		SearchModeKeyword:  {},
		SearchModeHybrid:   {},
	}
	for _, pattern := range searchModePatterns {
		if matchesAnyToken(query, pattern.contains) {
			modeScores[pattern.mode] += pattern.weight
			modeSignals[pattern.mode] = append(modeSignals[pattern.mode], pattern.label)
		}
	}

	if looksLikeNaturalQuestion(query) && modeScores[SearchModeHybrid] == 0 && modeScores[SearchModeKeyword] == 0 {
		modeScores[SearchModeSemantic] += 2
		modeSignals[SearchModeSemantic] = append(modeSignals[SearchModeSemantic], "natural_language_question")
	}
	if looksLikeCodeSymbol(query) {
		modeScores[SearchModeHybrid] += 4
		modeSignals[SearchModeHybrid] = append(modeSignals[SearchModeHybrid], "code_symbol_shape")
	}
	if hasExactLookupShape(query) {
		modeScores[SearchModeKeyword] += 2
		modeSignals[SearchModeKeyword] = append(modeSignals[SearchModeKeyword], "exact_lookup_shape")
	}
	if looksLikeResourceIDLookup(query) {
		modeScores[SearchModeKeyword] += 4
		modeSignals[SearchModeKeyword] = append(modeSignals[SearchModeKeyword], "resource_id_lookup")
	}
	if looksLikeIdentifierLookup(query) {
		modeScores[SearchModeKeyword] += 3
		modeSignals[SearchModeKeyword] = append(modeSignals[SearchModeKeyword], "identifier_lookup")
	}
	if looksLikeFileNameLookup(query) {
		modeScores[SearchModeKeyword] += 10
		modeSignals[SearchModeKeyword] = append(modeSignals[SearchModeKeyword], "file_name_lookup")
	}
	if looksLikeSectionLookup(query) {
		modeScores[SearchModeKeyword] += 5
		modeSignals[SearchModeKeyword] = append(modeSignals[SearchModeKeyword], "section_lookup")
	}

	resolvedMode, reason, signals := pickResolvedMode(modeScores, modeSignals)
	if len(signals) == 0 {
		signals = []string{"semantic_default"}
	}

	return SearchModeDecision{
		RequestedMode: requestedMode,
		ResolvedMode:  resolvedMode,
		Source:        modeSourceAuto,
		Reason:        reason,
		Signals:       append([]string(nil), signals...),
	}
}

func normalizeDecisionQuery(query string) string {
	return strings.TrimSpace(strings.ToLower(query))
}

func matchesAnyToken(query string, tokens []string) bool {
	for _, token := range tokens {
		if strings.Contains(query, token) {
			return true
		}
	}
	return false
}

func looksLikeNaturalQuestion(query string) bool {
	return strings.Contains(query, "吗") ||
		strings.Contains(query, "？") ||
		strings.Contains(query, "?") ||
		strings.HasPrefix(query, "如何") ||
		strings.HasPrefix(query, "怎么") ||
		strings.HasPrefix(query, "what") ||
		strings.HasPrefix(query, "why") ||
		strings.HasPrefix(query, "how")
}

func hasExactLookupShape(query string) bool {
	if strings.Count(query, " ") >= 6 {
		return false
	}
	return strings.Contains(query, "\"") ||
		strings.Contains(query, "'") ||
		strings.Contains(query, "《") ||
		strings.Contains(query, "》")
}

func looksLikeCodeSymbol(query string) bool {
	return strings.Contains(query, ".") &&
		(strings.Contains(query, "service") ||
			strings.Contains(query, "handler") ||
			strings.Contains(query, "controller") ||
			strings.Contains(query, "runner") ||
			strings.Contains(query, "workflow") ||
			strings.Contains(query, "stage"))
}

func looksLikeResourceIDLookup(query string) bool {
	return (strings.Contains(query, "document ") ||
		strings.Contains(query, "task ") ||
		strings.Contains(query, "任务") ||
		strings.Contains(query, "task-") ||
		strings.Contains(query, "doc-") ||
		strings.Contains(query, "trace-")) &&
		(strings.Contains(query, "查找") ||
			strings.Contains(query, "查询") ||
			strings.Contains(query, "日志") ||
			strings.Contains(query, "节点") ||
			strings.Contains(query, "log") ||
			strings.Contains(query, "chunk"))
}

func looksLikeIdentifierLookup(query string) bool {
	lookupIntent := matchesAnyToken(query, []string{
		"查找", "搜索", "定位", "匹配", "包含", "出现", "文件名", "类名", "方法名", "函数名", "节点名", "字段名", "标题",
	})
	if !lookupIntent {
		return false
	}

	return hasIdentifierShape(query)
}

func looksLikeFileNameLookup(query string) bool {
	if !matchesAnyToken(query, []string{"文件名", "文件", "实现"}) {
		return false
	}
	return matchesAnyToken(query, []string{
		".go", ".java", ".py", ".sql", ".yaml", ".yml", ".json", ".md",
	})
}

func looksLikeSectionLookup(query string) bool {
	if !matchesAnyToken(query, []string{"章节", "小节", "段落", "第一章", "第二章", "第三章", "概述", "section"}) {
		return false
	}
	return matchesAnyToken(query, []string{"查找", "搜索", "定位", "包含", "标题", "讲了什么", "内容", "实现"})
}

func hasIdentifierShape(query string) bool {
	if matchesAnyToken(query, []string{
		".go", ".java", ".py", ".sql", ".yaml", ".yml", ".json", ".md",
		"_", "-", "/", "\\", "::", "->",
	}) {
		return true
	}

	letterRun := 0
	for _, r := range query {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			letterRun++
			if letterRun >= 12 {
				return true
			}
			continue
		}
		letterRun = 0
	}
	return false
}

func pickResolvedMode(scores map[string]int, signals map[string][]string) (string, string, []string) {
	hybridScore := scores[SearchModeHybrid]
	keywordScore := scores[SearchModeKeyword]
	semanticScore := scores[SearchModeSemantic]

	switch {
	case hybridScore >= 4 && hybridScore >= keywordScore && hybridScore >= semanticScore:
		return SearchModeHybrid, "query contains technical locator or failure signals", signals[SearchModeHybrid]
	case keywordScore >= 4 && keywordScore > hybridScore:
		return SearchModeKeyword, "query shows exact lookup intent", signals[SearchModeKeyword]
	case semanticScore > 0:
		return SearchModeSemantic, "query is primarily conceptual or natural language", signals[SearchModeSemantic]
	case keywordScore > 0:
		return SearchModeKeyword, "query slightly favors exact lookup", signals[SearchModeKeyword]
	default:
		return SearchModeSemantic, "default semantic fallback", []string{"semantic_default"}
	}
}
