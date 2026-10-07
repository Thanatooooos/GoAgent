package rewrite

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	rewriteHistoryHeader = "\n\n## 对话历史\n"

	rewriteHistoryInstruction = "\n请根据以上对话历史，对用户的最新问题进行指代消解和改写。"
)
