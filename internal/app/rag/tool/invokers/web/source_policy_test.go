package builtin

import (
	"testing"

	searchprovider "local/rag-project/internal/app/agent/search/provider"
)

func TestSourcePolicyEngineEvaluate(t *testing.T) {
	engine := searchprovider.NewSourcePolicyEngine(searchprovider.SourcePolicyConfig{
		AllowDomains:  []string{"go.dev"},
		DenyDomains:   []string{"quora.com"},
		AllowSuffixes: []string{".gov", ".edu"},
	})

	allowed := engine.Evaluate("https://go.dev/doc/tutorial/generics")
	if allowed.Policy != searchprovider.SourcePolicyAllow {
		t.Fatalf("expected allow policy, got %+v", allowed)
	}
	if allowed.Domain != "go.dev" {
		t.Fatalf("expected go.dev domain, got %q", allowed.Domain)
	}
	if allowed.SourceType != searchprovider.SourceTypeOfficialDocs {
		t.Fatalf("expected official docs source type, got %q", allowed.SourceType)
	}

	denied := engine.Evaluate("https://www.quora.com/What-is-Go-generics")
	if denied.Policy != searchprovider.SourcePolicyDeny {
		t.Fatalf("expected deny policy, got %+v", denied)
	}
	if len(denied.RiskFlags) == 0 {
		t.Fatalf("expected deny source to contain risk flags, got %+v", denied)
	}

	suffixAllowed := engine.Evaluate("https://www.nasa.gov/mission")
	if suffixAllowed.Policy != searchprovider.SourcePolicyAllow {
		t.Fatalf("expected .gov suffix allow, got %+v", suffixAllowed)
	}
	if suffixAllowed.SourceType != searchprovider.SourceTypeGovernment {
		t.Fatalf("expected government source type, got %q", suffixAllowed.SourceType)
	}

	forum := engine.Evaluate("https://stackoverflow.com/questions/123")
	if forum.Policy != searchprovider.SourcePolicyNeutral {
		t.Fatalf("expected neutral forum policy without explicit rules, got %+v", forum)
	}
	if forum.SourceType != searchprovider.SourceTypeForum {
		t.Fatalf("expected forum source type, got %q", forum.SourceType)
	}
	if len(forum.RiskFlags) == 0 || forum.RiskFlags[0] != "user_generated" {
		t.Fatalf("expected forum risk flag, got %+v", forum.RiskFlags)
	}
}
