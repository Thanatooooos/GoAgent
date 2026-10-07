package pgvector

import (
	"strings"
	"testing"
)

func TestBuildLexicalPayloadIncludesMetadataFields(t *testing.T) {
	payload := BuildLexicalPayload("RAG 检索增强生成", map[string]any{
		"document_name":    "guide__rag_intro.md",
		"source_file_name": "rag_intro.md",
		"section":          "RAG > 检索增强生成",
	})

	if !strings.Contains(payload.ContentLexemes, "检索") {
		t.Fatalf("expected content lexemes to include Chinese bigram, got %q", payload.ContentLexemes)
	}
	if !strings.Contains(payload.DocumentNameLexemes, "guide") {
		t.Fatalf("expected document_name lexemes to include ASCII token, got %q", payload.DocumentNameLexemes)
	}
	if !strings.Contains(payload.SectionLexemes, "检索增强生成") && !strings.Contains(payload.SectionLexemes, "检索") {
		t.Fatalf("expected section lexemes to include section tokens, got %q", payload.SectionLexemes)
	}
}

func TestBuildLexicalQueryStripsHelperPhrases(t *testing.T) {
	query := buildLexicalQuery("查找 Development Notes - 2026-05-02 > Ingestion 进展 这一节")
	if strings.Contains(query.NormalizedText, "查找") || strings.Contains(query.NormalizedText, "这一节") {
		t.Fatalf("expected helper phrases stripped, got %q", query.NormalizedText)
	}
	if len(query.Tokens) == 0 || query.TSQuery == "" {
		t.Fatalf("expected non-empty lexical query, got %+v", query)
	}
}

func TestBuildShortIdentifierFallbackQuery(t *testing.T) {
	if !buildShortIdentifierFallbackQuery("AQ") {
		t.Fatal("expected short ASCII identifier to prefer fallback")
	}
	if buildShortIdentifierFallbackQuery("查找章节") {
		t.Fatal("did not expect Han query to prefer fallback")
	}
	if buildShortIdentifierFallbackQuery("very_long_identifier") {
		t.Fatal("did not expect long query to prefer fallback")
	}
}

func TestBuildBM25QueryExpandsNaturalChineseQuestion(t *testing.T) {
	got := buildBM25Query("保安服务是什么？")
	want := "保安服务 OR 保安 OR 安服 OR 服务"
	if got != want {
		t.Fatalf("buildBM25Query() = %q, want %q", got, want)
	}
}

func TestBuildBM25QueryPreservesNonChineseAndBooleanQueries(t *testing.T) {
	for _, query := range []string{"security service", "保安 AND 服务"} {
		if got := buildBM25Query(query); got != query {
			t.Fatalf("buildBM25Query(%q) = %q, want unchanged", query, got)
		}
	}
}
