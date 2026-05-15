package runtime

import (
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

func BuildAnswerGuidance(results []Result) string {
	if diagnosis, ok := selectDiagnosisResult(results); ok {
		return buildDiagnosisGuidance(diagnosis, results)
	}
	if externalEvidence, ok := selectExternalEvidenceResult(results); ok {
		return buildExternalEvidenceGuidance(results, externalEvidence)
	}
	if webResults := selectWebResults(results); len(webResults) > 0 {
		return buildWebSearchGuidance(results, webResults)
	}
	return ""
}

func selectDiagnosisResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		switch strings.TrimSpace(results[idx].Name) {
		case "document_ingestion_diagnose", "task_ingestion_diagnose", "trace_retrieval_diagnose":
			return results[idx], true
		}
	}
	return Result{}, false
}

func selectExternalEvidenceResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(results[idx].Name) == "external_evidence_workflow" {
			return results[idx], true
		}
	}
	return Result{}, false
}

func buildDiagnosisGuidance(result Result, allResults []Result) string {
	view, _ := systemmod.ViewDiagnosisResult(result)
	conclusion := strings.TrimSpace(view.Conclusion)
	confidence := strings.TrimSpace(view.Confidence)
	facts := append([]string(nil), view.Facts...)
	inferences := append([]string(nil), view.Inferences...)
	riskHints := append([]string(nil), view.RiskHints...)
	nextActions := append([]string(nil), view.NextActions...)

	conclusion, confidence, facts, inferences, riskHints = enrichDiagnosisWithDeeperEvidence(
		result, allResults, conclusion, confidence, facts, inferences, riskHints,
	)

	var builder strings.Builder
	builder.WriteString("这是一类诊断回答。请优先使用中文，并按「结论 / 证据 / 建议」的顺序组织回答。")
	builder.WriteString("\n结论部分只陈述当前最可能的问题，并明确写出置信度。")
	if conclusion != "" {
		builder.WriteString("\n当前建议结论：")
		builder.WriteString(conclusion)
		builder.WriteString("。")
	}
	if confidence != "" {
		builder.WriteString("\n当前置信度：")
		builder.WriteString(confidence)
		builder.WriteString("。")
	}
	if len(facts) > 0 {
		builder.WriteString("\n证据部分优先引用 3-5 条最关键事实，不要把推断写成事实：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(facts, "\n- "))
	}
	if len(inferences) > 0 {
		builder.WriteString("\n如果需要补充推断，请明确标注「推断」或「可能原因」，不要伪装成已确认事实：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(inferences, "\n- "))
	}
	if len(riskHints) > 0 {
		builder.WriteString("\n如果证据仍有边界或风险，请显式提醒用户：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(riskHints, "\n- "))
	}
	if len(nextActions) > 0 {
		builder.WriteString("\n建议部分给出可执行的下一步检查项：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(nextActions, "\n- "))
	}
	builder.WriteString("\n不要只复述工具调用过程。若后续 task/node 查询的结果与前面的文档级诊断结论不一致（例如诊断说 failed 但 task/node 实际 running），必须以 task/node 的实际状态为准，并在回答中显式说明状态不一致。没有更深的节点级证据时，不要臆造错误原因。")
	return builder.String()
}

func enrichDiagnosisWithDeeperEvidence(
	base Result, allResults []Result,
	conclusion string, confidence string, facts []string, inferences []string, riskHints []string,
) (string, string, []string, []string, []string) {
	if strings.TrimSpace(base.Name) == "trace_retrieval_diagnose" {
		return conclusion, confidence, facts, inferences, riskHints
	}

	taskID := strings.TrimSpace(base.GetString("taskId"))
	if taskID == "" {
		taskID = strings.TrimSpace(base.GetString("latestTaskId"))
	}
	if taskID == "" {
		return conclusion, confidence, facts, inferences, riskHints
	}

	taskResult, hasTask := findLatestTaskResult(allResults, taskID)
	nodeResult, hasNode := findLatestTaskNodeDetail(allResults, taskID)

	conclusionLower := strings.ToLower(conclusion)
	diagnoseSaysFailed := strings.Contains(conclusionLower, "failed") || strings.Contains(conclusionLower, "失败")

	// Collect real-time status from deeper evidence.
	taskStatus := strings.ToLower(strings.TrimSpace(taskResult.GetString("status")))
	nodeStatus := strings.ToLower(strings.TrimSpace(nodeResult.GetString("status")))

	// Conflict: diagnose says failed but task or node is still running.
	if diagnoseSaysFailed && (taskStatus == "running" || nodeStatus == "running") {
		conclusion, confidence, facts, inferences, riskHints = resolveStatusConflict(
			taskResult, nodeResult, hasTask, hasNode, taskID, conclusion, confidence, facts, inferences, riskHints,
		)
		return conclusion, confidence, facts, inferences, riskHints
	}

	// Node-level evidence is mandatory beyond this point.
	if !hasNode {
		return conclusion, confidence, facts, inferences, riskHints
	}

	nodeID := strings.TrimSpace(nodeResult.GetString("nodeId"))
	nodeError := strings.TrimSpace(nodeResult.GetString("errorMessage"))
	nodeOrder := nodeResult.GetInt("nodeOrder")
	durationMs := nodeResult.GetInt("durationMs")
	if nodeID == "" {
		return conclusion, confidence, facts, inferences, riskHints
	}

	if nodeStatus == "failed" {
		conclusion = fmt.Sprintf("文档导入失败，失败发生在 %s 节点", nodeID)
		if nodeError != "" {
			conclusion = fmt.Sprintf("%s，节点报错为 %q", conclusion, nodeError)
		}
		confidence = "high"
	}

	extraFacts := make([]string, 0, 3)
	if nodeOrder > 0 {
		extraFacts = append(extraFacts, fmt.Sprintf("对应的 ingestion 节点是第 %d 个节点 %s，状态为%s。", nodeOrder, nodeID, renderStatusLabel(nodeStatus)))
	} else {
		extraFacts = append(extraFacts, fmt.Sprintf("对应的 ingestion 节点是 %s，状态为%s。", nodeID, renderStatusLabel(nodeStatus)))
	}
	if nodeError != "" {
		extraFacts = append(extraFacts, fmt.Sprintf("该节点的具体错误是 %q。", nodeError))
	}
	if durationMs > 0 {
		extraFacts = append(extraFacts, fmt.Sprintf("该节点持续时间约为 %dms。", durationMs))
	}
	facts = prependUniqueFacts(facts, extraFacts...)

	if nodeStatus == "failed" {
		inferences = []string{
			fmt.Sprintf("推断：前序阶段大概率已经完成，失败更集中发生在 %s 节点对应的索引或下游持久化环节。", nodeID),
		}
	}

	return conclusion, confidence, facts, inferences, riskHints
}

func resolveStatusConflict(
	taskResult Result, nodeResult Result, hasTask bool, hasNode bool, taskID string,
	conclusion string, confidence string, facts []string, inferences []string, riskHints []string,
) (string, string, []string, []string, []string) {
	diagnoseConclusion := conclusion

	// Build the corrected conclusion from the deepest available evidence.
	if hasNode {
		nodeID := strings.TrimSpace(nodeResult.GetString("nodeId"))
		conclusion = fmt.Sprintf("文档仍在处理中，当前运行到 %s 节点", nodeID)
	} else {
		conclusion = fmt.Sprintf("文档仍在处理中，关联任务 %s 仍在运行", taskID)
	}
	confidence = "high"

	// Facts: surface the conflict explicitly.
	extraFacts := make([]string, 0, 3)
	if hasNode {
		nodeID := strings.TrimSpace(nodeResult.GetString("nodeId"))
		extraFacts = append(extraFacts, fmt.Sprintf("更接近实时执行链路的节点查询显示 %s 仍在运行中。", nodeID))
	} else {
		extraFacts = append(extraFacts, fmt.Sprintf("更接近实时执行链路的任务查询显示 %s 仍在运行中。", taskID))
	}
	extraFacts = append(extraFacts, fmt.Sprintf("文档级诊断结论（%s）与任务/节点实际状态不一致，可能因状态异步更新滞后。", strings.TrimSpace(diagnoseConclusion)))
	facts = prependUniqueFacts(facts, extraFacts...)

	inferences = []string{
		"推断：文档级状态可能因异步更新延迟而滞后，应以更接近执行链路的任务/节点状态为准。",
	}

	riskHints = prependUniqueFacts(riskHints,
		"状态不一致风险：文档级状态与任务/节点实际状态存在冲突，已优先采用任务/节点状态。建议检查文档状态同步机制。",
	)

	return conclusion, confidence, facts, inferences, riskHints
}

func findLatestTaskNodeDetail(results []Result, taskID string) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		result := results[idx]
		if strings.TrimSpace(result.Name) != "ingestion_task_node_query" {
			continue
		}
		if strings.TrimSpace(result.GetString("taskId")) != taskID {
			continue
		}
		if strings.TrimSpace(result.GetString("nodeId")) == "" {
			continue
		}
		return result, true
	}
	return Result{}, false
}

func findLatestTaskResult(results []Result, taskID string) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		result := results[idx]
		if strings.TrimSpace(result.Name) != "ingestion_task_query" {
			continue
		}
		if strings.TrimSpace(result.GetString("taskId")) != taskID {
			continue
		}
		return result, true
	}
	return Result{}, false
}

