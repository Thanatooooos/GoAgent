package recall

import (
	"strings"
	"unicode"
)

const maxRecallSearchTokens = 8

func extractRecallTokens(value string) []string {
	value = normalizeRecallText(value)
	if value == "" {
		return nil
	}
	tokens := strings.FieldsFunc(value, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r) && !isCJKRune(r)
	})
	result := make([]string, 0, len(tokens))
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token != "" {
			result = append(result, token)
		}
	}
	return result
}

func buildRecallSearchTokens(query string) []string {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	baseTokens := extractRecallTokens(query)
	result := make([]string, 0, len(baseTokens)+8)
	seen := map[string]struct{}{}
	for _, token := range baseTokens {
		if !shouldUseRecallSearchToken(query, token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	for _, token := range buildDistinctCJKSearchBigrams(query) {
		if !shouldUseRecallSearchToken(query, token) {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	if len(result) > maxRecallSearchTokens {
		return result[:maxRecallSearchTokens]
	}
	return result
}

func shouldUseRecallSearchToken(query string, token string) bool {
	token = strings.TrimSpace(token)
	if token == "" || isRecallNoiseToken(token) {
		return false
	}
	if containsCJKString(token) {
		return len([]rune(token)) >= 2
	}
	if containsCJKString(query) {
		return len(token) >= 2
	}
	return len(token) >= 3
}

func isRecallNoiseToken(token string) bool {
	switch compactLowerString(token) {
	case "a", "an", "and", "are", "for", "how", "pls", "please", "should", "the", "this", "what", "with", "你", "吗", "呢", "请问", "这个", "可以", "怎么", "了吗":
		return true
	default:
		return false
	}
}

func compactLowerString(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.Join(strings.Fields(value), " ")))
}

func containsCJKString(value string) bool {
	for _, r := range value {
		if isCJKRune(r) {
			return true
		}
	}
	return false
}

func buildDistinctCJKBigrams(value string) []string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) < 2 {
		return nil
	}
	result := make([]string, 0, len(runes)-1)
	seen := map[string]struct{}{}
	for idx := 0; idx < len(runes)-1; idx++ {
		if !isCJKRune(runes[idx]) || !isCJKRune(runes[idx+1]) {
			continue
		}
		token := string(runes[idx : idx+2])
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
	}
	return result
}

func buildDistinctCJKSearchBigrams(value string) []string {
	return buildDistinctCJKBigrams(compactLowerString(value))
}

func isCJKRune(r rune) bool {
	return unicode.In(r,
		unicode.Han,
		unicode.Hiragana,
		unicode.Katakana,
		unicode.Hangul,
	)
}
