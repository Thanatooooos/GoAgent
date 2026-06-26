package main

import (
	"testing"

	infraai "local/rag-project/internal/infra-ai"
)

func TestBuildSummaryOnlyEvalRuntimeDoesNotRequireDatabase(t *testing.T) {
	aiRuntime := infraai.NewRuntime()
	runtime := buildSummaryOnlyEvalRuntime(aiRuntime)
	if runtime == nil {
		t.Fatal("runtime is nil")
	}
	if runtime.DB != nil {
		t.Fatalf("DB = %#v, want nil", runtime.DB)
	}
	if runtime.LLMChat != aiRuntime.Chat {
		t.Fatal("LLMChat should come from the AI runtime")
	}
}
