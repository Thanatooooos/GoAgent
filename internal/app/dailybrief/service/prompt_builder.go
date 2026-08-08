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
		"\n10. 用户订阅的每个 topic 栏目最多输出 %d 条 item；尽量覆盖各订阅 topic，不要只写单一栏目。",
		maxItemsPerTopic,
	)
}

func BuildBriefGenerationPrompt(briefDate string, subscribedTopics []string, candidates []domain.Candidate, maxItemsPerTopic int) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "简报日期：%s\n", strings.TrimSpace(briefDate))
	if len(subscribedTopics) > 0 {
		builder.WriteString("用户订阅 topic：")
		builder.WriteString(strings.Join(subscribedTopics, ", "))
		builder.WriteByte('\n')
	}
	if maxItemsPerTopic > 0 {
		fmt.Fprintf(&builder, "每个订阅 topic 最多输出 %d 条 item。\n", maxItemsPerTopic)
	}
	fmt.Fprintf(&builder, "候选条目（%d 条）：\n", len(candidates))
	for index, candidate := range candidates {
		publishedAt := ""
		if !candidate.PublishedAt.IsZero() {
			publishedAt = candidate.PublishedAt.UTC().Format(time.RFC3339)
		}
		fmt.Fprintf(
			&builder,
			"%d. title=%q url=%q source=%q topic=%q publishedAt=%s summary=%q\n",
			index+1,
			candidate.Title,
			candidate.URL,
			candidate.Source,
			candidate.Topic,
			publishedAt,
			candidate.SummarySnippet,
		)
	}
	builder.WriteString("\n栏目中文标题参考：\n")
	for _, key := range domain.LeafTopicKeys() {
		fmt.Fprintf(&builder, "- %s: %s\n", key, domain.TopicBreadcrumb(key))
	}
	builder.WriteString("\n写作要求：每条 item 的 summary 与 whyItMatters 都要写足篇幅，不要过度压缩。")
	if maxItemsPerTopic > 0 && len(subscribedTopics) > 0 {
		builder.WriteString(" 为每个订阅 topic 各写最多 ")
		fmt.Fprintf(&builder, "%d", maxItemsPerTopic)
		builder.WriteString(" 条，确保多栏目都有内容。")
	}
	builder.WriteString("\n")
	builder.WriteString("\n返回 JSON 示例（文案请用简体中文）：\n")
	builder.WriteString(`{"headline":"...","topSummary":"...","sections":[{"key":"tech.ai.models","title":"模型发布","items":[{"title":"...","summary":"第一句交代事件。第二句补充关键主体或数据。第三句说明进展或边界。","whyItMatters":"第一句说明直接影响。第二句给出对从业者或行业的启示。","url":"...","source":"openai-blog","topic":"tech.ai.models"}]}]}`)
	return builder.String()
}
