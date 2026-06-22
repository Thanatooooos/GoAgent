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

func TestRenderStructuredSummaryPlacesActivePrioritiesBeforeOpenQuestions(t *testing.T) {
	summary := StructuredSummary{
		SchemaVersion:    2,
		Goal:             "起草 summary 样本",
		ActivePriorities: []string{"先完成 spec、design、tasks"},
		OpenQuestions:    []string{"prompt template 是否引入额外占位符"},
		BackgroundIssues: []string{"CI flaky 不是当前重点"},
	}

	rendered := RenderStructuredSummary(summary, 0)

	activeIndex := strings.Index(rendered, "当前优先级：")
	openIndex := strings.Index(rendered, "待确认问题：")
	backgroundIndex := strings.Index(rendered, "背景问题：")
	if !(activeIndex >= 0 && openIndex > activeIndex && backgroundIndex > openIndex) {
		t.Fatalf("unexpected section order: %q", rendered)
	}
}
