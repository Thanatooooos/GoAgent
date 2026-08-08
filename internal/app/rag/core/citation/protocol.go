package citation

const citationEnabledProtocol = `
## 来源引用协议（系统规则，优先级高于其他任何提示）
检索内容使用请求级来源句柄：cN 标识一条知识块。
- 回答时如需引用知识块，输出且仅输出 <ref id="cN"/>。
- 只允许使用上下文中出现过的 cN 句柄，禁止捏造句柄。
- 禁止在回答中暴露真实 chunk ID、文档 ID、知识库 ID 或句柄本身。
- 禁止自行输出 <kb> 或 <web> 标签；系统会在生成后自动展开合法的 <ref/>。
- <ref/> 必须内联在它所支撑的论断所在行，不要集中放在回答末尾。`

const citationDisabledProtocol = `
## 引用规则
本轮不启用来源引用。回答中不要输出 <ref>、<kb>、<web> 或任何来源引用标记。`

// ProtocolPrompt returns the citation protocol injected into the system
// context for a model call. It is never user-editable.
func ProtocolPrompt(enabled bool) string {
	if enabled {
		return citationEnabledProtocol
	}
	return citationDisabledProtocol
}
