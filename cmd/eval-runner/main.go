package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	raghistory "local/rag-project/internal/app/rag/core/history"
	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	return runWithDeps(args, stdout, stderr, evalRunnerDeps{})
}

type evalRunnerDeps struct {
	buildRuntime  func(context.Context, string) (*ragbootstrap.Runtime, error)
	buildRegistry func(*ragbootstrap.Runtime, rageval.SuiteName, []string) (*rageval.Registry, error)
}

func runWithDeps(args []string, stdout, stderr io.Writer, deps evalRunnerDeps) int {
	fs := flag.NewFlagSet("eval-runner", flag.ContinueOnError)
	fs.SetOutput(stderr)

	suiteFlag := fs.String("suite", "", "evaluation suite to run: summary, rewrite, all")
	inputPath := fs.String("input", "", "path to evaluation samples")
	configDir := fs.String("config-dir", "configs", "config directory used to build runtime dependencies")
	evalKBScope := fs.String("eval-kb-id", rageval.DefaultRewriteEvalKnowledgeBaseID, "rewrite eval knowledge base id; use 'all' to search every KB")
	outputPath := fs.String("output", "", "write suite JSON to this file instead of stdout")
	disableRewriteJudge := fs.Bool("no-rewrite-judge", false, "skip LLM judge for rewrite semantic quality scoring")
	disableRewriteSemantic := fs.Bool("no-rewrite-semantic", false, "skip embedding similarity for rewrite evaluation")
	rerankModel := fs.String("rerank-model", "", "override ai.rerank.default-model before loading config, e.g. qwen3-rerank or rerank-noop")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if strings.TrimSpace(*suiteFlag) == "" {
		fmt.Fprintln(stderr, "suite is required")
		return 2
	}

	suite, err := rageval.ParseSuiteName(*suiteFlag)
	if err != nil {
		fmt.Fprintf(stderr, "unsupported suite: %v\n", err)
		return 2
	}

	evalKnowledgeBaseIDs := resolveEvalKnowledgeBaseIDs(*evalKBScope)
	if model := strings.TrimSpace(*rerankModel); model != "" {
		_ = os.Setenv("AI_RERANK_DEFAULT_MODEL", model)
	}
	deps = deps.withDefaults(evalKnowledgeBaseIDs, rewriteEvalOptions{
		disableJudge:   *disableRewriteJudge,
		disableSemantic: *disableRewriteSemantic,
	})
	runtime, err := deps.buildRuntime(context.Background(), strings.TrimSpace(*configDir))
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if runtime != nil {
		defer func() { _ = runtime.Close() }()
	}

	registry, err := deps.buildRegistry(runtime, suite, evalKnowledgeBaseIDs)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	runner := rageval.NewRunner(registry)
	results, err := runner.Run(context.Background(), rageval.RunRequest{
		Suite:     suite,
		InputPath: strings.TrimSpace(*inputPath),
	})
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}

	out, closeOut, err := resolveOutputWriter(stdout, *outputPath)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	if closeOut != nil {
		defer func() {
			if closeErr := closeOut(); closeErr != nil {
				fmt.Fprintln(stderr, closeErr.Error())
			}
		}()
	}

	if err := writeResults(out, results); err != nil {
		fmt.Fprintln(stderr, err.Error())
		return 1
	}
	return 0
}

func resolveOutputWriter(stdout io.Writer, path string) (io.Writer, func() error, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return stdout, func() error { return nil }, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return file, file.Close, nil
}

func writeResults(stdout io.Writer, results []rageval.SuiteResult) error {
	encoder := json.NewEncoder(stdout)
	encoder.SetEscapeHTML(false)
	if len(results) == 1 {
		return encoder.Encode(results[0])
	}
	return encoder.Encode(results)
}

func resolveEvalKnowledgeBaseIDs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "all") {
		return nil
	}
	return []string{raw}
}

type rewriteEvalOptions struct {
	disableJudge    bool
	disableSemantic bool
}

func (d evalRunnerDeps) withDefaults(evalKnowledgeBaseIDs []string, rewriteOpts rewriteEvalOptions) evalRunnerDeps {
	if d.buildRuntime == nil {
		d.buildRuntime = func(ctx context.Context, configDir string) (*ragbootstrap.Runtime, error) {
			if err := config.LoadConfig(configDir); err != nil {
				return nil, fmt.Errorf("load config failed: %w", err)
			}
			runtime, err := ragbootstrap.NewRuntime(ctx, ragbootstrap.RuntimeOptions{})
			if err != nil {
				return nil, fmt.Errorf("build rag runtime failed: %w", err)
			}
			return runtime, nil
		}
	}
	if d.buildRegistry == nil {
		kbIDs := append([]string(nil), evalKnowledgeBaseIDs...)
		d.buildRegistry = func(runtime *ragbootstrap.Runtime, suite rageval.SuiteName, _ []string) (*rageval.Registry, error) {
			return buildPhase1Registry(runtime, suite, kbIDs, rewriteOpts)
		}
	}
	return d
}

