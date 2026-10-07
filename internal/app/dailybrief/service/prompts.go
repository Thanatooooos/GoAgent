package service

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	briefRuntimeRequest = "Research and write the daily brief now."

	briefRuntimeSystemPrompt = "You write a Chinese daily technology brief. The candidate list is trusted seed evidence; use web_search and web_fetch only for up to two material gaps, not to verify every candidate. Do not invent facts. Return only strict JSON with top-level keys headline, topSummary, sections. Each section has key, title, items. Each item has title, summary, whyItMatters, url, source, topic. All reader-facing text must be Simplified Chinese. Every item must cite a real URL and a source name. topic must be one of the subscribed topic keys. Group items by topic. Do not emit Markdown or explanations outside the JSON."

	briefRuntimeFallbackRequest = "Write the daily brief using only the provided candidate list. No tools are available. Do not invent facts. Return only strict JSON with top-level keys headline, topSummary, sections. Each section has key, title, items. Each item has title, summary, whyItMatters, url, source, topic. All reader-facing text must be Simplified Chinese. Every item must cite a candidate URL and source. topic must be one of the subscribed topic keys. Group items by topic. Do not emit Markdown or explanations outside the JSON."

	briefGenerationTopicQuotaTemplate = "\n10. 用户订阅的每个 topic 栏目最多输出 %d 条 item；尽量覆盖各订阅 topic，不要只写单一栏目。"

	briefDateContextTemplate = "简报日期：%s\n"

	briefSubscribedTopicsPrefix = "用户订阅 topic："

	briefTopicQuotaTemplate = "每个订阅 topic 最多输出 %d 条 item。\n"

	briefCandidatesHeaderTemplate = "候选条目（%d 条）：\n"

	briefCandidateContextTemplate = "%d. title=%q url=%q source=%q topic=%q publishedAt=%s summary=%q\n"

	briefTopicTitlesHeader = "\n栏目中文标题参考：\n"

	briefTopicTitleTemplate = "- %s: %s\n"

	briefWritingInstruction = "\n写作要求：每条 item 的 summary 与 whyItMatters 都要写足篇幅，不要过度压缩。"

	briefTopicCoveragePrefix = " 为每个订阅 topic 各写最多 "

	briefTopicCoverageSuffix = " 条，确保多栏目都有内容。"

	briefJSONExampleHeader = "\n返回 JSON 示例（文案请用简体中文）：\n"

	briefJSONExample = `{"headline":"...","topSummary":"...","sections":[{"key":"tech.ai.models","title":"模型发布","items":[{"title":"...","summary":"第一句交代事件。第二句补充关键主体或数据。第三句说明进展或边界。","whyItMatters":"第一句说明直接影响。第二句给出对从业者或行业的启示。","url":"...","source":"openai-blog","topic":"tech.ai.models"}]}]}`
)
