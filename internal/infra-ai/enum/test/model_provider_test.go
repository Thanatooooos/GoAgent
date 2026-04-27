package test

import (
	"testing"

	aienum "local/rag-project/internal/infra-ai/enum"
)

func TestModelProviderMatchesAndParse(t *testing.T) {
	if !aienum.ModelProviderBaiLian.Matches("BAILIAN") {
		t.Fatal("provider should match case-insensitively")
	}

	provider, ok := aienum.ParseModelProvider(" siliconflow ")
	if !ok {
		t.Fatal("expected parse provider success")
	}
	if provider != aienum.ModelProviderSiliconFlow {
		t.Fatalf("expected siliconflow, got %q", provider)
	}

	if _, ok := aienum.ParseModelProvider("abc"); ok {
		t.Fatal("expected parse provider failed for unknown")
	}
}

func TestAllModelProviders(t *testing.T) {
	all := aienum.AllModelProviders()
	if len(all) != 4 {
		t.Fatalf("expected 4 providers, got %d", len(all))
	}
}
