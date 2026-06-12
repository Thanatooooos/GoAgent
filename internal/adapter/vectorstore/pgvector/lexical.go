package pgvector

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"local/rag-project/internal/framework/config"
)

const (
	defaultMetadataSectionWeight        = 3.0
	defaultMetadataDocumentNameWeight   = 1.4
	defaultMetadataSourceFileNameWeight = 1.8
	maxLexicalQueryTokens               = 32
)

var lexicalStopPhrases = []string{
	"查找",
	"搜索",
	"检索",
	"这一节",
	"这节",
	"这一章",
	"这一部分",
	"章节",
	"文档",
	"文件",
	"相关内容",
	"主要讲什么",
	"讲了什么",
	"是什么",
}

type LexicalPayload struct {
	ContentLexemes        string
	DocumentNameLexemes   string
	SourceFileNameLexemes string
	SectionLexemes        string
}

type lexicalQuery struct {
	Raw            string
	NormalizedText string
	Tokens         []string
	TSQuery        string
}

func BuildLexicalPayload(content string, metadata map[string]any) LexicalPayload {
	return LexicalPayload{
		ContentLexemes:        lexicalLexemes(content),
		DocumentNameLexemes:   lexicalLexemes(metadataStringFromMap(metadata, "document_name")),
		SourceFileNameLexemes: lexicalLexemes(metadataStringFromMap(metadata, "source_file_name")),
		SectionLexemes:        lexicalLexemes(metadataStringFromMap(metadata, "section")),
	}
}

func buildLexicalQuery(raw string) lexicalQuery {
	normalized := normalizeLexicalQueryText(raw)
	tokens := lexicalTokens(normalized)
	if len(tokens) > maxLexicalQueryTokens {
		tokens = tokens[:maxLexicalQueryTokens]
	}

	var builder strings.Builder
	for _, token := range tokens {
		if builder.Len() > 0 {
			builder.WriteString(" | ")
		}
		builder.WriteString(token)
	}

	return lexicalQuery{
		Raw:            strings.TrimSpace(raw),
		NormalizedText: normalized,
		Tokens:         tokens,
		TSQuery:        builder.String(),
	}
}

func lexicalLexemes(text string) string {
	return strings.Join(lexicalTokens(normalizeLexicalDocumentText(text)), " ")
}

func lexicalTokens(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	result := make([]string, 0, utf8.RuneCountInString(text))
	seen := make(map[string]struct{})
	appendToken := func(token string) {
		token = strings.TrimSpace(strings.ToLower(token))
		if token == "" {
			return
		}
		if _, ok := seen[token]; ok {
			return
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}

	runes := []rune(text)
	for i := 0; i < len(runes); {
		switch {
		case isHanRune(runes[i]):
			j := i + 1
			for j < len(runes) && isHanRune(runes[j]) {
				j++
			}
			appendHanTokens(string(runes[i:j]), appendToken)
			i = j
		case isASCIIWordRune(runes[i]):
			j := i + 1
			for j < len(runes) && isASCIIWordRune(runes[j]) {
				j++
			}
			token := strings.Trim(strings.ToLower(string(runes[i:j])), "_-.")
			if token != "" {
				appendToken(token)
				for _, part := range splitASCIIToken(token) {
					appendToken(part)
				}
			}
			i = j
		default:
			i++
		}
	}

	return result
}

func appendHanTokens(text string, appendToken func(string)) {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return
	}
	if len(runes) == 1 {
		appendToken(string(runes))
		return
	}
	if len(runes) <= 8 {
		appendToken(string(runes))
	}
	for i := 0; i < len(runes)-1; i++ {
		appendToken(string(runes[i : i+2]))
	}
}

func splitASCIIToken(token string) []string {
	fields := strings.FieldsFunc(token, func(r rune) bool {
		return r == '_' || r == '-' || r == '.'
	})
	if len(fields) <= 1 {
		return nil
	}
	return fields
}

func normalizeLexicalDocumentText(text string) string {
	return normalizeLexicalText(text, false)
}

func normalizeLexicalQueryText(text string) string {
	normalized := normalizeLexicalText(text, true)
	for _, phrase := range lexicalStopPhrases {
		normalized = strings.ReplaceAll(normalized, phrase, " ")
	}
	return compactLexicalWhitespace(normalized)
}

func normalizeLexicalText(text string, keepPathSeparators bool) string {
	text = strings.ToLower(strings.TrimSpace(text))
	if text == "" {
		return ""
	}

	var builder strings.Builder
	spacePending := false
	for _, r := range text {
		switch {
		case isHanRune(r), unicode.IsLetter(r), unicode.IsDigit(r):
			if spacePending && builder.Len() > 0 {
				builder.WriteRune(' ')
			}
			spacePending = false
			builder.WriteRune(r)
		case keepPathSeparators && (r == '>' || r == '/' || r == '\\'):
			if builder.Len() > 0 {
				builder.WriteRune(' ')
			}
			builder.WriteRune(r)
			builder.WriteRune(' ')
			spacePending = false
		default:
			spacePending = true
		}
	}
	return compactLexicalWhitespace(builder.String())
}

func compactLexicalWhitespace(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func buildShortIdentifierFallbackQuery(raw string) bool {
	query := normalizeLexicalQueryText(raw)
	if query == "" {
		return false
	}
	if strings.Contains(query, " ") {
		return false
	}
	runes := []rune(query)
	if len(runes) == 0 || len(runes) > 6 {
		return false
	}
	for _, r := range runes {
		if isHanRune(r) {
			return false
		}
	}
	return true
}

func isHanRune(r rune) bool {
	return unicode.Is(unicode.Han, r)
}

func isASCIIWordRune(r rune) bool {
	return r == '_' || r == '-' || r == '.' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

func metadataStringFromMap(metadata map[string]any, key string) string {
	if len(metadata) == 0 {
		return ""
	}
	value, ok := metadata[key]
	if !ok || value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return strings.TrimSpace(text)
	}
	return strings.TrimSpace(fmt.Sprintf("%v", value))
}

func metadataTitleWeights() (section float64, documentName float64, sourceFileName float64) {
	cfg := lexicalSearchConfig()
	section = defaultMetadataSectionWeight
	documentName = defaultMetadataDocumentNameWeight
	sourceFileName = defaultMetadataSourceFileNameWeight
	if cfg == nil {
		return
	}
	if cfg.SectionWeight > 0 {
		section = cfg.SectionWeight
	}
	if cfg.DocumentNameWeight > 0 {
		documentName = cfg.DocumentNameWeight
	}
	if cfg.SourceFileNameWeight > 0 {
		sourceFileName = cfg.SourceFileNameWeight
	}
	return
}

func lexicalSearchConfig() *config.RagMetadataTitleSearchChannelConfig {
	cfg := config.Get()
	if cfg == nil {
		return nil
	}
	return &cfg.Rag.Search.Channels.MetadataTitle
}

func lexicalMetadataFallbackEnabled() bool {
	cfg := config.Get()
	if cfg == nil || cfg.Rag.Search.Channels.MetadataTitle.EnabledFallbackTrgm == nil {
		return true
	}
	return *cfg.Rag.Search.Channels.MetadataTitle.EnabledFallbackTrgm
}

func lexicalQueryTokenDebug(query string) []string {
	return slices.Clone(lexicalTokens(normalizeLexicalQueryText(query)))
}
