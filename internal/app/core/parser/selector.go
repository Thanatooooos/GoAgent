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
	var docReaderParser DocumentParser
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
		if address := strings.TrimSpace(cfg.Parser.DocReader.Address); address != "" {
			timeout := time.Duration(cfg.Parser.DocReader.TimeoutMs) * time.Millisecond
			var ocr OCRClient
			if url := strings.TrimSpace(cfg.Parser.OCR.URL); url != "" {
				ocr = NewHTTPOCRClient(url, time.Duration(cfg.Parser.OCR.TimeoutMs)*time.Millisecond)
			}
			parser, err := NewDocReaderDocumentParser(address, timeout, tikaParser, ocr)
			if err == nil {
				docReaderParser = parser
			}
		}
	}
	return NewSelector(
		NewMarkdownDocumentParser(),
		docReaderParser,
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
	case ".txt", ".text":
		if parser, ok := s.Select(ParserTypeMarkdown); ok {
			return parser
		}
	case ".pdf", ".doc", ".docx", ".xls", ".xlsx", ".epub", ".png", ".jpg", ".jpeg", ".gif", ".bmp", ".tif", ".tiff", ".webp":
		if parser, ok := s.Select(ParserTypeDocReader); ok {
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
	return nil
}
