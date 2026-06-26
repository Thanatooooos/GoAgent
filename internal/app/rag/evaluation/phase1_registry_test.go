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

func TestNewPhase1RegistryForSummaryPassesRuntimeOptions(t *testing.T) {
	registry, err := NewPhase1RegistryForSuite(SuiteSummary, Phase1RegistryDependencies{
		SummaryGenerator: &stubSummaryGenerator{},
		SummaryOptions: SummaryEvaluatorRuntimeOptions{
			Mode:                  SummaryEvalModeStrategy,
			ThresholdUnit:         SummaryStrategyThresholdTokens,
			Thresholds:            []int{800, 1200},
			MessageOverheadTokens: 4,
		},
	})
	if err != nil {
		t.Fatalf("NewPhase1RegistryForSuite(summary) error = %v", err)
	}
	evaluator, ok := registry.Get(SuiteSummary)
	if !ok {
		t.Fatal("summary evaluator expected registered")
	}
	typed, ok := evaluator.(*SummaryEvaluator)
	if !ok {
		t.Fatalf("unexpected evaluator type %T", evaluator)
	}
	if typed.runtime.Mode != SummaryEvalModeStrategy {
		t.Fatalf("Mode = %q, want %q", typed.runtime.Mode, SummaryEvalModeStrategy)
	}
	if typed.runtime.ThresholdUnit != SummaryStrategyThresholdTokens {
		t.Fatalf("ThresholdUnit = %q, want %q", typed.runtime.ThresholdUnit, SummaryStrategyThresholdTokens)
	}
	if len(typed.runtime.Thresholds) != 2 {
		t.Fatalf("thresholds len = %d, want 2", len(typed.runtime.Thresholds))
	}
	if typed.runtime.MessageOverheadTokens != 4 {
		t.Fatalf("MessageOverheadTokens = %d, want 4", typed.runtime.MessageOverheadTokens)
	}
}
