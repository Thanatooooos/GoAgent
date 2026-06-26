# Summary Processing from Prompt History V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Run the existing structured summary generation path on `tmp/prompt_history_eval_results_v2.json` with a fixed `qwen-max` model and save the resulting structured summary, rendered summary, raw model output, and validation artifact for manual review.

**Architecture:** Add one small repo-local Go command that reads the saved multi-turn Q&A JSON, converts it into ordered summary source messages, wraps the existing chat runtime with a fixed-model adapter, and calls `internal/app/rag/core/history.GenerateStructuredSummary(...)`. Keep the work out of production paths and persist output as a temp artifact JSON file for repeatable inspection.

**Tech Stack:** Go, existing `internal/app/rag/core/history` summary generation, existing `internal/bootstrap/rag` runtime wiring, JSON artifact output

---

## File Map

- Create: `cmd/summary-inspect/main.go`
  - one-off but reusable local command for summary generation inspection
- Create: `cmd/summary-inspect/main_test.go`
  - focused tests for input loading, message mapping order, and fixed-model wrapper behavior
- Read only: `tmp/prompt_history_eval_results_v2.json`
  - existing multi-turn Q&A input artifact
- Emit at runtime: `tmp/summary_from_prompt_history_v2_qwen_max.json`
  - saved summary inspection artifact

### Task 1: Build the prompt-history loader and source-message mapper

**Files:**
- Create: `cmd/summary-inspect/main.go`
- Create: `cmd/summary-inspect/main_test.go`

- [ ] **Step 1: Write the failing tests for input loading and message mapping**

```go
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPromptHistoryBuildsOrderedSourceMessages(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "prompt-history.json")
	payload := map[string]any{
		"results": []map[string]any{
			{"question": "Q1", "answer": "A1"},
			{"question": "Q2", "answer": "A2"},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(inputPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	messages, err := loadSourceMessages(inputPath)
	if err != nil {
		t.Fatalf("loadSourceMessages() error = %v", err)
	}
	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	if messages[0].Role != "user" || messages[0].Content != "Q1" {
		t.Fatalf("messages[0] = %+v, want user Q1", messages[0])
	}
	if messages[1].Role != "assistant" || messages[1].Content != "A1" {
		t.Fatalf("messages[1] = %+v, want assistant A1", messages[1])
	}
	if messages[2].Role != "user" || messages[2].Content != "Q2" {
		t.Fatalf("messages[2] = %+v, want user Q2", messages[2])
	}
	if messages[3].Role != "assistant" || messages[3].Content != "A2" {
		t.Fatalf("messages[3] = %+v, want assistant A2", messages[3])
	}
}

func TestLoadPromptHistoryRejectsMissingPairs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	inputPath := filepath.Join(dir, "prompt-history.json")
	payload := map[string]any{
		"results": []map[string]any{
			{"question": "Q1", "answer": ""},
		},
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if err := os.WriteFile(inputPath, raw, 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	if _, err := loadSourceMessages(inputPath); err == nil {
		t.Fatal("loadSourceMessages() expected error for empty answer")
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./cmd/summary-inspect -run 'TestLoadPromptHistory' -v`

Expected: FAIL with undefined `loadSourceMessages`

- [ ] **Step 3: Write the minimal loader and mapper implementation**

```go
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

type promptHistoryFile struct {
	Results []promptHistoryTurn `json:"results"`
}

type promptHistoryTurn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

func loadSourceMessages(path string) ([]rageval.SummaryMessage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var payload promptHistoryFile
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode prompt history: %w", err)
	}
	if len(payload.Results) == 0 {
		return nil, fmt.Errorf("prompt history results are required")
	}

	messages := make([]rageval.SummaryMessage, 0, len(payload.Results)*2)
	for idx, turn := range payload.Results {
		question := strings.TrimSpace(turn.Question)
		answer := strings.TrimSpace(turn.Answer)
		if question == "" || answer == "" {
			return nil, fmt.Errorf("prompt history turn %d requires question and answer", idx)
		}
		messages = append(messages, rageval.SummaryMessage{Role: "user", Content: question})
		messages = append(messages, rageval.SummaryMessage{Role: "assistant", Content: answer})
	}
	return messages, nil
}
```

- [ ] **Step 4: Run the tests to verify the mapper passes**

