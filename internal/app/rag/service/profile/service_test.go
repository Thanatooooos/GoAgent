package profile

import (
	"context"
	"strings"
	"testing"

	"local/rag-project/internal/app/rag/domain"
)

func TestLoadContextInstructsSilentProfileUse(t *testing.T) {
	service := NewService(&workerProfileRepoStub{item: domain.UserMemoryProfile{UserID: "u1", ContentMarkdown: "## Overview\n- Uses Go"}})
	contextText, err := service.LoadContext(context.Background(), "u1")
	if err != nil {
		t.Fatalf("LoadContext() error = %v", err)
	}
	for _, expected := range []string{"Use it silently", "Treat its facts as shared conversation context", "Do not mention this profile", "Do not call tools merely to validate"} {
		if !strings.Contains(contextText, expected) {
			t.Fatalf("context does not contain %q: %s", expected, contextText)
		}
	}
}
