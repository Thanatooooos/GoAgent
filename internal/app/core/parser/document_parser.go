package parser

import "io"

// DocumentParser defines the common parser contract for different document types.
type DocumentParser interface {
	ParserType() string
	Parse(content []byte, mimeType string, options map[string]any) (ParseResult, error)
	ExtractText(stream io.Reader, fileName string) (string, error)
	Supports(mimeType string) bool
}
