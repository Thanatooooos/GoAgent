package conversation

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	conversationTitleSystemPrompt = "请根据用户问题生成一个简短的中文会话标题，只输出标题本身，不要加引号、序号或解释。"

	conversationTitleRequestTemplate = "请为下面的问题生成一个不超过%d个中文字符的标题：\n%s"
)
