package evaluation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const defaultSharedEvalAssetRoot = "testdata/evals/shared"

type PromptFileJudge struct {
	chatService aichat.LLMService
	assetRoot   string
}

func NewPromptFileJudge(chatService aichat.LLMService, assetRoot string) *PromptFileJudge {
	if strings.TrimSpace(assetRoot) == "" {
		assetRoot = resolveSharedEvalAssetRoot()
	}
	return &PromptFileJudge{
		chatService: chatService,
		assetRoot:   assetRoot,
	}
}

func (j *PromptFileJudge) Evaluate(_ context.Context, req JudgeRequest) (JudgeResult, error) {
	if j == nil || j.chatService == nil {
		return JudgeResult{}, fmt.Errorf("judge chat service is required")
	}
	promptText, err := loadSharedEvalMarkdown(j.assetRoot, "judge_prompts", req.PromptRef)
	if err != nil {
		return JudgeResult{}, err
	}
	rubricText, err := loadSharedEvalMarkdown(j.assetRoot, "rubrics", req.RubricRef)
	if err != nil {
		return JudgeResult{}, err
	}
	payload, err := json.MarshalIndent(req.Payload, "", "  ")
	if err != nil {
		return JudgeResult{}, fmt.Errorf("marshal judge payload: %w", err)
	}

	cfg := normalizeJudgeConfig(req.Config)
	attempts := []bool{true, false, false}
	var lastErr error
	for i, jsonMode := range attempts {
		request := buildJudgeChatRequest(promptText, rubricText, string(payload), cfg, jsonMode)
		result, err := j.evaluateRequest(request, cfg)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if !shouldRetryJudgeResponse(err) || i == len(attempts)-1 {
			return JudgeResult{}, err
		}
	}
	return JudgeResult{}, lastErr
}

func (j *PromptFileJudge) evaluateRequest(request convention.ChatRequest, cfg JudgeConfig) (JudgeResult, error) {
	var raw string
	var err error
	if strings.TrimSpace(cfg.Model) != "" {
		raw, err = j.chatService.ChatWithModel(request, cfg.Model)
	} else {
		raw, err = j.chatService.ChatWithRequest(request)
	}
	if err != nil {
		return JudgeResult{}, fmt.Errorf("judge call failed: %w", err)
	}
	result, err := parseJudgeResult(raw)
	if err != nil {
		return JudgeResult{}, err
	}
	return result, nil
}

func buildJudgeChatRequest(promptText, rubricText, payload string, cfg JudgeConfig, jsonMode bool) convention.ChatRequest {
	return convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(buildJudgeSystemPrompt(promptText, rubricText)),
			convention.UserMessage("Judge payload:\n" + payload),
		},
		Temperature: &cfg.Temperature,
		MaxTokens:   &cfg.MaxTokens,
		JSONMode:    &jsonMode,
	}
}

func buildJudgeSystemPrompt(promptText, rubricText string) string {
	return strings.TrimSpace("You are an offline evaluation judge.\n\nPrompt template:\n" + promptText + "\n\nRubric:\n" + rubricText + "\n\nReturn strict JSON only with keys: passed, score, missed_items, incorrect_claims, reason, details.\n- passed must be boolean\n- score must be a number between 0 and 1\n- details may contain prompt-specific structured data\nDo not wrap the response in prose.")
}

func normalizeJudgeConfig(cfg JudgeConfig) JudgeConfig {
	if cfg.MaxTokens <= 0 {
		cfg.MaxTokens = 800
	}
	return cfg
}

func parseJudgeResult(raw string) (JudgeResult, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return JudgeResult{}, &JudgeParseError{Raw: raw, Cause: fmt.Errorf("judge response is empty")}
	}
	if extracted := extractJSONBlock(trimmed); extracted != "" {
		trimmed = extracted
	}

	var result JudgeResult
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return JudgeResult{}, &JudgeParseError{Raw: raw, Cause: err}
	}
	return result, nil
}

func shouldRetryJudgeResponse(err error) bool {
	var parseErr *JudgeParseError
	return errors.As(err, &parseErr)
}

func loadSharedEvalMarkdown(root, folder, ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("eval asset ref is required")
	}
	if strings.Contains(ref, "..") {
		return "", fmt.Errorf("invalid eval asset ref %q", ref)
	}
	path := filepath.Join(root, folder, ref+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read eval asset %q: %w", path, err)
	}
	return strings.TrimSpace(string(data)), nil
}

func resolveSharedEvalAssetRoot() string {
	cwd, err := os.Getwd()
	if err != nil {
		return defaultSharedEvalAssetRoot
	}
	current := cwd
	for {
		candidate := filepath.Join(current, filepath.FromSlash(defaultSharedEvalAssetRoot))
		if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.FromSlash(defaultSharedEvalAssetRoot)
		}
		current = parent
	}
}

func extractJSONBlock(raw string) string {
	marker := "```json"
	start := strings.Index(raw, marker)
	if start == -1 {
		marker = "```"
		start = strings.Index(raw, marker)
	}
	if start == -1 {
		return ""
	}
	contentStart := strings.IndexByte(raw[start:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += start + 1
	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}
