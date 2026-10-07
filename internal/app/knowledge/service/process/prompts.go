package process

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	documentSummaryPrompt = "请仅基于以下文档生成简洁摘要，不要编造信息：\n\n"

	chunkSummaryPrompt = "Summarize this chunk in one concise sentence. Do not add facts that are not in the content.\n\n"

	chunkQuestionsPromptTemplate = "基于以下内容生成最多两个用户问题，每行一个，不要编号。每个问题必须能只凭这段内容回答。\n\n%s"
)
