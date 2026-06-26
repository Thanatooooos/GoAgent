package evaluation

import (
	"fmt"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	"local/rag-project/internal/infra-ai/embedding"
)

type Phase1RegistryDependencies struct {
	SummaryGenerator               SummaryGenerator
	SummaryJudge                   Judge
	SummaryAnswerGenerator         SummaryAnswerGenerator
	SummaryOptions                 SummaryEvaluatorRuntimeOptions
	RewriteService                 ragrewrite.Service
	RewriteEmbedding               embedding.EmbeddingService
	RewriteEmbeddingModelID        string
	RewriteJudge                   Judge
	RetrieveService                ragretrieve.Service
	RewriteRetrievalKs             []int
	RewriteSubQuestionOptions      ragretrieve.SubQuestionOptions
	RewriteDefaultKnowledgeBaseIDs []string
}

func NewPhase1Registry(deps Phase1RegistryDependencies) (*Registry, error) {
	return NewPhase1RegistryForSuite(SuiteAll, deps)
}

func NewPhase1RegistryForSuite(target SuiteName, deps Phase1RegistryDependencies) (*Registry, error) {
	registry := NewRegistry()

	switch target {
	case SuiteSummary:
		if err := registerPhase1SummarySuite(registry, deps); err != nil {
			return nil, err
		}
	case SuiteRewrite:
		if err := registerPhase1RewriteSuite(registry, deps); err != nil {
			return nil, err
		}
	case SuiteAll:
		if err := registerPhase1SummarySuite(registry, deps); err != nil {
			return nil, err
		}
		if err := registerPhase1RewriteSuite(registry, deps); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported phase-1 suite %q", target)
	}

	return registry, nil
}

func registerPhase1SummarySuite(registry *Registry, deps Phase1RegistryDependencies) error {
	if deps.SummaryGenerator == nil {
		return fmt.Errorf("summary generator is required")
	}
	if registry == nil {
		return fmt.Errorf("registry is required")
	}

	summaryOptions := []SummaryEvaluatorOption{WithSummaryRuntimeOptions(deps.SummaryOptions)}
	if deps.SummaryJudge != nil {
		summaryOptions = append(summaryOptions, WithSummaryJudge(deps.SummaryJudge))
	}
	if deps.SummaryAnswerGenerator != nil {
		summaryOptions = append(summaryOptions, WithSummaryAnswerGenerator(deps.SummaryAnswerGenerator))
	}
	return registry.Register(NewSummaryEvaluator(deps.SummaryGenerator, summaryOptions...))
}

func registerPhase1RewriteSuite(registry *Registry, deps Phase1RegistryDependencies) error {
	if deps.RewriteService == nil {
		return fmt.Errorf("rewrite service is required")
	}
	if registry == nil {
		return fmt.Errorf("registry is required")
	}

	rewriteOptions := []RewriteEvaluatorOption{
		WithRewriteRetrieveService(deps.RetrieveService),
		WithRewriteSubQuestionOptions(deps.RewriteSubQuestionOptions),
	}
	if len(deps.RewriteRetrievalKs) > 0 {
		rewriteOptions = append(rewriteOptions, WithRewriteRetrievalKs(deps.RewriteRetrievalKs))
	}
	if len(deps.RewriteDefaultKnowledgeBaseIDs) > 0 {
		rewriteOptions = append(rewriteOptions, WithRewriteDefaultKnowledgeBaseIDs(deps.RewriteDefaultKnowledgeBaseIDs))
	}
	if deps.RewriteEmbedding != nil {
		rewriteOptions = append(rewriteOptions, WithRewriteQueryEmbedder(NewModelPinnedQueryEmbedder(deps.RewriteEmbedding, deps.RewriteEmbeddingModelID)))
	}
	if deps.RewriteJudge != nil {
		rewriteOptions = append(rewriteOptions, WithRewriteJudge(deps.RewriteJudge))
	}
	return registry.Register(NewRewriteEvaluator(deps.RewriteService, rewriteOptions...))
}
