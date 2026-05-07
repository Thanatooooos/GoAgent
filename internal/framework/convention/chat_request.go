package convention

import "fmt"

type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Temperature *float64      `json:"temperature,omitempty"`
	TopP        *float64      `json:"top_p,omitempty"`
	TopK        *int          `json:"top_k,omitempty"`
	MaxTokens   *int          `json:"max_tokens,omitempty"`
	Thinking    *bool         `json:"thinking,omitempty"`
	EnableTools *bool         `json:"enable_tools,omitempty"`
	JSONMode    *bool         `json:"json_mode,omitempty"`
}

func (r ChatRequest) Validate() error {
	if len(r.Messages) == 0 {
		return fmt.Errorf("messages is required")
	}
	if r.Temperature != nil && (*r.Temperature < 0 || *r.Temperature > 2) {
		return fmt.Errorf("temperature must be between 0 and 2")
	}
	if r.TopP != nil && (*r.TopP <= 0 || *r.TopP > 1) {
		return fmt.Errorf("top_p must be between 0 and 1")
	}
	if r.TopK != nil && *r.TopK < 0 {
		return fmt.Errorf("top_k must be greater than or equal to 0")
	}
	if r.MaxTokens != nil && *r.MaxTokens <= 0 {
		return fmt.Errorf("max_tokens must be greater than 0")
	}
	return nil
}

func (r ChatRequest) ThinkingEnabled() bool {
	return r.Thinking != nil && *r.Thinking
}

func (r ChatRequest) ToolsEnabled() bool {
	return r.EnableTools != nil && *r.EnableTools
}

func (r ChatRequest) HasTemperature() bool {
	return r.Temperature != nil
}

func (r ChatRequest) HasTopP() bool {
	return r.TopP != nil
}

func (r ChatRequest) HasTopK() bool {
	return r.TopK != nil
}

func (r ChatRequest) HasMaxTokens() bool {
	return r.MaxTokens != nil
}

// JSONModeEnabled 表示是否请求 LLM 以 JSON 格式输出。
func (r ChatRequest) JSONModeEnabled() bool {
	return r.JSONMode != nil && *r.JSONMode
}
