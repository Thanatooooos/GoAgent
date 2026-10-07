package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const Model = "Qwen/Qwen3.8-27B"
const PromptVersion = "visible-facts-v1"

const prompt = `请仅根据图片中可见的内容，用简体中文描述图片。说明可见对象、文字、图表结构、数值和对象关系；看不清的细节明确说不清。不要推断图片之外的事实，不要使用相邻正文。如果图片没有可描述的内容，仅输出 NO_CONTENT。`

type Result struct {
	Status        string // described or no_content
	Text          string
	Model         string
	PromptVersion string
	InputTokens   int
	OutputTokens  int
}

type RequestError struct {
	Code       string
	HTTPStatus int
	Permanent  bool
}

func (e *RequestError) Error() string {
	if e.HTTPStatus != 0 {
		return fmt.Sprintf("vision provider HTTP %d", e.HTTPStatus)
	}
	return "vision " + e.Code
}

// Classify exposes stable, safe error categories without provider body text.
func Classify(err error) (string, bool) {
	var requestError *RequestError
	if errors.As(err, &requestError) {
		return requestError.Code, requestError.Permanent
	}
	var networkError net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &networkError) && networkError.Timeout()) {
		return "vision_timeout", false
	}
	return "vision_request", false
}

type Client struct {
	URL       string
	APIKey    string
	HTTP      *http.Client
	MaxTokens int
}

func NewClient(url, apiKey string, timeout time.Duration, maxTokens int) *Client {
	if timeout <= 0 {
		timeout = 90 * time.Second
	}
	if maxTokens <= 0 {
		maxTokens = 512
	}
	return &Client{URL: strings.TrimRight(url, "/"), APIKey: apiKey, HTTP: &http.Client{Timeout: timeout}, MaxTokens: maxTokens}
}

func (c *Client) Describe(ctx context.Context, image []byte, mimeType string) (Result, error) {
	if c == nil || c.URL == "" || c.APIKey == "" {
		return Result{}, &RequestError{Code: "vision_unconfigured", Permanent: true}
	}
	if len(image) == 0 || len(image) > 10<<20 {
		return Result{}, &RequestError{Code: "vision_image_size", Permanent: true}
	}
	switch mimeType {
	case "image/jpeg", "image/png", "image/webp", "image/gif":
	default:
		return Result{}, &RequestError{Code: "vision_image_format", Permanent: true}
	}
	requestBody := map[string]any{
		"model":      Model,
		"max_tokens": c.MaxTokens,
		"stream":     false,
		"messages": []any{map[string]any{
			"role": "user",
			"content": []any{
				map[string]any{"type": "text", "text": prompt},
				map[string]any{"type": "image_url", "image_url": map[string]string{
					"url": "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(image),
				}},
			},
		}},
	}
	encoded, err := json.Marshal(requestBody)
	if err != nil {
		return Result{}, err
	}
	url := c.URL
	if !strings.HasSuffix(url, "/v1/chat/completions") {
		url += "/v1/chat/completions"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(encoded))
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("vision request: %w", err)
	}
	defer resp.Body.Close()
	// Provider response bodies can include submitted image data or sensitive
	// prompts. Never include them in errors or logs.
	if resp.StatusCode != http.StatusOK {
		code, permanent := "vision_provider", false
		switch resp.StatusCode {
		case http.StatusUnauthorized, http.StatusForbidden:
			code, permanent = "vision_auth", true
		case http.StatusNotFound:
			code, permanent = "vision_model_unavailable", true
		case http.StatusBadRequest, http.StatusUnprocessableEntity:
			code, permanent = "vision_request_invalid", true
		case http.StatusTooManyRequests:
			code = "vision_rate_limit"
		}
		return Result{}, &RequestError{Code: code, HTTPStatus: resp.StatusCode, Permanent: permanent}
	}
	var decoded struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&decoded); err != nil {
		return Result{}, &RequestError{Code: "vision_response_invalid"}
	}
	if len(decoded.Choices) == 0 {
		return Result{}, &RequestError{Code: "vision_response_invalid"}
	}
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	result := Result{Model: Model, PromptVersion: PromptVersion,
		InputTokens: decoded.Usage.PromptTokens, OutputTokens: decoded.Usage.CompletionTokens}
	if content == "NO_CONTENT" {
		result.Status = "no_content"
		return result, nil
	}
	if content == "" || len([]rune(content)) > 2000 {
		return Result{}, &RequestError{Code: "vision_response_invalid"}
	}
	result.Status = "described"
	result.Text = content
	return result, nil
}
