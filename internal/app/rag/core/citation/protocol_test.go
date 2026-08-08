package citation

import (
	"strings"
	"testing"
)

func TestProtocolPromptEnabled(t *testing.T) {
	prompt := ProtocolPrompt(true)
	if prompt == "" {
		t.Fatal("enabled protocol should not be empty")
	}
	for _, want := range []string{`<ref id="cN"/>`, "cN", "禁止"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("enabled protocol missing %q: %s", want, prompt)
		}
	}
}

func TestProtocolPromptDisabled(t *testing.T) {
	prompt := ProtocolPrompt(false)
	if prompt == "" {
		t.Fatal("disabled protocol should not be empty")
	}
	if !strings.Contains(prompt, "<ref") {
		t.Fatalf("disabled protocol should mention no-cite rule: %s", prompt)
	}
}
