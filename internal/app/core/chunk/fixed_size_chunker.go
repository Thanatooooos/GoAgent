package chunk

import (
	"strings"
	"unicode"
)

// sentenceBoundaries is the punctuation set that marks sentence endings.
var sentenceBoundaries = map[rune]bool{
	'。': true,
	'！': true,
	'？': true,
	'.': true,
	'!': true,
	'?': true,
}

type FixedSizeChunker struct{}

func NewFixedSizeChunker() *FixedSizeChunker {
	return &FixedSizeChunker{}
}

func (c *FixedSizeChunker) Strategy() Strategy {
	return StrategyFixedSize
}

func (c *FixedSizeChunker) Chunk(text string, opts Options) ([]Chunk, error) {
	opts = opts.Normalize()
	if strings.TrimSpace(text) == "" {
		return []Chunk{}, nil
	}

	runes := []rune(normalizeChunkText(text))
	chunks := make([]Chunk, 0, estimateChunkCount(len(runes), opts.ChunkSize, opts.OverlapSize))

	start := 0
	index := 0
	for start < len(runes) {
		end := start + opts.ChunkSize
		if end > len(runes) {
			end = len(runes)
		} else {
			end = findSentenceBoundary(runes, end, opts.ChunkSize)
		}

		if end < len(runes) && opts.MinChunkSize > 0 && len(runes)-end < opts.MinChunkSize {
			end = len(runes)
		}

		part := string(runes[start:end])
		if strings.TrimSpace(part) != "" {
			chunkID, err := nextChunkID()
			if err != nil {
				return nil, err
			}
			chunk := NewChunk(chunkID, index, part)
			chunks = append(chunks, chunk)
			index++
		}

		if end >= len(runes) {
			break
		}
		nextStart := end - opts.OverlapSize
		if nextStart <= start {
			nextStart = end
		}
		start = nextStart
	}
	return chunks, nil
}

func findSentenceBoundary(runes []rune, end int, chunkSize int) int {
	if end <= 0 || end >= len(runes) {
		return end
	}

	window := max(1, chunkSize/2)
	searchStart := max(0, end-window)
	searchEnd := min(len(runes)-1, end+window)

	for _, candidate := range orderedBoundaryCandidates(runes, end, searchStart, searchEnd) {
		if candidate > searchStart && candidate <= len(runes) {
			return candidate
		}
	}
	return end
}

func orderedBoundaryCandidates(runes []rune, end int, searchStart int, searchEnd int) []int {
	candidates := make([]int, 0, 8)
	appendUnique := func(value int) {
		if value <= searchStart || value > len(runes) {
			return
		}
		for _, existing := range candidates {
			if existing == value {
				return
			}
		}
		candidates = append(candidates, value)
	}

	limit := max(end-searchStart, searchEnd-end)
	for offset := 0; offset <= limit; offset++ {
		if offset == 0 {
			if boundary := classifyBoundary(runes, end); boundary > 0 {
				appendUnique(boundary)
			}
			continue
		}
		left := end - offset
		if left >= searchStart && left <= searchEnd {
			if boundary := classifyBoundary(runes, left); boundary > 0 {
				appendUnique(boundary)
			}
		}
		right := end + offset
		if right >= searchStart && right <= searchEnd {
			if boundary := classifyBoundary(runes, right); boundary > 0 {
				appendUnique(boundary)
			}
		}
	}
	return candidates
}

func classifyBoundary(runes []rune, index int) int {
	if index < 0 || index >= len(runes) {
		return 0
	}
	if isSentenceBoundary(runes[index]) {
		return index + 1
	}
	if index > 0 && runes[index-1] == '\n' && runes[index] == '\n' {
		return index + 1
	}
	if index > 0 && isSoftChineseBoundaryStart(runes, index) {
		return index
	}
	if runes[index] == '\n' {
		return index + 1
	}
	return 0
}

func isSentenceBoundary(r rune) bool {
	return sentenceBoundaries[r]
}

func isSoftChineseBoundaryStart(runes []rune, index int) bool {
	lineStart := index
	for lineStart > 0 && runes[lineStart-1] != '\n' {
		lineStart--
	}
	if lineStart != index {
		return false
	}
	lineEnd := index
	for lineEnd < len(runes) && runes[lineEnd] != '\n' {
		lineEnd++
	}
	line := strings.TrimSpace(string(runes[index:lineEnd]))
	if line == "" {
		return false
	}
	line = strings.TrimLeftFunc(line, unicode.IsSpace)
	return isChineseOutlinePrefix(line) || isFAQPrefix(line)
}

func isChineseOutlinePrefix(line string) bool {
	if strings.HasPrefix(line, "第") && hasChineseSectionMarker(line) {
		return true
	}
	prefixes := []string{
		"（一）", "(一)", "一、",
		"（二）", "(二)", "二、",
		"（三）", "(三)", "三、",
		"（四）", "(四)", "四、",
		"1.", "2.", "3.",
		"1.1", "1.2", "2.1",
	}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func hasChineseSectionMarker(line string) bool {
	markers := []string{"章", "节", "条", "部分", "篇"}
	for _, marker := range markers {
		if strings.Contains(line, marker) {
			return true
		}
	}
	return false
}

func isFAQPrefix(line string) bool {
	prefixes := []string{"Q:", "Q：", "A:", "A：", "问题：", "答案："}
	for _, prefix := range prefixes {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func normalizeChunkText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func estimateChunkCount(length int, chunkSize int, overlapSize int) int {
	if length <= 0 || chunkSize <= 0 {
		return 0
	}
	step := chunkSize - overlapSize
	if step <= 0 {
		step = chunkSize
	}
	count := length / step
	if length%step != 0 {
		count++
	}
	if count == 0 {
		count = 1
	}
	return count
}
