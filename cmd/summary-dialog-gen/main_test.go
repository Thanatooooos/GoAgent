package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	rageval "local/rag-project/internal/app/rag/evaluation"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

func TestParseSummaryDialogGenArgsUsesDefaults(t *testing.T) {
	opts, err := parseSummaryDialogGenArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if opts.ScriptPath != "testdata/evals/summary/long_dialogue_questions.json" {
		t.Fatalf("script path = %q", opts.ScriptPath)
	}
	if opts.OutputPath != "tmp/software_project_state_transitions_v1_raw.json" {
		t.Fatalf("output path = %q", opts.OutputPath)
	}
	if opts.DraftPath != "" {
		t.Fatalf("draft path = %q, want empty", opts.DraftPath)
	}
	if opts.ModelID != "qwen-max-test" {
		t.Fatalf("model = %q", opts.ModelID)
	}
}

func TestRunSummaryDialogGenRejectsEmptyModelBeforeRuntime(t *testing.T) {
	scriptPath := writeSummaryDialogScriptFile(t)
	deps := summaryDialogGenDeps{
		LoadConfig: func(string) error { return nil },
		NewChat: func() aichat.LLMService {
			t.Fatal("NewChat should not be called")
			return nil
		},
	}
	err := runSummaryDialogGen(summaryDialogGenOptions{
		ScriptPath: scriptPath,
		OutputPath: filepath.Join(t.TempDir(), "raw.json"),
		ModelID:    " ",
		ConfigDir:  "configs",
		Provider:   "configured",
		Overhead:   4,
	}, deps)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunSummaryDialogGenRejectsMalformedScriptBeforeRuntime(t *testing.T) {
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(scriptPath, []byte(`{"schema_version":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runSummaryDialogGen(summaryDialogGenOptions{
		ScriptPath: scriptPath,
		OutputPath: filepath.Join(dir, "raw.json"),
		ModelID:    "model-a",
		ConfigDir:  "configs",
		Provider:   "configured",
		Overhead:   4,
	}, summaryDialogGenDeps{
		LoadConfig: func(string) error { return nil },
		NewChat: func() aichat.LLMService {
			t.Fatal("NewChat should not be called")
			return nil
		},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunSummaryDialogGenResumesExistingArtifact(t *testing.T) {
	dir := t.TempDir()
	scriptPath := writeSummaryDialogScriptFile(t)
	outputPath := filepath.Join(dir, "raw.json")
	existing := summaryDialogArtifactWithTurns(23)
	if err := rageval.WriteSummaryDialogArtifact(outputPath, existing); err != nil {
		t.Fatal(err)
	}
	chat := &summaryDialogGenLLMFake{responses: []string{"a24"}}

	err := runSummaryDialogGen(summaryDialogGenOptions{
		ScriptPath: scriptPath,
		OutputPath: outputPath,
		ModelID:    "model-a",
		ConfigDir:  "configs",
		Provider:   "configured",
		Overhead:   4,
	}, summaryDialogGenDeps{
		LoadConfig: func(string) error { return nil },
		NewChat:    func() aichat.LLMService { return chat },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.requests) != 1 {
		t.Fatalf("requests = %d, want 1", len(chat.requests))
	}
	loaded, err := rageval.LoadSummaryDialogArtifact(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Turns) != 24 || loaded.Status != rageval.SummaryDialogStatusComplete {
		t.Fatalf("loaded artifact = %+v", loaded)
	}
}

func TestRunSummaryDialogGenOverwriteStartsFromFirstTurn(t *testing.T) {
	dir := t.TempDir()
	scriptPath := writeSummaryDialogScriptFile(t)
	outputPath := filepath.Join(dir, "raw.json")
	if err := rageval.WriteSummaryDialogArtifact(outputPath, summaryDialogArtifactWithTurns(23)); err != nil {
		t.Fatal(err)
	}
	chat := &summaryDialogGenLLMFake{responses: repeatedSummaryDialogAnswers(24)}

	err := runSummaryDialogGen(summaryDialogGenOptions{
		ScriptPath: scriptPath,
		OutputPath: outputPath,
		ModelID:    "model-a",
		ConfigDir:  "configs",
		Provider:   "configured",
		Overhead:   4,
		Overwrite:  true,
	}, summaryDialogGenDeps{
		LoadConfig: func(string) error { return nil },
		NewChat:    func() aichat.LLMService { return chat },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(chat.requests) != 24 {
		t.Fatalf("requests = %d, want 24", len(chat.requests))
	}
}

func TestRunSummaryDialogGenWritesDraftWhenRequested(t *testing.T) {
	dir := t.TempDir()
	draftPath := filepath.Join(dir, "draft.json")
	chat := &summaryDialogGenLLMFake{responses: repeatedSummaryDialogAnswers(24)}

	err := runSummaryDialogGen(summaryDialogGenOptions{
		ScriptPath: writeSummaryDialogScriptFile(t),
		OutputPath: filepath.Join(dir, "raw.json"),
		DraftPath:  draftPath,
		ModelID:    "model-a",
		ConfigDir:  "configs",
		Provider:   "configured",
		Overhead:   4,
	}, summaryDialogGenDeps{
		LoadConfig: func(string) error { return nil },
		NewChat:    func() aichat.LLMService { return chat },
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(draftPath)
	if err != nil {
		t.Fatal(err)
	}
	var samples []rageval.SummarySample
	if err := json.Unmarshal(raw, &samples); err != nil {
		t.Fatal(err)
	}
	if len(samples) != 1 || len(samples[0].Input.SourceMessages) != 48 {
		t.Fatalf("unexpected draft: %+v", samples)
	}
}

type summaryDialogGenLLMFake struct {
	responses []string
	requests  []convention.ChatRequest
}

func (f *summaryDialogGenLLMFake) Chat(string) (string, error) {
	return "", fmt.Errorf("Chat should not be used")
}

func (f *summaryDialogGenLLMFake) ChatWithRequest(convention.ChatRequest) (string, error) {
	return "", fmt.Errorf("ChatWithRequest should not be used")
}

func (f *summaryDialogGenLLMFake) ChatWithModel(request convention.ChatRequest, _ string) (string, error) {
	index := len(f.requests)
	f.requests = append(f.requests, request)
	if index >= len(f.responses) {
		return "", fmt.Errorf("missing fake response")
	}
	return f.responses[index], nil
}

func (f *summaryDialogGenLLMFake) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, fmt.Errorf("streaming is not used")
}

func (f *summaryDialogGenLLMFake) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, fmt.Errorf("streaming is not used")
}

func writeSummaryDialogScriptFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "script.json")
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "evals", "summary", "long_dialogue_questions.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func summaryDialogArtifactWithTurns(count int) rageval.SummaryDialogArtifact {
	artifact := rageval.SummaryDialogArtifact{
		SchemaVersion: 1,
		ScenarioID:    "software_project_state_transitions_v1",
		Status:        rageval.SummaryDialogStatusInProgress,
		Provider:      "configured",
		Model:         "model-a",
		Estimator: rageval.SummaryDialogEstimatorMetadata{
			Name:                  "tokenestimate",
			Version:               "v0.1.0",
			MessageOverheadTokens: 4,
		},
	}
	for i := 0; i < count; i++ {
		artifact.Turns = append(artifact.Turns, rageval.SummaryDialogGeneratedTurn{
			Turn:      i + 1,
			Phase:     "phase",
			Purpose:   "purpose",
			User:      fmt.Sprintf("q%d", i+1),
			Assistant: fmt.Sprintf("a%d", i+1),
		})
	}
	return artifact
}

func repeatedSummaryDialogAnswers(count int) []string {
	answers := make([]string, count)
	for i := range answers {
		answers[i] = fmt.Sprintf("a%d", i+1)
	}
	return answers
}
