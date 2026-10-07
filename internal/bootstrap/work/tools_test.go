package work

import "testing"

func TestValidateDraftParagraphsRejectsDanglingConnector(t *testing.T) {
	if err := validateDraftParagraphs([]string{"一、你的起点评估", "你已有熟练的C++功底。这意味着你不必从", "二、语言迁移"}); err == nil {
		t.Fatal("expected an incomplete paragraph to be rejected")
	}
}

func TestValidateDraftParagraphsAllowsCompleteParagraphs(t *testing.T) {
	paragraphs := []string{"你已有熟练的C++功底，因此可以直接进入Python数据分析实践。", "使用真实数据集完成一个小项目。"}
	if err := validateDraftParagraphs(paragraphs); err != nil {
		t.Fatalf("complete paragraphs rejected: %v", err)
	}
}
