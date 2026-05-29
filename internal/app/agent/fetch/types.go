package fetch

type PageResult struct {
	URL            string `json:"url"`
	Text           string `json:"text,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	OriginalLength int    `json:"original_length,omitempty"`
	WasTruncated   bool   `json:"was_truncated,omitempty"`
}

type Output struct {
	URLs          []string     `json:"urls"`
	Pages         []PageResult `json:"pages"`
	CombinedText  string       `json:"combined_text,omitempty"`
	SuccessCount  int          `json:"success_count"`
	FailCount     int          `json:"fail_count"`
	Summary       string       `json:"summary"`
	Degraded      bool         `json:"degraded,omitempty"`
	DegradeReason string       `json:"degrade_reason,omitempty"`
	ErrorMessage  string       `json:"error_message,omitempty"`
}
