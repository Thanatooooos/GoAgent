package parser

import (
	"io"
	"strings"
)

type MarkdownDocumentParser struct{}

func NewMarkdownDocumentParser() *MarkdownDocumentParser {
	return &MarkdownDocumentParser{}
}

func (p *MarkdownDocumentParser) ParserType() string {
	return ParserTypeMarkdown
}

func (p *MarkdownDocumentParser) Parse(content []byte, mimeType string, options map[string]any) (ParseResult, error) {
	text := normalizeTextContent(content)
	return Of(text, map[string]any{
		"mime_type":   mimeType,
		"parser_type": p.ParserType(),
		"format":      "markdown",
	}), nil
}

func (p *MarkdownDocumentParser) ExtractText(stream io.Reader, fileName string) (string, error) {
	content, err := io.ReadAll(stream)
	if err != nil {
		return "", err
	}
	return normalizeTextContent(content), nil
}

func (p *MarkdownDocumentParser) Supports(mimeType string) bool {
	mimeType = strings.ToLower(strings.TrimSpace(mimeType))
	return mimeType == "text/markdown" || mimeType == "text/x-markdown"
}
