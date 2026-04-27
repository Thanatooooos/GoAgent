package parser

import (
	"bytes"
	"strings"
)

func normalizeTextContent(content []byte) string {
	trimmed := bytes.TrimPrefix(content, []byte{0xEF, 0xBB, 0xBF})
	text := string(trimmed)
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}
