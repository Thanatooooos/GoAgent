package websource

import "testing"

func TestEngineDenyRuleWinsAndAllowsConfiguredDomain(t *testing.T) {
	t.Parallel()
	policy := New(Config{AllowDomains: []string{"go.dev"}, DenyDomains: []string{"quora.com"}})
	if got := policy.Evaluate("https://www.quora.com/question"); got.Policy != PolicyDeny {
		t.Fatalf("deny assessment = %#v", got)
	}
	if got := policy.Evaluate("https://go.dev/doc"); got.Policy != PolicyAllow || got.Domain != "go.dev" {
		t.Fatalf("allow assessment = %#v", got)
	}
}
