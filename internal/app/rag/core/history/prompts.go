package history

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	structuredMemorySummaryRequest = "现在请直接返回结构化工作记忆 JSON。"

	structuredSummaryFactInstruction = "\n\u8865\u5145\u89c4\u5219：\u5982\u679c\u67d0\u9879\u7ed3\u8bba\u53ea\u6765\u81ea\u52a9\u624b\u5efa\u8bae\u3001\u793a\u4f8b\u4ee3\u7801\u6216\u901a\u7528\u65b9\u6848\u8bf4\u660e，\u800c\u6ca1\u6709\u88ab\u7528\u6237\u786e\u8ba4\u6216\u5b9e\u9645\u843d\u5730，\u4e0d\u8981\u5199\u6210 established_facts\u3002\n"

	previousStructuredSummaryHeader = "\n上一次结构化摘要 JSON：\n"

	previousSummaryHeader = "\n上一轮压缩摘要：\n"

	recentSummaryMessagesHeader = "\n最近消息：\n"

	conversationSummaryContextPrefix = "对话摘要："

	compressionPreviousSummaryHeader = "\n\n上一次压缩的摘要为：\n"

	compressionMergeInstruction = "\n\n请结合之前的摘要，将以下新对话内容合并到摘要中。"

	compressionNewMessagesHeader = "\n\n## 新对话\n"
)
