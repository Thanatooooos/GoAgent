package sessionrecall

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	longMessageSummaryRequest = "请直接输出摘要正文，不要输出解释、标题或额外说明。"

	mediumMessageSummaryPromptTemplate = `你是一个长消息摘要助手。请将下面这条用户长消息压缩成一个高密度摘要，供后续多轮对话直接放入上下文。

要求：
1. 摘要使用中文。
2. 长度不超过 %d 个字符。
3. 保留核心问题、关键约束、重要错误、关键代码意图或关键事实。
4. 删除寒暄、重复表述和低价值细节。
5. 如果内容是日志/报错，优先保留错误现象、模块、异常关键词。
6. 如果内容是代码，优先保留代码用途、模块、调用关系、报错点。

原文预估 tokens：%d

原文：
%s`

	chunkMessageSummaryPromptTemplate = `你是一个超长文本分段摘要助手。下面是长消息的第 %d/%d 段，请输出该分段的摘要。

要求：
1. 使用中文。
2. 不超过 %d 个字符。
3. 只保留该分段的关键信息。
4. 如果该段主要是日志/报错，保留错误关键词、模块、异常现象。
5. 如果该段主要是代码，保留代码用途、关键函数、关键问题点。

该段预估 tokens：%d

分段内容：
%s`

	mergedMessageSummaryPromptTemplate = `你是一个超长文本总摘要助手。下面是一个超长消息各分段的摘要，请合并为一条总摘要。

要求：
1. 使用中文。
2. 不超过 %d 个字符。
3. 优先保留整条长消息的核心问题、关键约束、主要异常、关键事实。
4. 删除重复信息，避免逐段复述。

原文总预估 tokens：%d

分段摘要：
`

	messageChunkSummaryTemplate = "%d. %s\n"
)
