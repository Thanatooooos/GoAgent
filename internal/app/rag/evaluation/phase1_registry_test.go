package evaluation

import "testing"

func TestNewPhase1RegistryRegistersSummaryAndRewrite(t *testing.T) {
	registry, err := NewPhase1Registry(Phase1RegistryDependencies{
		SummaryGenerator: &stubSummaryGenerator{},
		RewriteService:   &captureRewriteService{},
		RetrieveService:  &queryRetrieveService{},
	})
	if err != nil {
		t.Fatalf("NewPhase1Registry() error = %v", err)
	}

	suites := registry.List()
	if len(suites) != 2 {
		t.Fatalf("registry suites len = %d, want 2", len(suites))
	}
	if _, ok := registry.Get(SuiteSummary); !ok {
		t.Fatal("summary evaluator expected registered")
	}
	if _, ok := registry.Get(SuiteRewrite); !ok {
		t.Fatal("rewrite evaluator expected registered")
	}
}

func TestNewPhase1RegistryForSummaryDoesNotRequireRewriteService(t *testing.T) {
	registry, err := NewPhase1RegistryForSuite(SuiteSummary, Phase1RegistryDependencies{
		SummaryGenerator: &stubSummaryGenerator{},
	})
	if err != nil {
		t.Fatalf("NewPhase1RegistryForSuite(summary) error = %v", err)
	}
	if _, ok := registry.Get(SuiteSummary); !ok {
		t.Fatal("summary evaluator expected registered")
	}
	if _, ok := registry.Get(SuiteRewrite); ok {
		t.Fatal("rewrite evaluator should not be registered for summary-only registry")
	}
}

func TestNewPhase1RegistryForRewriteDoesNotRequireSummaryGenerator(t *testing.T) {
	registry, err := NewPhase1RegistryForSuite(SuiteRewrite, Phase1RegistryDependencies{
		RewriteService:  &captureRewriteService{},
		RetrieveService: &queryRetrieveService{},
	})
	if err != nil {
		t.Fatalf("NewPhase1RegistryForSuite(rewrite) error = %v", err)
	}
	if _, ok := registry.Get(SuiteRewrite); !ok {
		t.Fatal("rewrite evaluator expected registered")
	}
	if _, ok := registry.Get(SuiteSummary); ok {
		t.Fatal("summary evaluator should not be registered for rewrite-only registry")
	}
}
