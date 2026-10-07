package parser

import (
	"context"
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

func (p *MarkdownDocumentParser) Parse(_ context.Context, content []byte, mimeType string, options map[string]any) (ParseResult, error) {
	text := normalizeTextContent(content)
	result := Of(text, map[string]any{
		"mime_type":   mimeType,
		"parser_type": p.ParserType(),
		"format":      "markdown",
	})
	for _, match := range imageMarkdownPattern.FindAllStringSubmatch(text, -1) {
		if len(match) > 1 && (strings.HasPrefix(match[1], "https://") || strings.HasPrefix(match[1], "http://")) {
			result.Images = append(result.Images, ImageOccurrence{Index: len(result.Images), OriginalRef: match[1], Error: "external image is not downloaded"})
		}
	}
	return result, nil
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
	return mimeType == "text/markdown" || mimeType == "text/x-markdown" || mimeType == "text/plain"
}
