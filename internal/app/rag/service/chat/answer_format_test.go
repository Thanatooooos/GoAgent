package chat

import "testing"

func TestNormalizeAssistantMarkdownSeparatesSectionsAndDropsDanglingCitationArtifacts(t *testing.T) {
	input := "结论。###信息状态说明- **本地知识库**未包含记录（）。- **外部来源质量**有限。最后（"
	want := "结论。\n\n### 信息状态说明\n\n- **本地知识库**未包含记录。\n- **外部来源质量**有限。最后"
	if got := normalizeAssistantMarkdown(input); got != want {
		t.Fatalf("normalizeAssistantMarkdown() = %q, want %q", got, want)
	}
}

func TestNormalizeAssistantMarkdownPreservesCodeAndNormalParentheses(t *testing.T) {
	input := "调用 foo()；代码 `###title()`；\n```go\n# include <stdio.h>\n```"
	got := normalizeAssistantMarkdown(input)
	if got != input {
		t.Fatalf("normalizeAssistantMarkdown() changed protected content: %q", got)
	}
}
