package rewrite

import (
	"testing"

	"local/rag-project/internal/framework/convention"
)

type rewriteServiceStubForTermNormalization struct {
	rewrite            string
	rewriteWithSplit   Result
	rewriteWithHistory Result
}

func (s rewriteServiceStubForTermNormalization) Rewrite(string) string {
	return s.rewrite
}

func (s rewriteServiceStubForTermNormalization) RewriteWithSplit(string) Result {
	return s.rewriteWithSplit
}

func (s rewriteServiceStubForTermNormalization) RewriteWithHistory(string, []convention.ChatMessage) Result {
	return s.rewriteWithHistory
}

func TestTermNormalizingServiceRewriteWithSplit(t *testing.T) {
	base := rewriteServiceStubForTermNormalization{
		rewriteWithSplit: Result{
			RewrittenQuestion: "pg connection issue",
			SubQuestions: []string{
				"pg connection issue",
				"postgres timeout",
				"es health",
			},
			NeedRetrieval: true,
		},
	}
	service := NewTermNormalizingService(base, TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{Canonical: "PostgreSQL", Aliases: []string{"postgres", "pg"}},
			{Canonical: "Elasticsearch", Aliases: []string{"es"}},
		},
	})

	result := service.RewriteWithSplit("ignored")
	if result.RewrittenQuestion != "PostgreSQL connection issue" {
		t.Fatalf("unexpected rewritten question: %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 3 {
		t.Fatalf("expected 3 normalized sub questions, got %v", result.SubQuestions)
	}
	if result.SubQuestions[0] != "PostgreSQL connection issue" {
		t.Fatalf("unexpected first sub question: %q", result.SubQuestions[0])
	}
	if result.SubQuestions[1] != "PostgreSQL timeout" {
		t.Fatalf("unexpected second sub question: %q", result.SubQuestions[1])
	}
	if result.SubQuestions[2] != "Elasticsearch health" {
		t.Fatalf("unexpected third sub question: %q", result.SubQuestions[2])
	}
}

func TestTermNormalizingServiceRewriteRespectsWordBoundaries(t *testing.T) {
	base := rewriteServiceStubForTermNormalization{
		rewrite: "jpg preview and pg status",
	}
	service := NewTermNormalizingService(base, TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{Canonical: "PostgreSQL", Aliases: []string{"pg"}},
		},
	})

	rewritten := service.Rewrite("ignored")
	if rewritten != "jpg preview and PostgreSQL status" {
		t.Fatalf("unexpected boundary normalization result: %q", rewritten)
	}
}

func TestTermNormalizingServiceRewriteWithHistoryFallsBackToRewrittenQuestion(t *testing.T) {
	base := rewriteServiceStubForTermNormalization{
		rewriteWithHistory: Result{
			RewrittenQuestion: "向量库 如何扩容",
			NeedRetrieval:     true,
		},
	}
	service := NewTermNormalizingService(base, TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{Canonical: "向量数据库", Aliases: []string{"向量库"}},
		},
	})

	result := service.RewriteWithHistory("ignored", nil)
	if result.RewrittenQuestion != "向量数据库 如何扩容" {
		t.Fatalf("unexpected rewritten question: %q", result.RewrittenQuestion)
	}
	if len(result.SubQuestions) != 1 || result.SubQuestions[0] != "向量数据库 如何扩容" {
		t.Fatalf("expected fallback sub question from rewritten text, got %v", result.SubQuestions)
	}
}

func TestTermNormalizerReportsMatches(t *testing.T) {
	normalizer := NewTermNormalizer(TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{
				Canonical: "PostgreSQL",
				Aliases:   []string{"postgres", "pg"},
				Category:  "component",
				Version:   1,
			},
		},
	})

	_, report := normalizer.NormalizeTextWithReport("pg connection issue")
	if !report.Changed {
		t.Fatal("expected report.Changed=true")
	}
	if len(report.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(report.Matches))
	}
	if report.Matches[0].Alias != "pg" || report.Matches[0].Canonical != "PostgreSQL" {
		t.Fatalf("unexpected match: %+v", report.Matches[0])
	}
	if report.Matches[0].Category != "component" || report.Matches[0].Version != 1 {
		t.Fatalf("unexpected match metadata: %+v", report.Matches[0])
	}
}

func TestTermNormalizerSkipsDisabledRules(t *testing.T) {
	disabled := false
	normalizer := NewTermNormalizer(TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{
				Canonical: "PostgreSQL",
				Aliases:   []string{"pg"},
				Enabled:   &disabled,
			},
		},
	})
	if normalizer != nil {
		t.Fatal("expected disabled rule to produce nil normalizer")
	}
}

func TestTermNormalizerPreservesMetadata(t *testing.T) {
	base := rewriteServiceStubForTermNormalization{
		rewriteWithSplit: Result{
			RewrittenQuestion: "pg error",
			SubQuestions:      []string{"pg error"},
			NeedRetrieval:     true,
			Metadata: map[string]any{
				"source": "llm",
			},
		},
	}
	service := NewTermNormalizingService(base, TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{Canonical: "PostgreSQL", Aliases: []string{"pg"}},
		},
	})

	result := service.RewriteWithSplit("ignored")
	if result.Metadata["source"] != "llm" {
		t.Fatalf("expected existing metadata preserved, got %#v", result.Metadata)
	}
	report, ok := result.Metadata["termNormalization"].(TermNormalizationReport)
	if !ok {
		t.Fatalf("expected termNormalization metadata, got %#v", result.Metadata["termNormalization"])
	}
	if !report.Changed || len(report.Matches) == 0 {
		t.Fatalf("expected normalization report, got %+v", report)
	}
}

func TestTermNormalizerLongestAliasFirst(t *testing.T) {
	normalizer := NewTermNormalizer(TermNormalizationOptions{
		Enabled: true,
		Rules: []TermNormalizationRule{
			{Canonical: "PostgreSQL", Aliases: []string{"pg", "postgres"}},
		},
	})

	normalized, report := normalizer.NormalizeTextWithReport("postgres timeout")
	if normalized != "PostgreSQL timeout" {
		t.Fatalf("unexpected normalized text: %q", normalized)
	}
	if len(report.Matches) != 1 || report.Matches[0].Alias != "postgres" {
		t.Fatalf("expected longest alias match, got %+v", report.Matches)
	}
}

func TestNewTermNormalizingServiceDisabledKeepsBaseBehavior(t *testing.T) {
	base := rewriteServiceStubForTermNormalization{
		rewriteWithSplit: Result{
			RewrittenQuestion: "pg error",
			SubQuestions:      []string{"pg error"},
			NeedRetrieval:     true,
		},
	}
	service := NewTermNormalizingService(base, TermNormalizationOptions{
		Enabled: false,
		Rules: []TermNormalizationRule{
			{Canonical: "PostgreSQL", Aliases: []string{"pg"}},
		},
	})

	result := service.RewriteWithSplit("ignored")
	if result.RewrittenQuestion != "pg error" {
		t.Fatalf("expected disabled normalizer to keep original result, got %q", result.RewrittenQuestion)
	}
}
