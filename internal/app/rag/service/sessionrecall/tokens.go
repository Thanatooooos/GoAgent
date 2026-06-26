package sessionrecall

import (
	"strings"
	"unicode"
	"unicode/utf8"

	ragtoken "local/rag-project/internal/app/rag/core/tokenbudget"
)

type TokenEstimator = ragtoken.Estimator

type RoughTokenEstimator struct{}

func (RoughTokenEstimator) EstimateTokens(text string) int {
	return ragtoken.NewDefaultEstimator().EstimateTokens(text)
}

func splitTextByTokenBudget(text string, tokenBudget int, overlapTokens int, estimator TokenEstimator) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}
	if tokenBudget <= 0 {
		tokenBudget = 3000
	}
	if estimator == nil {
		estimator = RoughTokenEstimator{}
	}

	type tokenizedLine struct {
		text   string
		tokens int
	}

	lines := strings.Split(text, "\n")
	chunks := make([]string, 0, len(lines)/8+1)
	current := make([]tokenizedLine, 0, 16)
	currentTokens := 0

	appendChunk := func(items []tokenizedLine) {
		if len(items) == 0 {
			return
		}
		lines := make([]string, 0, len(items))
		for _, item := range items {
			lines = append(lines, item.text)
		}
		chunks = append(chunks, strings.TrimSpace(strings.Join(lines, "\n")))
	}

	tailOverlap := func(items []tokenizedLine, budget int) ([]tokenizedLine, int) {
		if budget <= 0 || len(items) == 0 {
			return nil, 0
		}
		total := 0
		start := len(items)
		for i := len(items) - 1; i >= 0; i-- {
			if total > 0 && total+items[i].tokens > budget {
				break
			}
			total += items[i].tokens
			start = i
		}
		if start >= len(items) {
			return nil, 0
		}
		result := make([]tokenizedLine, len(items[start:]))
		copy(result, items[start:])
		return result, total
	}

	trimFrontToFit := func(items []tokenizedLine, total int, nextTokens int, budget int) ([]tokenizedLine, int) {
		for len(items) > 0 && total+nextTokens > budget {
			total -= items[0].tokens
			items = items[1:]
		}
		return items, total
	}

	flush := func(preserveOverlap bool) {
		if len(current) == 0 {
			return
		}
		snapshot := make([]tokenizedLine, len(current))
		copy(snapshot, current)
		appendChunk(snapshot)
		if preserveOverlap {
			overlap, overlapTotal := tailOverlap(snapshot, overlapTokens)
			current = overlap
			currentTokens = overlapTotal
			return
		}
		current = current[:0]
		currentTokens = 0
	}

	for _, line := range lines {
		line = strings.TrimRight(line, "\r")
		lineTokens := estimator.EstimateTokens(line)
		if currentTokens > 0 && currentTokens+lineTokens > tokenBudget {
			flush(true)
			current, currentTokens = trimFrontToFit(current, currentTokens, lineTokens, tokenBudget)
		}
		if lineTokens > tokenBudget {
			segments := splitOversizedLine(line, tokenBudget, estimator)
			for _, segment := range segments {
				if strings.TrimSpace(segment) == "" {
					continue
				}
				if currentTokens > 0 {
					flush(false)
				}
				chunks = append(chunks, strings.TrimSpace(segment))
			}
			continue
		}
		current = append(current, tokenizedLine{text: line, tokens: lineTokens})
		currentTokens += lineTokens
	}
	flush(false)

	if len(chunks) == 0 {
		return []string{text}
	}
	return chunks
}

func splitOversizedLine(line string, tokenBudget int, estimator TokenEstimator) []string {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return nil
	}
	segments := make([]string, 0, len(runes)/1024+1)
	start := 0
	for start < len(runes) {
		end := minInt(len(runes), start+maxInt(256, tokenBudget*2))
		for end > start {
			segment := string(runes[start:end])
			if estimator.EstimateTokens(segment) <= tokenBudget {
				segments = append(segments, segment)
				start = end
				break
			}
			end--
		}
		if end == start {
			end = minInt(len(runes), start+256)
			segments = append(segments, string(runes[start:end]))
			start = end
		}
	}
	return segments
}

func truncateRunes(text string, maxChars int) string {
	text = strings.TrimSpace(text)
	if text == "" || maxChars <= 0 {
		return text
	}
	if utf8.RuneCountInString(text) <= maxChars {
		return text
	}
	runes := []rune(text)
	if maxChars <= 1 {
		return string(runes[:maxChars])
	}
	return strings.TrimSpace(string(runes[:maxChars-1])) + "…"
}

func isCJKRune(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
