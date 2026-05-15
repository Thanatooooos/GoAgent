package tool

import (
	"fmt"
	"strings"
)

func renderWebSearchContextCompat(result Result) string {
	view, ok := ViewWebSearchResult(result)
	if !ok || len(view.Results) == 0 {
		return ""
	}

	lines := make([]string, 0, len(view.Results))
	for idx, item := range view.Results {
		title := strings.TrimSpace(item.Title)
		u := strings.TrimSpace(item.URL)
		snippet := strings.TrimSpace(item.Snippet)
		if title == "" && u == "" && snippet == "" {
			continue
		}

		meta := make([]string, 0, 3)
		if domain := strings.TrimSpace(item.Domain); domain != "" {
			meta = append(meta, domain)
		}
		if policy := strings.TrimSpace(item.Policy); policy != "" {
			meta = append(meta, "policy="+policy)
		}
		if sourceType := strings.TrimSpace(item.SourceType); sourceType != "" {
			meta = append(meta, "type="+sourceType)
		}

		switch {
		case len(meta) > 0:
			lines = append(lines, fmt.Sprintf("%d. %s (%s) [%s]: %s", idx+1, title, u, strings.Join(meta, ", "), snippet))
		case snippet != "":
			lines = append(lines, fmt.Sprintf("%d. %s (%s): %s", idx+1, title, u, snippet))
		default:
			lines = append(lines, fmt.Sprintf("%d. %s (%s)", idx+1, title, u))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Search results:\n" + strings.Join(lines, "\n")
}

func renderWebFetchContextCompat(result Result) string {
	view, ok := ViewWebFetchResult(result)
	if !ok {
		return ""
	}
	combined := strings.TrimSpace(view.ReadableText())
	if combined == "" {
		return ""
	}
	return "Fetched web content:\n" + truncateRenderedContextCompat(combined)
}

func renderExternalEvidenceContextCompat(result Result) string {
	view, ok := ViewExternalEvidenceWorkflowResult(result)
	if !ok {
		return ""
	}

	parts := make([]string, 0, 4)
	if searchDetail := renderWebSearchContextCompat(Result{Name: "web_search", Data: result.Data}); searchDetail != "" {
		parts = append(parts, searchDetail)
	}
	if combined := strings.TrimSpace(view.Fetch.ReadableText()); combined != "" {
		parts = append(parts, "Fetched web content:\n"+truncateRenderedContextCompat(combined))
	}
	if len(view.SourceReview.SelectedSources) > 0 {
		lines := make([]string, 0, len(view.SourceReview.SelectedSources))
		for _, item := range view.SourceReview.SelectedSources {
			meta := make([]string, 0, 2)
			if policy := strings.TrimSpace(item.Policy); policy != "" {
				meta = append(meta, "policy="+policy)
			}
			if sourceType := strings.TrimSpace(item.SourceType); sourceType != "" {
				meta = append(meta, "type="+sourceType)
			}
			if len(meta) > 0 {
				lines = append(lines, fmt.Sprintf("- %s (%s) [%s]", item.Title, item.URL, strings.Join(meta, ", ")))
				continue
			}
			lines = append(lines, fmt.Sprintf("- %s (%s)", item.Title, item.URL))
		}
		parts = append(parts, "Selected sources:\n"+strings.Join(lines, "\n"))
	}
	if readiness := strings.TrimSpace(view.Readiness); readiness != "" {
		line := fmt.Sprintf("Readiness: %s (confidence=%.2f)", readiness, view.ReadinessConfidence)
		if reasoning := strings.TrimSpace(view.ReadinessReasoning); reasoning != "" {
			line += "\n" + reasoning
		}
		if quality := strings.TrimSpace(view.Quality.Quality); quality != "" {
			line += fmt.Sprintf("\nQuality: %s (confidence=%.2f)", quality, view.Quality.Confidence)
		}
		if qualityReasoning := strings.TrimSpace(view.Quality.Reasoning); qualityReasoning != "" {
			line += "\n" + qualityReasoning
		}
		if strategy := strings.TrimSpace(view.AnswerStrategy); strategy != "" {
			line += "\nStrategy: " + strategy
		}
		parts = append(parts, line)
	}
	return strings.Join(parts, "\n\n")
}

func renderTraceNodeQueryContextCompat(result Result) string {
	view, ok := ViewTraceNodeQueryResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, len(view.Nodes)+2)
	if view.ErrorMessage != "" {
		lines = append(lines, "Trace error: "+view.ErrorMessage)
	}
	for idx, node := range view.Nodes {
		label := firstNonEmpty(strings.TrimSpace(node.NodeName), strings.TrimSpace(node.NodeID))
		if label == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%d. %s type=%s status=%s", idx+1, label, strings.TrimSpace(node.NodeType), strings.TrimSpace(node.Status)))
	}
	if len(lines) == 0 {
		return ""
	}
	return "Trace nodes:\n" + strings.Join(lines, "\n")
}

func renderDocumentRootCauseDiagnosisContextCompat(result Result) string {
	view, ok := ViewDocumentRootCauseDiagnosisResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, 5)
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Conclusion: "+conclusion)
	}
	if confidence := strings.TrimSpace(view.Confidence); confidence != "" {
		lines = append(lines, "Confidence: "+confidence)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "Diagnosis depth: "+depth)
	}
	if view.ChainLength > 0 {
		lines = append(lines, fmt.Sprintf("Chain length: %d", view.ChainLength))
	}
	if view.LatestTaskID != "" || view.LatestNodeID != "" {
		lines = append(lines, fmt.Sprintf("Latest task/node: %s / %s", firstNonEmpty(view.LatestTaskID, "-"), firstNonEmpty(view.LatestNodeID, "-")))
	}
	return strings.Join(lines, "\n")
}

