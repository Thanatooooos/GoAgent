package llmgen

import (
	"regexp"
	"strings"
	"testing"
)

func TestRewriteRefsBasic(t *testing.T) {
	got, stats := RewriteRefs("答案 [[r1]] 结束", []RewriteRule{{Find: "[[r1]]", Replace: "[来源]"}}, RewriteOptions{})
	if got != "答案 [来源] 结束" {
		t.Fatalf("rewrite = %q", got)
	}
	if stats.Rewritten != 1 || stats.Skipped != 0 {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestRewriteRefsSkipsCodeBlocks(t *testing.T) {
	text := "正文\n```\n[[r1]] not a ref\n```\n后文 [[r1]]"
	got, stats := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "X"}}, RewriteOptions{SkipCodeBlocks: true})
	if strings.Contains(got, "后文 X") == false {
		t.Fatalf("inline ref not rewritten: %q", got)
	}
	if !strings.Contains(got, "```\n[[r1]] not a ref\n```") {
		t.Fatalf("code block ref should be preserved: %q", got)
	}
	if stats.Rewritten != 1 || stats.Skipped != 1 {
		t.Fatalf("stats = %+v (rewritten=%d skipped=%d)", stats, stats.Rewritten, stats.Skipped)
	}
}

func TestRewriteRefsSkipsInlineCodeAndLinks(t *testing.T) {
	text := "`[[r1]]` and [label](https://x/[[r1]]) and [[r1]]"
	got, _ := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "R"}}, RewriteOptions{SkipCodeBlocks: true, SkipLinks: true})
	if got != "`[[r1]]` and [label](https://x/[[r1]]) and R" {
		t.Fatalf("rewrite = %q", got)
	}
}

func TestRewriteRefsWordBoundary(t *testing.T) {
	text := "a[[r1]]b [[r1]]"
	got, _ := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "R"}}, RewriteOptions{WordBoundary: true})
	if got != "a[[r1]]b R" {
		t.Fatalf("rewrite with word boundary = %q", got)
	}
}

func TestCleanDeadRefs(t *testing.T) {
	text := "见 [[r1]] 与 [[r9]]"
	refRE := regexp.MustCompile(`\[\[(r[1-9][0-9]*)\]\]`)
	got, removed := CleanDeadRefs(text, func(id string) bool { return id == "r1" }, refRE)
	if removed != 1 {
		t.Fatalf("removed = %d", removed)
	}
	if strings.Contains(got, "r9") {
		t.Fatalf("dead ref not removed: %q", got)
	}
	if !strings.Contains(got, "[[r1]]") {
		t.Fatalf("valid ref removed: %q", got)
	}
}

func TestRewriteRefsSkipsStraddlingMatch(t *testing.T) {
	// match starts before the inline-code span opener and ends inside it
	text := "ab`c`d"
	got, _ := RewriteRefs(text, []RewriteRule{{Find: "b`c", Replace: "X"}}, RewriteOptions{SkipCodeBlocks: true})
	if got != "ab`c`d" {
		t.Fatalf("straddling match should not be rewritten: %q", got)
	}
}

func TestRewriteRefsUnclosedFenceProtectsToEOF(t *testing.T) {
	text := "```\n[[r1]] still code"
	got, stats := RewriteRefs(text, []RewriteRule{{Find: "[[r1]]", Replace: "X"}}, RewriteOptions{SkipCodeBlocks: true})
	if !strings.Contains(got, "[[r1]]") {
		t.Fatalf("unclosed fence content should be preserved: %q", got)
	}
	if stats.Skipped != 1 {
		t.Fatalf("Skipped = %d, want 1", stats.Skipped)
	}
}

func TestCleanDeadRefsDegenerateInputs(t *testing.T) {
	refRE := regexp.MustCompile(`\[\[(r[1-9][0-9]*)\]\]`)
	if got, removed := CleanDeadRefs("", func(string) bool { return true }, refRE); removed != 0 || got != "" {
		t.Fatalf("empty input = (%q,%d)", got, removed)
	}
	if got, removed := CleanDeadRefs("x [[r1]]", nil, refRE); removed != 1 || strings.Contains(got, "r1") {
		t.Fatalf("nil keep removes all = (%q,%d)", got, removed)
	}
	if got, removed := CleanDeadRefs("x [[r1]]", func(string) bool { return true }, nil); removed != 0 || got != "x [[r1]]" {
		t.Fatalf("nil pattern = (%q,%d)", got, removed)
	}
}
