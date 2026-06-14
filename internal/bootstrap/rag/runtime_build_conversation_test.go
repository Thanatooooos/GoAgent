package rag

import (
	"testing"

	"local/rag-project/internal/framework/config"
)

func TestBuildSummaryBudgetOptionsUsesConfiguredValues(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.SummaryMaxChars = 320
	cfg.Rag.Memory.SummaryBudget.SmallMaxChars = 400
	cfg.Rag.Memory.SummaryBudget.MediumMaxChars = 600
	cfg.Rag.Memory.SummaryBudget.LargeMaxChars = 800
	cfg.Rag.Memory.SummaryBudget.MediumMessageCountMin = 6
	cfg.Rag.Memory.SummaryBudget.LargeMessageCountMin = 10

	options := buildSummaryBudgetOptions(cfg)
	if options.SmallMaxChars != 400 {
		t.Fatalf("expected small max chars 400, got %d", options.SmallMaxChars)
	}
	if options.MediumMaxChars != 600 {
		t.Fatalf("expected medium max chars 600, got %d", options.MediumMaxChars)
	}
	if options.LargeMaxChars != 800 {
		t.Fatalf("expected large max chars 800, got %d", options.LargeMaxChars)
	}
	if options.MediumMessageCountMin != 6 {
		t.Fatalf("expected medium message threshold 6, got %d", options.MediumMessageCountMin)
	}
	if options.LargeMessageCountMin != 10 {
		t.Fatalf("expected large message threshold 10, got %d", options.LargeMessageCountMin)
	}
}

func TestBuildSummaryBudgetOptionsFallsBackToLegacySummaryMaxChars(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.SummaryMaxChars = 320

	options := buildSummaryBudgetOptions(cfg)
	if options.SmallMaxChars != 320 {
		t.Fatalf("expected small max chars fallback 320, got %d", options.SmallMaxChars)
	}
	if options.MediumMaxChars != 320 {
		t.Fatalf("expected medium max chars fallback 320, got %d", options.MediumMaxChars)
	}
	if options.LargeMaxChars != 320 {
		t.Fatalf("expected large max chars fallback 320, got %d", options.LargeMaxChars)
	}
}