Run: `go test ./cmd/summary-inspect -run 'TestLoadPromptHistory' -v`

Expected: PASS

- [ ] **Step 5: Commit the loader and tests**

```bash
git add cmd/summary-inspect/main.go cmd/summary-inspect/main_test.go
git commit -m "feat: add prompt history loader for summary inspection"
```

### Task 2: Add fixed-model summary execution and artifact output

**Files:**
- Modify: `cmd/summary-inspect/main.go`
- Modify: `cmd/summary-inspect/main_test.go`

- [ ] **Step 1: Write the failing tests for fixed-model delegation and artifact shape**

```go
func TestFixedModelChatServiceUsesConfiguredModel(t *testing.T) {
	t.Parallel()

	base := &chatServiceStub{response: `{"schema_version":1,"goal":"g"}`}
	svc := fixedModelChatService{base: base, modelID: "qwen-max-test"}

	jsonMode := true
	_, err := svc.ChatWithRequest(convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage("hi")},
		JSONMode: &jsonMode,
	})
	if err != nil {
		t.Fatalf("ChatWithRequest() error = %v", err)
	}
	if base.lastModelID != "qwen-max-test" {
		t.Fatalf("lastModelID = %q, want qwen-max-test", base.lastModelID)
	}
}

func TestBuildOutputArtifactIncludesSummaryFields(t *testing.T) {
	t.Parallel()

	artifact := buildOutputArtifact(outputEnvelope{
		ModelID: "qwen-max-test",
		SourceMessages: 20,
		Structured: map[string]any{"goal": "design rag"},
		Rendered: "目标：design rag",
		Raw: `{"goal":"design rag"}`,
		Validation: map[string]any{"valid": true},
	})

	if artifact["model_id"] != "qwen-max-test" {
		t.Fatalf("model_id = %v, want qwen-max-test", artifact["model_id"])
	}
	if artifact["rendered_summary"] == "" {
		t.Fatal("rendered_summary expected non-empty")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./cmd/summary-inspect -run 'TestFixedModelChatService|TestBuildOutputArtifact' -v`

Expected: FAIL with undefined `fixedModelChatService`, `buildOutputArtifact`, or `outputEnvelope`

- [ ] **Step 3: Implement the fixed-model wrapper, summary call, and output envelope**

```go
type fixedModelChatService struct {
	base    aichat.LLMService
	modelID string
}

func (s fixedModelChatService) Chat(prompt string) (string, error) {
	return s.ChatWithRequest(convention.ChatRequest{
		Messages: []convention.ChatMessage{convention.UserMessage(prompt)},
	})
}

func (s fixedModelChatService) ChatWithRequest(request convention.ChatRequest) (string, error) {
	return s.base.ChatWithModel(request, s.modelID)
}

func (s fixedModelChatService) ChatWithModel(request convention.ChatRequest, modelID string) (string, error) {
	return s.base.ChatWithModel(request, modelID)
}

func (s fixedModelChatService) StreamChat(string, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, fmt.Errorf("streaming not supported in summary-inspect")
}

func (s fixedModelChatService) StreamChatWithRequest(convention.ChatRequest, aichat.StreamCallback) (aichat.StreamCancellationHandle, error) {
	return nil, fmt.Errorf("streaming not supported in summary-inspect")
}

type outputEnvelope struct {
	ModelID        string         `json:"model_id"`
	SourceMessages int            `json:"source_message_count"`
	Structured     map[string]any `json:"structured_summary"`
	Rendered       string         `json:"rendered_summary"`
	Raw            string         `json:"raw_model_output"`
	Validation     map[string]any `json:"validation"`
}

func buildOutputArtifact(out outputEnvelope) map[string]any {
	return map[string]any{
		"model_id":             out.ModelID,
		"source_message_count": out.SourceMessages,
		"structured_summary":   out.Structured,
		"rendered_summary":     out.Rendered,
		"raw_model_output":     out.Raw,
		"validation":           out.Validation,
	}
}

func runSummaryInspect(inputPath, outputPath, modelID string) error {
	if err := config.LoadConfig("configs"); err != nil {
		return err
	}
	runtime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{})
	if err != nil {
		return err
	}
	defer runtime.Close()

	sourceMessages, err := loadSourceMessages(inputPath)
	if err != nil {
		return err
	}

	summaryChat := fixedModelChatService{base: runtime.LLMChat, modelID: modelID}
	output, err := rageval.NewHistorySummaryGenerator(summaryChat, raghistory.SummaryBudgetOptions{}).Generate(
		context.Background(),
		rageval.SummaryGenerationInput{SourceMessages: sourceMessages},
	)
	if err != nil {
		return err
	}

	structuredRaw, err := json.Marshal(output.Structured)
	if err != nil {
		return err
	}
	validationRaw, err := json.Marshal(output.Structured)
	if err != nil {
		return err
	}

	var structuredMap map[string]any
	if err := json.Unmarshal(structuredRaw, &structuredMap); err != nil {
		return err
	}
	var validationMap map[string]any
	if err := json.Unmarshal(validationRaw, &validationMap); err != nil {
		return err
	}

	filePayload := buildOutputArtifact(outputEnvelope{
		ModelID:        modelID,
		SourceMessages: len(sourceMessages),
		Structured:     structuredMap,
		Rendered:       output.Rendered,
		Raw:            output.Raw,
		Validation:     validationMap,
	})

	data, err := json.MarshalIndent(filePayload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(outputPath, data, 0o644)
}
```

