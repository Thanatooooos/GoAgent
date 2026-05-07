package rewrite

import "testing"

func TestDefaultServiceRewriteWithSplit(t *testing.T) {
	service := NewDefaultService()

	result := service.RewriteWithSplit("  hello world  ")

	if result.RewrittenQuestion != "hello world" {
		t.Fatalf("expected normalized question, got %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 1 || result.SubQuestions[0] != "hello world" {
		t.Fatalf("unexpected sub questions: %#v", result.SubQuestions)
	}
	if result.PreferredSearchMode == "" {
		t.Fatal("expected preferred search mode")
	}
}