func buildPhase1Registry(runtime *ragbootstrap.Runtime, suite rageval.SuiteName, evalKnowledgeBaseIDs []string, rewriteOpts rewriteEvalOptions) (*rageval.Registry, error) {
	if runtime == nil {
		return nil, fmt.Errorf("rag runtime is required")
	}

	registryDeps := rageval.Phase1RegistryDependencies{
		RetrieveService:                runtime.Retrieve,
		RewriteRetrievalKs:             []int{1, 3, 5},
		RewriteDefaultKnowledgeBaseIDs: append([]string(nil), evalKnowledgeBaseIDs...),
		RewriteSubQuestionOptions: ragretrieve.SubQuestionOptions{
			ParallelEnabled: true,
			MaxConcurrency:  2,
		},
	}
	if cfg := config.Get(); cfg != nil {
		registryDeps.RewriteSubQuestionOptions = ragretrieve.SubQuestionOptions{
			ParallelEnabled: cfg.Rag.Retrieve.ParallelSubquestions.Enabled,
			MaxConcurrency:  cfg.Rag.Retrieve.ParallelSubquestions.MaxConcurrency,
		}
		registryDeps.RewriteEmbeddingModelID = strings.TrimSpace(cfg.AI.Embedding.DefaultModel)
	}

	switch suite {
	case rageval.SuiteSummary:
		if runtime.LLMChat == nil {
			return nil, fmt.Errorf("llm chat service is unavailable")
		}
		registryDeps.SummaryGenerator = rageval.NewHistorySummaryGenerator(runtime.LLMChat, raghistory.SummaryBudgetOptions{})
		registryDeps.SummaryJudge = rageval.NewPromptFileJudge(runtime.LLMChat, "")
		registryDeps.SummaryAnswerGenerator = rageval.NewPromptSummaryAnswerGenerator(nil, runtime.LLMChat, rageval.SummaryAnswerConfig{})
	case rageval.SuiteRewrite:
		if runtime.Rewrite == nil {
			return nil, fmt.Errorf("rewrite service is unavailable")
		}
		if runtime.Retrieve == nil {
			return nil, fmt.Errorf("retrieve service is unavailable")
		}
		registryDeps.RewriteService = runtime.Rewrite
		if !rewriteOpts.disableSemantic && runtime.Embedding != nil {
			registryDeps.RewriteEmbedding = runtime.Embedding
		}
		if !rewriteOpts.disableJudge && runtime.LLMChat != nil {
			registryDeps.RewriteJudge = rageval.NewPromptFileJudge(runtime.LLMChat, "")
		}
	case rageval.SuiteAll:
		if runtime.LLMChat == nil {
			return nil, fmt.Errorf("llm chat service is unavailable")
		}
		if runtime.Rewrite == nil {
			return nil, fmt.Errorf("rewrite service is unavailable")
		}
		if runtime.Retrieve == nil {
			return nil, fmt.Errorf("retrieve service is unavailable")
		}
		registryDeps.SummaryGenerator = rageval.NewHistorySummaryGenerator(runtime.LLMChat, raghistory.SummaryBudgetOptions{})
		registryDeps.SummaryJudge = rageval.NewPromptFileJudge(runtime.LLMChat, "")
		registryDeps.SummaryAnswerGenerator = rageval.NewPromptSummaryAnswerGenerator(nil, runtime.LLMChat, rageval.SummaryAnswerConfig{})
		registryDeps.RewriteService = runtime.Rewrite
		if !rewriteOpts.disableSemantic && runtime.Embedding != nil {
			registryDeps.RewriteEmbedding = runtime.Embedding
		}
		if !rewriteOpts.disableJudge && runtime.LLMChat != nil {
			registryDeps.RewriteJudge = rageval.NewPromptFileJudge(runtime.LLMChat, "")
		}
	default:
		return nil, fmt.Errorf("unsupported suite %q", suite)
	}

	return rageval.NewPhase1RegistryForSuite(suite, registryDeps)
}
