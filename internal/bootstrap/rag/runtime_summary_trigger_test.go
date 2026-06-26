package rag

import (
	"testing"

	"local/rag-project/internal/framework/config"
)

func TestComputeSummaryTriggerTokensExplicitOverride(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.SummaryTriggerTokens = 4200
	if got := computeSummaryTriggerTokens(cfg); got != 4200 {
		t.Fatalf("expected explicit override 4200, got %d", got)
	}
}

func TestComputeSummaryTriggerTokensAutoDerived(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.ChatContext.MaxPromptTokens = 8000
	cfg.Rag.Memory.ExplicitRecall.MaxContextChars = 1600
	cfg.Rag.Memory.SessionRecall.MaxPromptTokens = 1500

	got := computeSummaryTriggerTokens(cfg)
	if got != 2700 {
		t.Fatalf("expected auto-derived history budget 2700, got %d", got)
	}
}

func TestComputeSummaryTriggerTokensCustomOverhead(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.ChatContext.MaxPromptTokens = 8000
	cfg.Rag.Memory.SummaryOverheadReserveTokens = 6000

	got := computeSummaryTriggerTokens(cfg)
	if got != 2000 {
		t.Fatalf("expected history budget 2000, got %d", got)
	}
}

func TestComputeSummaryTriggerTokensUsesStageReserves(t *testing.T) {
	cfg := &config.Config{}
	cfg.Rag.Memory.ChatContext.MaxPromptTokens = 8000
	cfg.Rag.Memory.ChatContext.FixedReserveTokens = 800
	cfg.Rag.Memory.ChatContext.SafetyReserveTokens = 500
	cfg.Rag.Memory.ChatContext.StageBudget.MemoryTokens = 500
	cfg.Rag.Memory.ChatContext.StageBudget.SessionRecallTokens = 1500
	cfg.Rag.Memory.ChatContext.StageBudget.RetrieveTokens = 2000
	cfg.Rag.Memory.ChatContext.StageBudget.ToolTokens = 1500

	if got := computeSummaryTriggerTokens(cfg); got != 1200 {
		t.Fatalf("computeSummaryTriggerTokens() = %d, want 1200", got)
	}
}
