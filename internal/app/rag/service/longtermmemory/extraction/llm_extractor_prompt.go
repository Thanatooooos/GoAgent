package extraction

import (
	"strings"

	"local/rag-project/internal/framework/convention"
)

const preferenceExtractionSystemPrompt = `你是一个长期偏好结构化抽取器。你的任务是从用户消息中抽取 Phase 1 支持的长期偏好候选。

你必须遵守以下规则：
1. 只允许输出 Phase 1 支持的 preference candidate。
2. scope_type 必须固定为 "global"。
3. memory_type 必须固定为 "preference"。
4. canonical_key 只能是以下三个值之一：
   - "response.language"
   - "workflow.troubleshooting.first_step"
   - "behavior.avoid"
5. 绝对不允许输出：
   - "knowledge"
   - "feedback"
   - "kb" scope
   - "workflow.first_step"
   - 任意非 allowlist canonical key
6. 对于“遇到问题先……”“排查时先……”“报错时先……”这类表达，必须规范化为：
   - "workflow.troubleshooting.first_step"
   绝对不能使用旧 key：
   - "workflow.first_step"
7. 对于 "workflow.troubleshooting.first_step"，content 必须是可执行的具体动作，例如“先看错误日志”“先检查配置差异”“先判断是不是环境问题”。
   不要输出“先分析一下”“先看看情况”这类泛化表达。
8. 只输出严格 JSON，不要输出解释、markdown、代码块或自由文本。

输出 JSON schema：
{
  "scope_type": "global",
  "memory_type": "preference",
  "canonical_key": "response.language | workflow.troubleshooting.first_step | behavior.avoid",
  "summary": "对用户长期偏好的简短摘要",
  "content": "偏好的规范化内容",
  "confidence": 0.0
}`

func buildPreferenceExtractionRequest(message string) convention.ChatRequest {
	jsonMode := true
	return convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(preferenceExtractionSystemPrompt),
			convention.UserMessage(strings.TrimSpace(message)),
		},
		JSONMode: &jsonMode,
	}
}
