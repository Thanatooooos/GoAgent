package history

import (
	"strings"
	"testing"
)

func TestRenderStructuredSummaryOmitsEmptySectionsAndKeepsOrder(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion: 1,
		Goal:          "实现结构化摘要",
		Constraints:   []string{"保持 LoadLatestSummary 兼容"},
	}

	rendered := RenderStructuredSummary(summary, 400)
	if !strings.HasPrefix(rendered, "目标：实现结构化摘要") {
		t.Fatalf("unexpected render prefix: %q", rendered)
	}
	if strings.Contains(rendered, "用户偏好：") {
		t.Fatalf("expected empty sections to be omitted: %q", rendered)
	}
	if !strings.Contains(rendered, "约束：\n- 保持 LoadLatestSummary 兼容") {
		t.Fatalf("expected constraints section, got %q", rendered)
	}
}