- [ ] **Step 4: Add the tiny CLI shell and correct the validation serialization**

```go
func main() {
	inputPath := flag.String("input", "tmp/prompt_history_eval_results_v2.json", "prompt history input json")
	outputPath := flag.String("output", "tmp/summary_from_prompt_history_v2_qwen_max.json", "summary output json")
	modelID := flag.String("model", "qwen-max-test", "chat model id to pin for summary generation")
	flag.Parse()

	if err := runSummaryInspect(*inputPath, *outputPath, *modelID); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

// Replace the temporary validationRaw line above with:
validationRaw, err := json.Marshal(output.Validation)
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./cmd/summary-inspect -v`

Expected: PASS

- [ ] **Step 6: Commit the command implementation**

```bash
git add cmd/summary-inspect/main.go cmd/summary-inspect/main_test.go
git commit -m "feat: add summary inspection command"
```

### Task 3: Generate and inspect the qwen-max summary artifact

**Files:**
- Read: `tmp/prompt_history_eval_results_v2.json`
- Generate: `tmp/summary_from_prompt_history_v2_qwen_max.json`

- [ ] **Step 1: Run the command on the saved 10-turn prompt history**

Run: `go run ./cmd/summary-inspect -input tmp/prompt_history_eval_results_v2.json -model qwen-max-test -output tmp/summary_from_prompt_history_v2_qwen_max.json`

Expected: exit code `0` and a new artifact file at `tmp/summary_from_prompt_history_v2_qwen_max.json`

- [ ] **Step 2: Inspect the saved output artifact**

Run: `Get-Content -Raw 'D:\goagent\tmp\summary_from_prompt_history_v2_qwen_max.json'`

Expected: JSON containing:

```json
{
  "model_id": "qwen-max-test",
  "source_message_count": 20,
  "structured_summary": {},
  "rendered_summary": "",
  "raw_model_output": "",
  "validation": {}
}
```

- [ ] **Step 3: Review the summary against the approved checklist**

Check manually:

```text
1. goal 是否抓住“企业知识库问答 + 检索链路架构决策”
2. constraints 是否保住“PostgreSQL 主库 + 暂不想养独立系统”
3. established_facts 是否把讨论误写成已落地事实
4. recent_progress / open_questions 是否保住后半段 rewrite / eval / scale / fault isolation
```

- [ ] **Step 4: Record the verdict in the handoff note**

```text
- broadly usable
- lossy but fixable
- fundamentally drifting
```

- [ ] **Step 5: Commit the command only, not the temp artifact**

```bash
git add cmd/summary-inspect/main.go cmd/summary-inspect/main_test.go
git commit -m "feat: add prompt-history summary inspection workflow"
```

## Spec Coverage Check

- Uses the approved `prompt_history_eval_results_v2.json` input directly: covered by Task 1 and Task 3.
- Uses the existing summary generation path instead of `eval-runner`: covered by Task 2.
- Pins `qwen-max` as the first-pass model: covered by Task 2 and Task 3.
- Saves four required artifacts: covered by Task 2 and Task 3.
- Supports first-pass manual review before any formal eval work: covered by Task 3.

