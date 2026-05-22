package runtime

import (
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

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