func prependUniqueFacts(existing []string, incoming ...string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(existing)+len(incoming))
	for _, fact := range incoming {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		items = append(items, fact)
	}
	for _, fact := range existing {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		items = append(items, fact)
	}
	return items
}

func renderStatusLabel(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed":
		return "失败"
	case "running":
		return "运行中"
	case "success":
		return "成功"
	case "pending":
		return "待处理"
	default:
		return strings.TrimSpace(status)
	}
}

func readDataString(data map[string]any, key string) string {
	if len(data) == 0 {
		return ""
	}
	value, ok := data[key]
	if !ok || value == nil {
		return ""
	}
	typed, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(typed)
}

func readDataStringSlice(data map[string]any, key string) []string {
	if len(data) == 0 {
		return []string{}
	}
	value, ok := data[key]
	if !ok || value == nil {
		return []string{}
	}
	switch typed := value.(type) {
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := fmt.Sprintf("%v", item)
			if trimmed := strings.TrimSpace(text); trimmed != "" {
				items = append(items, trimmed)
			}
		}
		return items
	default:
		return []string{}
	}
}

func preferDataStringSlice(data map[string]any, primary string, fallback string) []string {
	items := readDataStringSlice(data, primary)
	if len(items) > 0 {
		return items
	}
	return readDataStringSlice(data, fallback)
}

