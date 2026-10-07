package service

import (
	"fmt"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

const briefGenerationSystemPrompt = `你是一位资深科技资讯编辑，负责撰写信息充实的每日 AI 与技术简报。

规则：
1. 仅使用提供的候选条目作为事实依据，可翻译或概括英文原文，但不得捏造事实。
2. 不得编造 URL、source 或 topic；source 与 topic 必须从对应候选条目原样复制，禁止改写或拼接。
3. 面向中文读者，所有面向用户的文案必须使用简体中文，包括 headline、topSummary、各 section 的 title，以及每条 item 的 title、summary、whyItMatters。
4. 内容要“写得丰富”，不要只写一句话带过；在事实范围内补充背景、关键数字、涉及主体与进展阶段。
5. 按 topic 分组到对应 section；section 的 key 使用英文 topic 键，title 使用中文栏目名。
6. 仅返回严格 JSON，顶层键为：headline、topSummary、sections。
7. 每个 section 必须包含 key、title、items。
8. 每个 item 必须包含 title、summary、whyItMatters、url、source、topic。
9. 篇幅要求（中文）：
   - headline：1 句，20-45 字，概括当日最重要趋势。
   - topSummary：3-5 句，120-220 字，串联当日要闻与共同主题。
   - item.title：1 句标题，15-35 字，信息具体、避免空泛。
   - item.summary：2-4 句，80-160 字，写清发生了什么、涉及谁、有何关键细节。
   - item.whyItMatters：2-3 句，60-120 字，说明对开发者、产品团队或行业的实际影响与可行动启示。`

func BuildBriefGenerationSystemPrompt(maxItemsPerTopic int) string {
	if maxItemsPerTopic <= 0 {
		return briefGenerationSystemPrompt
	}
	return briefGenerationSystemPrompt + fmt.Sprintf(
		briefGenerationTopicQuotaTemplate,
		maxItemsPerTopic,
	)
}

func BuildBriefGenerationPrompt(briefDate string, subscribedTopics []string, candidates []domain.Candidate, maxItemsPerTopic int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, briefDateContextTemplate, strings.TrimSpace(briefDate))
	if len(subscribedTopics) > 0 {
		builder.WriteString(briefSubscribedTopicsPrefix)
		builder.WriteString(strings.Join(subscribedTopics, ", "))
		builder.WriteByte('\n')
	}
	if maxItemsPerTopic > 0 {
		fmt.Fprintf(&builder, briefTopicQuotaTemplate, maxItemsPerTopic)
	}
	fmt.Fprintf(&builder, briefCandidatesHeaderTemplate, len(candidates))
	for index, candidate := range candidates {
		publishedAt := ""
		if !candidate.PublishedAt.IsZero() {
			publishedAt = candidate.PublishedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(
			&builder,
			briefCandidateContextTemplate,
			index+1,
			candidate.Title,
			candidate.URL,
			candidate.Source,
			candidate.Topic,
			publishedAt,
			candidate.SummarySnippet,
		)
	}
	builder.WriteString(briefTopicTitlesHeader)
	for _, key := range domain.LeafTopicKeys() {
		fmt.Fprintf(&builder, briefTopicTitleTemplate, key, domain.TopicBreadcrumb(key))
	}
	builder.WriteString(briefWritingInstruction)
	if maxItemsPerTopic > 0 && len(subscribedTopics) > 0 {
		builder.WriteString(briefTopicCoveragePrefix)
		fmt.Fprintf(&builder, "%d", maxItemsPerTopic)
		builder.WriteString(briefTopicCoverageSuffix)
	}
	builder.WriteString("\n")
	builder.WriteString(briefJSONExampleHeader)
	builder.WriteString(briefJSONExample)
	return builder.String()
}
