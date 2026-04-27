package parser

import (
	"net/http"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"local/rag-project/internal/framework/config"
)

// Selector chooses the most suitable parser from the registered implementations.
type Selector struct {
	parsers   []DocumentParser
	parserMap map[string]DocumentParser
}

func NewSelector(parsers ...DocumentParser) *Selector {
	normalized := make([]DocumentParser, 0, len(parsers))
	parserMap := make(map[string]DocumentParser, len(parsers))
	for _, each := range parsers {
		if each == nil {
			continue
		}
		parserType := strings.TrimSpace(each.ParserType())
		if parserType == "" {
			continue
		}
		if _, exists := parserMap[parserType]; exists {
			continue
		}
		normalized = append(normalized, each)
		parserMap[parserType] = each
	}
	return &Selector{
		parsers:   normalized,
		parserMap: parserMap,
	}
}

func NewDefaultSelector(httpClient *http.Client) *Selector {
	var tikaParser DocumentParser
	if cfg := config.Get(); cfg != nil {
		tikaURL := strings.TrimSpace(cfg.Parser.Tika.URL)
		if tikaURL != "" {
			client := httpClient
			timeoutMs := cfg.Parser.Tika.TimeoutMs
			if client == nil {
				timeout := defaultTikaTimeout
				if timeoutMs > 0 {
					timeout = time.Duration(timeoutMs) * time.Millisecond
				}
				client = &http.Client{Timeout: timeout}
			}
			tikaParser = NewTikaDocumentParser(client, tikaURL)
		}
	}
	return NewSelector(
		NewMarkdownDocumentParser(),
		tikaParser,
	)
}

func (s *Selector) Select(parserType string) (DocumentParser, bool) {
	if s == nil {
		return nil, false
	}
	parser, ok := s.parserMap[strings.TrimSpace(parserType)]
	return parser, ok
}

func (s *Selector) SelectByMimeType(mimeType string) DocumentParser {
	if s == nil {
		return nil
	}
	for _, each := range s.parsers {
		if each.Supports(mimeType) {
			return each
		}
	}
	return s.fallback()
}

func (s *Selector) SelectByFileName(fileName string) DocumentParser {
	if s == nil {
		return nil
	}
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(fileName)))
	switch ext {
	case ".md", ".markdown":
		if parser, ok := s.Select(ParserTypeMarkdown); ok {
			return parser
		}
	}
	return nil
}

func (s *Selector) SelectFor(mimeType string, fileName string) DocumentParser {
	if parser := s.SelectByFileName(fileName); parser != nil {
		return parser
	}
	return s.SelectByMimeType(mimeType)
}

func (s *Selector) AvailableTypes() []string {
	if s == nil {
		return nil
	}
	types := make([]string, 0, len(s.parsers))
	for _, each := range s.parsers {
		types = append(types, each.ParserType())
	}
	slices.Sort(types)
	return types
}

func (s *Selector) fallback() DocumentParser {
	if parser, ok := s.Select(ParserTypeTika); ok {
		return parser
	}
	if len(s.parsers) == 0 {
		return nil
	}
	return s.parsers[0]
}
