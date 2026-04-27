package chunk

import (
	"strings"
)

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
			chunks = append(chunks, NewChunk(chunkID, index, part))
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
