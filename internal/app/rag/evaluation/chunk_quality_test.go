package evaluation

import (
	"strings"
	"testing"
)

func TestEvaluateChunkQualityEmpty(t *testing.T) {
	report := EvaluateChunkQuality(nil, ChunkQualityOptions{})
	if report.TotalChunks != 0 {
		t.Fatalf("expected 0 total chunks, got %d", report.TotalChunks)
	}
	if report.TotalDocs != 0 {
		t.Fatalf("expected 0 total docs, got %d", report.TotalDocs)
	}
}

func TestEvaluateChunkQualitySingleStrategy(t *testing.T) {
	longText := strings.Repeat("x", 900) // oversized
	normalText := "# Section\n" + strings.Repeat("a", 300) + " end" // ~315 chars, well under 800
	chunks := []ChunkSample{
		{ID: "c1", Index: 0, Text: normalText, Strategy: "fixed_size", Metadata: map[string]any{"section": "Overview"}, DocumentID: "doc-1"},
		{ID: "c2", Index: 1, Text: longText, Strategy: "fixed_size", Metadata: map[string]any{}, DocumentID: "doc-1"},
		{ID: "c3", Index: 2, Text: "# Heading\ncontent", Strategy: "fixed_size", Metadata: map[string]any{"heading_path": []string{"H1"}}, DocumentID: "doc-2"},
	}
	report := EvaluateChunkQuality(chunks, ChunkQualityOptions{
		ExpectedChunkSize: 800,
		MinChunkSize:      50,
	})
	if report.TotalChunks != 3 {
		t.Fatalf("expected 3 total chunks, got %d", report.TotalChunks)
	}
	if report.TotalDocs != 2 {
		t.Fatalf("expected 2 total docs, got %d", report.TotalDocs)
	}
	if len(report.ByStrategy) != 1 {
		t.Fatalf("expected 1 strategy, got %d", len(report.ByStrategy))
	}
	s := report.ByStrategy[0]
	if s.Strategy != "fixed_size" {
		t.Fatalf("expected fixed_size strategy, got %q", s.Strategy)
	}
	if s.OversizedCount != 1 {
		t.Fatalf("expected 1 oversized, got %d", s.OversizedCount)
	}
	if s.UndersizedCount != 1 {
		t.Fatalf("expected 1 undersized (chunk3), got %d", s.UndersizedCount)
	}
	if s.SectionCoverage <= 0 {
		t.Fatalf("expected non-zero section coverage, got %.2f", s.SectionCoverage)
	}
	if s.HeadingPathCoverage <= 0 {
		t.Fatalf("expected non-zero heading coverage, got %.2f", s.HeadingPathCoverage)
	}
	if s.BoundaryQuality <= 0 {
		t.Fatalf("expected non-zero boundary quality, got %.2f", s.BoundaryQuality)
	}
}

func TestEvaluateChunkQualityMultipleStrategies(t *testing.T) {
	chunks := []ChunkSample{
		{ID: "c1", Index: 0, Text: "content A", Strategy: "fixed_size", Metadata: map[string]any{}, DocumentID: "doc-1"},
		{ID: "c2", Index: 0, Text: "# Intro\ncontent B", Strategy: "markdown", Metadata: map[string]any{"section": "Intro", "heading_path": []string{"Intro"}}, DocumentID: "doc-2"},
	}
	report := EvaluateChunkQuality(chunks, ChunkQualityOptions{})
	if len(report.ByStrategy) != 2 {
		t.Fatalf("expected 2 strategies, got %d", len(report.ByStrategy))
	}
	names := map[string]bool{}
	for _, s := range report.ByStrategy {
		names[s.Strategy] = true
	}
	if !names["fixed_size"] || !names["markdown"] {
		t.Fatalf("expected both fixed_size and markdown in strategies, got %v", names)
	}
	if report.Overall.ChunkCount != 2 {
		t.Fatalf("expected 2 overall chunks, got %d", report.Overall.ChunkCount)
	}
	if report.Overall.BoundaryQuality <= 0 {
		t.Fatalf("expected non-zero overall boundary quality")
	}
}

func TestStartsAtNaturalBoundary(t *testing.T) {
	tests := []struct {
		text     string
		expected bool
	}{
		{"regular text", false},
		{"\nparagraph after blank line", true},
		{"# heading", true},
		{"## subheading", true},
		{"第一章 概述", true},
		{"一、简介", true},
		{"  # indented heading", true},
	}
	for _, tc := range tests {
		if got := startsAtNaturalBoundary(tc.text); got != tc.expected {
			t.Errorf("startsAtNaturalBoundary(%q) = %v, want %v", tc.text, got, tc.expected)
		}
	}
}
