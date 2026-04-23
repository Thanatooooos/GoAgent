package convention

type ChatRequest struct {
	Messages    []ChatMessage `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	TopP        float64       `json:"top_p,omitempty"`
	TopK        int           `json:"top_k,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Thinking    bool          `json:"thinking,omitempty"`
	EnableTools bool          `json:"enable_tools,omitempty"`
}