func buildExternalEvidenceGuidance(allResults []Result, result Result) string {
	view, ok := webmod.ViewExternalEvidenceWorkflowResult(result)
	if !ok {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("这是一个需要结合外部网页证据的回答。请优先使用中文，并按“本地证据 / 外部来源质量 / 结论 / 局限与引用”的顺序组织回答。")
	builder.WriteString("\n先交代本地知识库里已经确认的内容，再说明哪些结论来自外部网页。不要把外部信息伪装成本地已知事实。")
	builder.WriteString("\n必须明确标注关键结论对应的 URL；如果来源之间存在差异或来源质量有限，要显式说清楚。")

	localEvidence := collectLocalEvidenceSummaries(allResults)
	if len(localEvidence) > 0 {
		builder.WriteString("\n\n本地/知识库侧已知证据：")
		for _, item := range localEvidence {
			builder.WriteString("\n- ")
			builder.WriteString(item)
		}
	}

	builder.WriteString("\n\n外部来源质量要求：")
	if view.SourceCoverage != "" {
		builder.WriteString("\n- 本次选取的来源覆盖类型：")
		builder.WriteString(view.SourceCoverage)
	}
	if view.Quality.Quality != "" {
		builder.WriteString(fmt.Sprintf("\n- 当前外部证据质量：%s（confidence=%.2f）。", view.Quality.Quality, view.Quality.Confidence))
	}
	if view.Quality.Reasoning != "" {
		builder.WriteString("\n- 质量判断依据：")
		builder.WriteString(view.Quality.Reasoning)
	}
	if view.Quality.SourceDiversity != "" || view.Quality.Corroboration != "" {
		builder.WriteString(fmt.Sprintf("\n- 来源多样性=%s，交叉印证=%s。", firstNonEmpty(view.Quality.SourceDiversity, "unknown"), firstNonEmpty(view.Quality.Corroboration, "unknown")))
	}
	if len(view.Quality.Notes) > 0 {
		builder.WriteString("\n- 质量边界：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(view.Quality.Notes, "\n- "))
	}

	if view.Readiness != "" {
		builder.WriteString(fmt.Sprintf("\n\n当前回答可用性：%s（confidence=%.2f）。", view.Readiness, view.ReadinessConfidence))
	}
	if view.ReadinessReasoning != "" {
		builder.WriteString("\n可用性判断：")
		builder.WriteString(view.ReadinessReasoning)
	}
	if view.AnswerStrategy != "" {
		builder.WriteString("\n作答策略：")
		builder.WriteString(view.AnswerStrategy)
	}
	if len(view.MissingInformation) > 0 {
		builder.WriteString("\n如果仍有信息缺口，请明确指出：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(view.MissingInformation, "\n- "))
	}

	sources := collectExternalEvidenceSources(view)
	if len(sources) > 0 {
		builder.WriteString("\n\n外部来源清单：")
		for _, src := range sources {
			meta := make([]string, 0, 2)
			if src.policy != "" {
				meta = append(meta, "policy="+src.policy)
			}
			if src.sourceType != "" {
				meta = append(meta, "type="+src.sourceType)
			}
			if len(meta) > 0 {
				builder.WriteString(fmt.Sprintf("\n- %s (%s) [%s]", src.title, src.url, strings.Join(meta, ", ")))
				continue
			}
			builder.WriteString(fmt.Sprintf("\n- %s (%s)", src.title, src.url))
		}
	}

	citedURLs := view.CitedURLs
	if len(citedURLs) == 0 {
		citedURLs = view.SelectedURLs
	}
	if len(citedURLs) > 0 {
		builder.WriteString("\n\n在答案结尾追加“引用来源”小节，至少列出以下 URL：")
		for _, citedURL := range citedURLs {
			builder.WriteString("\n- ")
			builder.WriteString(citedURL)
		}
	}

	builder.WriteString("\n不要只复述工具执行过程；重点是把最终结论、来源质量和剩余不确定性讲清楚。")
	return builder.String()
}

func selectWebResults(results []Result) []Result {
	webResults := make([]Result, 0)
	for idx := len(results) - 1; idx >= 0; idx-- {
		name := strings.TrimSpace(results[idx].Name)
		if name == "web_search" || name == "web_fetch" {
			webResults = append(webResults, results[idx])
		}
	}
	return webResults
}

func buildWebSearchGuidance(allResults []Result, webResults []Result) string {
	var builder strings.Builder
	builder.WriteString("这是一类需要联网搜索的回答。请优先使用中文，并按「信息来源 / 核心发现 / 局限性」的顺序组织回答。")
	builder.WriteString("\n必须明确标注每个关键信息的来源 URL，以便用户核实。")
	builder.WriteString("\n如果多个来源信息一致，可合并陈述并标注多个来源；如果信息有矛盾，必须显式指出分歧。")
	builder.WriteString("\n如果知识库中的内容与搜索结果相关，优先陈述知识库或本地证据，再把搜索结果作为补充参考。")
	builder.WriteString("\n在回答末尾添加「搜索结果来源」小节，列出所有引用的 URL 和标题。")
	builder.WriteString("\n如果信息不足以得出确定结论，请如实说明不确定性，不要臆造事实。")

	localEvidence := collectLocalEvidenceSummaries(allResults)
	if len(localEvidence) > 0 {
		builder.WriteString("\n\n本地/知识库侧已知证据：")
		for _, item := range localEvidence {
			builder.WriteString("\n- ")
			builder.WriteString(item)
		}
		builder.WriteString("\n回答时请先交代这些本地证据，再说明哪些信息来自外部网页。")
	}

	// Collect source URLs and titles for the guidance.
	sources := collectWebSources(webResults)
	if len(sources) > 0 {
		builder.WriteString("\n\n已获取的网页来源：")
		for _, src := range sources {
			meta := make([]string, 0, 2)
			if src.policy != "" {
				meta = append(meta, "policy="+src.policy)
			}
			if src.sourceType != "" {
				meta = append(meta, "type="+src.sourceType)
			}
			if len(meta) > 0 {
				builder.WriteString(fmt.Sprintf("\n- %s (%s) [%s]", src.title, src.url, strings.Join(meta, ", ")))
				continue
			}
			builder.WriteString(fmt.Sprintf("\n- %s (%s)", src.title, src.url))
		}
	}

	return builder.String()
}

type webSource struct {
	title      string
	url        string
	sourceType string
	policy     string
}

func collectWebSources(results []Result) []webSource {
	seen := map[string]int{}
	sources := make([]webSource, 0)
	for _, result := range results {
		switch strings.TrimSpace(result.Name) {
		case "web_search":
			view, ok := webmod.ViewWebSearchResult(result)
			if !ok {
				continue
			}
			for _, item := range view.Results {
				u := strings.TrimSpace(item.URL)
				if u == "" {
					continue
				}
				if idx, exists := seen[u]; exists {
					if strings.TrimSpace(sources[idx].title) == "" {
						sources[idx].title = strings.TrimSpace(item.Title)
					}
					if strings.TrimSpace(sources[idx].sourceType) == "" {
						sources[idx].sourceType = strings.TrimSpace(item.SourceType)
					}
					if strings.TrimSpace(sources[idx].policy) == "" {
						sources[idx].policy = strings.TrimSpace(item.Policy)
					}
					continue
				}
				seen[u] = len(sources)
				sources = append(sources, webSource{
					title:      strings.TrimSpace(item.Title),
					url:        u,
					sourceType: strings.TrimSpace(item.SourceType),
					policy:     strings.TrimSpace(item.Policy),
				})
			}
		case "web_fetch":
			view, ok := webmod.ViewWebFetchResult(result)
			if !ok {
				continue
			}
			for _, page := range view.Pages {
				u := strings.TrimSpace(page.URL)
				if u == "" {
					continue
				}
				if _, exists := seen[u]; exists {
					continue
				}
				seen[u] = len(sources)
				sources = append(sources, webSource{url: u})
			}
		}
	}
	return sources
}

func collectLocalEvidenceSummaries(results []Result) []string {
	items := make([]string, 0, 3)
	for _, result := range results {
		name := strings.TrimSpace(result.Name)
		if name == "" || name == "web_search" || name == "web_fetch" || name == "external_evidence_workflow" || name == "think" {
			continue
		}
		summary := strings.TrimSpace(firstNonEmpty(result.Summary, result.ErrorMessage))
		if summary == "" {
			continue
		}
		items = append(items, fmt.Sprintf("%s: %s", name, summary))
		if len(items) >= 3 {
			break
		}
	}
	return items
}

func collectExternalEvidenceSources(view webmod.ExternalEvidenceWorkflowView) []webSource {
	seen := map[string]struct{}{}
	sources := make([]webSource, 0, len(view.SourceReview.SelectedSources)+len(view.SourceReview.RejectedSources))
	appendSource := func(title, rawURL, policy, sourceType string) {
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return
		}
		if _, ok := seen[rawURL]; ok {
			return
		}
		seen[rawURL] = struct{}{}
		sources = append(sources, webSource{
			title:      strings.TrimSpace(title),
			url:        rawURL,
			policy:     strings.TrimSpace(policy),
			sourceType: strings.TrimSpace(sourceType),
		})
	}
	for _, item := range view.SourceReview.SelectedSources {
		appendSource(item.Title, item.URL, item.Policy, item.SourceType)
	}
	for _, item := range view.SourceReview.RejectedSources {
		appendSource(item.Title, item.URL, item.Policy, item.SourceType)
	}
	if len(sources) == 0 {
		for _, item := range view.Search.Results {
			appendSource(item.Title, item.URL, item.Policy, item.SourceType)
		}
	}
	return sources
}