func renderDocumentDiagnoseWithSearchContextCompat(result Result) string {
	view, ok := ViewDocumentDiagnoseWithSearchResult(result)
	if !ok {
		return ""
	}

	lines := make([]string, 0, 4)
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "Conclusion: "+conclusion)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "Diagnosis depth: "+depth)
	}
	if query := strings.TrimSpace(view.SearchQuery); query != "" {
		lines = append(lines, "Search query: "+query)
	}
	if view.SearchResultCount > 0 {
		lines = append(lines, fmt.Sprintf("Search results: %d", view.SearchResultCount))
	}
	return strings.Join(lines, "\n")
}

func buildDocumentRootCauseDiagnosisGuidanceNotes(result Result) []GuidanceNote {
	view, ok := ViewDocumentRootCauseDiagnosisResult(result)
	if !ok {
		return nil
	}

	lines := []string{
		"这是一次图诊断结果。请优先用中文，按[结论 / 证据边界 / 建议]的顺序组织回答。",
	}
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "当前结论："+conclusion)
	}
	if confidence := strings.TrimSpace(view.Confidence); confidence != "" {
		lines = append(lines, "当前置信度："+confidence)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "诊断深度："+depth)
		switch depth {
		case "node_level":
			lines = append(lines, "这已经是节点级证据，可以直接回答，但不要夸大超出证据边界的根因。")
		case "task_level":
			lines = append(lines, "当前只有任务级证据，不要编造成已经确认的节点级错误。")
		}
	}
	if view.LatestTaskID != "" || view.LatestNodeID != "" {
		lines = append(lines, fmt.Sprintf("可引用的最近执行线索：task=%s, node=%s。", firstNonEmpty(view.LatestTaskID, "-"), firstNonEmpty(view.LatestNodeID, "-")))
	}
	return []GuidanceNote{{Text: strings.Join(lines, "\n")}}
}

func buildDocumentDiagnoseWithSearchGuidanceNotes(result Result, allResults []Result) []GuidanceNote {
	view, ok := ViewDocumentDiagnoseWithSearchResult(result)
	if !ok {
		return nil
	}

	lines := []string{
		"这是一次诊断 + 搜索结果。请优先用中文，先给出系统内诊断结论，再说明外部搜索只提供补充参考。",
	}
	if conclusion := strings.TrimSpace(view.Conclusion); conclusion != "" {
		lines = append(lines, "当前诊断结论："+conclusion)
	}
	if depth := strings.TrimSpace(view.DiagnosisDepth); depth != "" {
		lines = append(lines, "诊断深度："+depth)
	}
	if query := strings.TrimSpace(view.SearchQuery); query != "" {
		lines = append(lines, "外部搜索查询："+query)
	}
	if view.SearchResultCount > 0 {
		lines = append(lines, fmt.Sprintf("当前命中的搜索结果数量：%d。", view.SearchResultCount))
	}
	if hasFetchedWebEvidence(allResults) {
		lines = append(lines, "引用外部方案时，只能基于已经抓取到的网页正文内容，并明确标注来源 URL。")
	} else {
		lines = append(lines, "当前只有搜索结果摘要，没有抓取到网页正文证据时，不要编造具体修复方案；只能把外部结果作为排查方向或参考。")
	}
	return []GuidanceNote{{Text: strings.Join(lines, "\n")}}
}

func hasFetchedWebEvidence(results []Result) bool {
	for _, result := range results {
		switch strings.TrimSpace(result.Name) {
		case "web_fetch":
			view, ok := ViewWebFetchResult(result)
			if ok && strings.TrimSpace(view.ReadableText()) != "" {
				return true
			}
		case "external_evidence_workflow":
			view, ok := ViewExternalEvidenceWorkflowResult(result)
			if ok && strings.TrimSpace(view.Fetch.ReadableText()) != "" {
				return true
			}
		}
	}
	return false
}

func truncateRenderedContextCompat(raw string) string {
	const maxRenderedToolContextLen = 12000

	raw = strings.TrimSpace(raw)
	if len(raw) <= maxRenderedToolContextLen {
		return raw
	}
	return strings.TrimSpace(raw[:maxRenderedToolContextLen-3]) + "..."
}

func renderGuidanceNotes(notes []GuidanceNote) string {
	if len(notes) == 0 {
		return ""
	}
	parts := make([]string, 0, len(notes))
	for _, note := range notes {
		text := strings.TrimSpace(note.Text)
		if text == "" {
			continue
		}
		parts = append(parts, text)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}
