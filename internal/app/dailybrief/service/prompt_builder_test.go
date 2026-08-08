package service

import (
	"strings"
	"testing"

	"local/rag-project/internal/app/dailybrief/domain"
)

func TestBuildBriefGenerationSystemPromptRequiresChinese(t *testing.T) {
	prompt := BuildBriefGenerationSystemPrompt(0)
	if !strings.Contains(prompt, "简体中文") {
		t.Fatalf("system prompt should require Simplified Chinese, got: %q", prompt)
	}
	if !strings.Contains(prompt, "80-160") {
		t.Fatalf("system prompt should require richer item summaries, got: %q", prompt)
	}
}

func TestBuildBriefGenerationSystemPromptIncludesPerTopicLimit(t *testing.T) {
	prompt := BuildBriefGenerationSystemPrompt(2)
	if !strings.Contains(prompt, "每个 topic 栏目最多输出 2 条") {
		t.Fatalf("expected per-topic limit in system prompt, got: %q", prompt)
	}
}

func TestBuildBriefGenerationPromptIncludesTopicTitles(t *testing.T) {
	prompt := BuildBriefGenerationPrompt("2026-06-29", []string{domain.TopicKeyTechAIModels}, []domain.Candidate{
		domain.NewCandidate("c1", domain.SourceKeyTechCrunchAI, "Launch", "https://example.com/a"),
	}, 2)
	if !strings.Contains(prompt, "简报日期：2026-06-29") {
		t.Fatalf("expected Chinese brief date label, got: %q", prompt)
	}
	if !strings.Contains(prompt, "模型发布") {
		t.Fatalf("expected Chinese topic title hint, got: %q", prompt)
	}
	if !strings.Contains(prompt, "每个订阅 topic 最多输出 2 条") {
		t.Fatalf("expected per-topic instruction, got: %q", prompt)
	}
}
