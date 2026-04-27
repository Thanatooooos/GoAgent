package chunk

import "strings"

type MarkdownChunker struct{}

func NewMarkdownChunker() *MarkdownChunker {
	return &MarkdownChunker{}
}

func (c *MarkdownChunker) Strategy() Strategy {
	return StrategyMarkdown
}

func (c *MarkdownChunker) Chunk(text string, opts Options) ([]Chunk, error) {
	opts = opts.Normalize()
	text = normalizeChunkText(text)
	if strings.TrimSpace(text) == "" {
		return []Chunk{}, nil
	}

	blocks := splitMarkdownBlocks(text)
	if len(blocks) == 0 {
		return []Chunk{}, nil
	}

	chunks := make([]Chunk, 0, len(blocks))
	var builder strings.Builder
	index := 0

	flush := func() error {
		content := strings.TrimSpace(builder.String())
		builder.Reset()
		if content == "" {
			return nil
		}
		chunkID, err := nextChunkID()
		if err != nil {
			return err
		}
		chunks = append(chunks, NewChunk(chunkID, index, content))
		index++
		return nil
	}

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		if builder.Len() == 0 {
			builder.WriteString(block)
			continue
		}

		if isHeadingBlock(block) {
			if err := flush(); err != nil {
				return nil, err
			}
			builder.WriteString(block)
			continue
		}

		candidate := builder.String() + "\n\n" + block
		if utf8Len(candidate) <= opts.ChunkSize {
			builder.WriteString("\n\n")
			builder.WriteString(block)
			continue
		}

		if err := flush(); err != nil {
			return nil, err
		}

		if utf8Len(block) <= opts.ChunkSize {
			builder.WriteString(block)
			continue
		}

		subChunks, err := NewFixedSizeChunker().Chunk(block, Options{
			Strategy:     StrategyFixedSize,
			ChunkSize:    opts.ChunkSize,
			OverlapSize:  opts.OverlapSize,
			MinChunkSize: opts.MinChunkSize,
		})
		if err != nil {
			return nil, err
		}
		for _, sub := range subChunks {
			sub.Index = index
			chunks = append(chunks, sub)
			index++
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}
	return chunks, nil
}

func splitMarkdownBlocks(text string) []string {
	lines := strings.Split(text, "\n")
	blocks := make([]string, 0)
	var current strings.Builder
	inCodeFence := false

	flush := func() {
		content := strings.TrimSpace(current.String())
		current.Reset()
		if content != "" {
			blocks = append(blocks, content)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if current.Len() > 0 {
				current.WriteString("\n")
			}
			current.WriteString(line)
			if inCodeFence {
				flush()
				inCodeFence = false
			} else {
				inCodeFence = true
			}
			continue
		}

		if inCodeFence {
			if current.Len() > 0 {
				current.WriteString("\n")
			}
			current.WriteString(line)
			continue
		}

		if isMarkdownHeading(trimmed) {
			flush()
			current.WriteString(line)
			continue
		}

		if trimmed == "" {
			flush()
			continue
		}

		if current.Len() > 0 {
			current.WriteString("\n")
		}
		current.WriteString(line)
	}

	flush()
	return blocks
}

func isMarkdownHeading(line string) bool {
	if line == "" || !strings.HasPrefix(line, "#") {
		return false
	}
	hashCount := 0
	for _, r := range line {
		if r == '#' {
			hashCount++
			continue
		}
		break
	}
	return hashCount > 0 && hashCount <= 6 && len(line) > hashCount && line[hashCount] == ' '
}

func isHeadingBlock(block string) bool {
	firstLine := block
	if idx := strings.Index(block, "\n"); idx >= 0 {
		firstLine = block[:idx]
	}
	return isMarkdownHeading(strings.TrimSpace(firstLine))
}

func utf8Len(text string) int {
	return len([]rune(text))
}
