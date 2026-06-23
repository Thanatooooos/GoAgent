package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	rageval "local/rag-project/internal/app/rag/evaluation"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type summaryDialogGenOptions struct {
	ScriptPath string
	OutputPath string
	DraftPath  string
	ModelID    string
	ConfigDir  string
	Provider   string
	Overhead   int
	Overwrite  bool
}

type summaryDialogGenDeps struct {
	LoadConfig func(string) error
	NewChat    func() aichat.LLMService
}

type summaryDialogFileStore struct {
	path string
}

func main() {
	opts, err := parseSummaryDialogGenArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(2)
	}
	if err := runSummaryDialogGen(opts, defaultSummaryDialogGenDeps()); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func parseSummaryDialogGenArgs(args []string) (summaryDialogGenOptions, error) {
	opts := summaryDialogGenOptions{
		ScriptPath: "testdata/evals/summary/long_dialogue_questions.json",
		OutputPath: "tmp/software_project_state_transitions_v1_raw.json",
		ModelID:    "qwen-max-test",
		ConfigDir:  "configs",
		Provider:   "configured",
		Overhead:   4,
	}
	fs := flag.NewFlagSet("summary-dialog-gen", flag.ContinueOnError)
	fs.StringVar(&opts.ScriptPath, "script", opts.ScriptPath, "controlled question script path")
	fs.StringVar(&opts.OutputPath, "output", opts.OutputPath, "raw generation artifact path")
	fs.StringVar(&opts.DraftPath, "draft-output", "", "optional review draft output path")
	fs.StringVar(&opts.ModelID, "model", opts.ModelID, "chat model id")
	fs.StringVar(&opts.ConfigDir, "config-dir", opts.ConfigDir, "configuration directory")
	fs.StringVar(&opts.Provider, "provider", opts.Provider, "provider label persisted in the raw artifact")
	fs.IntVar(&opts.Overhead, "message-overhead", opts.Overhead, "message overhead tokens")
	fs.BoolVar(&opts.Overwrite, "overwrite", false, "ignore existing raw output and start from turn 1")
	if err := fs.Parse(args); err != nil {
		return summaryDialogGenOptions{}, err
	}
	return opts, nil
}

func defaultSummaryDialogGenDeps() summaryDialogGenDeps {
	return summaryDialogGenDeps{
		LoadConfig: config.LoadConfig,
		NewChat: func() aichat.LLMService {
			runtime := infraai.NewRuntime()
			if runtime == nil {
				return nil
			}
			return runtime.Chat
		},
	}
}

func runSummaryDialogGen(opts summaryDialogGenOptions, deps summaryDialogGenDeps) error {
	rawScript, err := os.ReadFile(opts.ScriptPath)
	if err != nil {
		return fmt.Errorf("read summary dialog script: %w", err)
	}
	script, err := rageval.ParseSummaryDialogScript(rawScript)
	if err != nil {
		return err
	}
	modelID := strings.TrimSpace(opts.ModelID)
	if modelID == "" {
		return fmt.Errorf("model is required")
	}
	provider := strings.TrimSpace(opts.Provider)
	if provider == "" {
		provider = "configured"
	}

	var existing *rageval.SummaryDialogArtifact
	if !opts.Overwrite {
		loaded, err := rageval.LoadSummaryDialogArtifact(opts.OutputPath)
		if err == nil {
			existing = &loaded
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("load existing summary dialog artifact: %w", err)
		}
	}

	loadDotEnvIfPresent(".env")
	loadConfig := deps.LoadConfig
	if loadConfig == nil {
		loadConfig = config.LoadConfig
	}
	if err := loadConfig(opts.ConfigDir); err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	newChat := deps.NewChat
	if newChat == nil {
		newChat = defaultSummaryDialogGenDeps().NewChat
	}
	chat := newChat()
	if chat == nil {
		return fmt.Errorf("chat runtime is unavailable")
	}

	artifact, err := rageval.GenerateSummaryDialog(context.Background(), rageval.SummaryDialogGenerationInput{
		Script:                script,
		Existing:              existing,
		ModelID:               modelID,
		Provider:              provider,
		Chat:                  chat,
		Estimator:             tokenbudget.NewDefaultEstimator(),
		MessageOverheadTokens: opts.Overhead,
		Store:                 summaryDialogFileStore{path: opts.OutputPath},
	})
	if err != nil {
		return err
	}
	if opts.DraftPath != "" && artifact.Status == rageval.SummaryDialogStatusComplete {
		draft, err := rageval.BuildSummaryDialogReviewDraft(artifact)
		if err != nil {
			return err
		}
		if err := writeSummaryDialogJSON(opts.DraftPath, []rageval.SummarySample{draft}); err != nil {
			return err
		}
	}
	printSummaryDialogResult(opts, artifact)
	return nil
}

func (s summaryDialogFileStore) Save(artifact rageval.SummaryDialogArtifact) error {
	return rageval.WriteSummaryDialogArtifact(s.path, artifact)
}

func writeSummaryDialogJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("ensure summary dialog output directory: %w", err)
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode summary dialog output: %w", err)
	}
	return os.WriteFile(path, data, 0o644)
}

func printSummaryDialogResult(opts summaryDialogGenOptions, artifact rageval.SummaryDialogArtifact) {
	keys := make([]string, 0, len(artifact.Suitability.CrossedAt))
	for key := range artifact.Suitability.CrossedAt {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	crossings := make([]string, 0, len(keys))
	for _, key := range keys {
		crossings = append(crossings, fmt.Sprintf("%s:%d", key, artifact.Suitability.CrossedAt[key]))
	}
	fmt.Printf("completed_turns=%d\n", len(artifact.Turns))
	fmt.Printf("status=%s\n", artifact.Status)
	fmt.Printf("final_tokens=%d\n", artifact.Suitability.FinalTokens)
	fmt.Printf("crossed_at=%s\n", strings.Join(crossings, ","))
	fmt.Printf("suitable=%v\n", artifact.Suitability.Suitable)
	fmt.Printf("raw_output=%s\n", opts.OutputPath)
	if opts.DraftPath != "" {
		fmt.Printf("draft_output=%s\n", opts.DraftPath)
	}
}

func loadDotEnvIfPresent(path string) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(raw), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, strings.TrimSpace(value))
	}
}
