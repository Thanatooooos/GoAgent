package parser

// ParseResult is the normalized parser output.
type ParseResult struct {
	Text     string
	Metadata map[string]any
}

func OfText(text string) ParseResult {
	return ParseResult{
		Text:     text,
		Metadata: map[string]any{},
	}
}

func Of(text string, metadata map[string]any) ParseResult {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return ParseResult{
		Text:     text,
		Metadata: metadata,
	}
}
