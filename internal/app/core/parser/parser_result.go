package parser

// ParseResult is the normalized parser output.
type ParseResult struct {
	Text     string
	Metadata map[string]any
	// Images contains each image occurrence in document order. Image data is kept
	// separate from text so OCR and visual description can run independently.
	Images []ImageOccurrence
	// ImageInventoryConfirmed is false when a text-only fallback could not
	// establish whether the source document contains images.
	ImageInventoryConfirmed bool
}

type ImageOccurrence struct {
	Index        int
	OriginalRef  string
	Filename     string
	MIMEType     string
	StorageKey   string
	Data         []byte
	Page         int
	AdjacentText string
	Error        string
}

func OfText(text string) ParseResult {
	return ParseResult{
		Text:                    text,
		Metadata:                map[string]any{},
		ImageInventoryConfirmed: true,
	}
}

func Of(text string, metadata map[string]any) ParseResult {
	if metadata == nil {
		metadata = map[string]any{}
	}
	return ParseResult{
		Text:                    text,
		Metadata:                metadata,
		ImageInventoryConfirmed: true,
	}
}
