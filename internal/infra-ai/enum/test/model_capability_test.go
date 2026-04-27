package test

import (
	"testing"

	aienum "local/rag-project/internal/infra-ai/enum"
)

func TestModelCapabilityDisplayName(t *testing.T) {
	if got := aienum.ModelCapabilityChat.DisplayName(); got != "Chat" {
		t.Fatalf("expected Chat, got %q", got)
	}
	if got := aienum.ModelCapabilityEmbedding.DisplayName(); got != "Embedding" {
		t.Fatalf("expected Embedding, got %q", got)
	}
	if got := aienum.ModelCapabilityRerank.DisplayName(); got != "Rerank" {
		t.Fatalf("expected Rerank, got %q", got)
	}
}

func TestModelCapabilityMatchesAndParse(t *testing.T) {
	if !aienum.ModelCapabilityChat.Matches("CHAT") {
		t.Fatal("chat capability should match case-insensitively")
	}

	capability, ok := aienum.ParseModelCapability(" embedding ")
	if !ok {
		t.Fatal("expected parse capability success")
	}
	if capability != aienum.ModelCapabilityEmbedding {
		t.Fatalf("expected embedding, got %q", capability)
	}

	if _, ok := aienum.ParseModelCapability("unknown"); ok {
		t.Fatal("expected parse capability failed for unknown")
	}
}

func TestAllModelCapabilities(t *testing.T) {
	all := aienum.AllModelCapabilities()
	if len(all) != 3 {
		t.Fatalf("expected 3 capabilities, got %d", len(all))
	}
}
