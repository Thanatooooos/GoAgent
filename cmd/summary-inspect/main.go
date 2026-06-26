package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	raghistory "local/rag-project/internal/app/rag/core/history"
	"local/rag-project/internal/app/rag/domain"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	infraai "local/rag-project/internal/infra-ai"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type promptHistoryFile struct {
	Results []promptHistoryTurn `json:"results"`
}

type promptHistoryTurn struct {
	Question string `json:"question"`
	Answer   string `json:"answer"`
}

type fixedModelChatService struct {
	base    aichat.LLMService
	modelID string
}

type outputEnvelope struct {
	ModelID        string         `json:"model_id"`
	SourceMessages int            `json:"source_message_count"`
	Structured     map[string]any `json:"structured_summary"`
	Rendered       string         `json:"rendered_summary"`
	Raw            string         `json:"raw_model_output"`
	Validation     map[string]any `json:"validation"`
}

func main() {
	inputPath := flag.String("input", "tmp/prompt_history_eval_results_v2.json", "prompt history input json")
	outputPath := flag.String("output", "tmp/summary_from_prompt_history_v2_qwen_max.json", "summary output json")
	modelID := flag.String("model", "qwen-max-test", "chat model id to pin for summary generation")
	configDir := flag.String("config-dir", "configs", "configuration directory")
	flag.Parse()

	if err := runSummaryInspect(*inputPath, *outputPath, *modelID, *configDir); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func loadSourceMessages(path string) ([]domain.ConversationMessage, error) {
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

	messages := make([]domain.ConversationMessage, 0, len(payload.Results)*2)
	for idx, turn := range payload.Results {
		question := strings.TrimSpace(turn.Question)
		answer := strings.TrimSpace(turn.Answer)
		if question == "" || answer == "" {
			return nil, fmt.Errorf("prompt history turn %d requires question and answer", idx)
		}
		messages = append(messages, domain.ConversationMessage{
			Role:    string(convention.UserRole),
			Content: question,
		})
		messages = append(messages, domain.ConversationMessage{
			Role:    string(convention.AssistantRole),
			Content: answer,
		})
	}
	return messages, nil
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

func runSummaryInspect(inputPath, outputPath, modelID, configDir string) error {
	loadDotEnvIfPresent(".env")
	if err := config.LoadConfig(configDir); err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	aiRuntime := infraai.NewRuntime()
	if aiRuntime == nil || aiRuntime.Chat == nil {
		return fmt.Errorf("chat runtime is unavailable")
	}

	sourceMessages, err := loadSourceMessages(inputPath)
	if err != nil {
		return err
	}

	summaryChat := fixedModelChatService{base: aiRuntime.Chat, modelID: modelID}
	output, err := raghistory.GenerateStructuredSummary(context.Background(), summaryChat, raghistory.GenerateStructuredSummaryInput{
		SourceMessages: sourceMessages,
	})
	if err != nil {
		return err
	}

	structuredRaw, err := json.Marshal(output.Structured)
	if err != nil {
		return err
	}
	validationRaw, err := json.Marshal(output.Validation)
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

	artifact := buildOutputArtifact(outputEnvelope{
		ModelID:        modelID,
		SourceMessages: len(sourceMessages),
		Structured:     structuredMap,
		Rendered:       output.Rendered,
		Raw:            output.Raw,
		Validation:     validationMap,
	})

	data, err := json.MarshalIndent(artifact, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(outputPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}

	fmt.Printf("wrote summary artifact to %s\n", outputPath)
	return nil
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
		value = strings.TrimSpace(value)
		if _, exists := os.LookupEnv(key); exists {
			continue
		}
		_ = os.Setenv(key, value)
	}
}
