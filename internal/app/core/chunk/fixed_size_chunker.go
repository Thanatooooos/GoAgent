package chunk

import "strings"

// sentenceBoundaries 句末字符集合。
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
			// 在 chunkSize ± 20% 范围内寻找更优的语义切分点。
			end = findSentenceBoundary(runes, end, opts.ChunkSize)
		}

		// 剩余尾部不足 MinChunkSize 时合并到上一个 chunk 中。
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

// findSentenceBoundary 在 end 附近的语义边界窗口中寻找最佳切分点。
// 优先找句末标点，其次找段落分隔符（双换行），最后找单换行。
func findSentenceBoundary(runes []rune, end int, chunkSize int) int {
	if end <= 0 || end >= len(runes) {
		return end
	}

	// 搜索窗口：chunkSize 的 25% 向前和向后。
	window := max(1, chunkSize/4)

	// 优先级 1：向前搜索句末标点（。！？. ! ?）。
	searchStart := max(0, end-window)
	searchEnd := min(len(runes)-1, end+window)

	best := end

	// 从句末标点附近向 end 方向收敛。
	for i := end + window; i >= searchStart && i > end-5; i-- {
		if i < len(runes) && i > 0 && isSentenceBoundary(runes[i]) {
			// 在标点后一个字符处切分。
			return min(i+1, len(runes))
		}
	}

	// 优先级 2：向前搜索段落分隔（双换行）。
	for i := end; i >= searchStart; i-- {
		if i > 0 && runes[i-1] == '\n' && runes[i] == '\n' {
			return i + 1
		}
	}

	// 优先级 3：向前搜索单换行。
	for i := end; i >= searchStart; i-- {
		if runes[i] == '\n' {
			return i + 1
		}
	}

	// 未找到语义边界时也尝试向后搜索到下一个换行（短距离）。
	if end < searchEnd {
		for i := end + 1; i <= searchEnd && i < len(runes); i++ {
			if runes[i] == '\n' {
				return i + 1
			}
		}
	}

	return best
}

func isSentenceBoundary(r rune) bool {
	return sentenceBoundaries[r]
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

