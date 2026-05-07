package chunk

import "strings"

// markdownHeading 描述一个解析到的标题层级。
type markdownHeading struct {
	Level   int    // 1-6，对应 h1-h6。
	Title   string // 去除 # 前缀后的标题文本。
	LineIdx int    // 在原 blocks 中的索引，用于层级退出判断。
}

// headingStack 维护当前 chunk 所处的标题路径。
type headingStack struct {
	stack []markdownHeading
}

func (s *headingStack) push(h markdownHeading) {
	// 弹出所有 >= 当前级别的标题（例如 h1 出现时，清空所有子标题）。
	idx := len(s.stack) - 1
	for idx >= 0 && s.stack[idx].Level >= h.Level {
		idx--
	}
	s.stack = append(s.stack[:idx+1], h)
}

func (s *headingStack) path() []string {
	result := make([]string, 0, len(s.stack))
	for _, h := range s.stack {
		result = append(result, h.Title)
	}
	return result
}

func (s *headingStack) section() string {
	titles := s.path()
	if len(titles) == 0 {
		return ""
	}
	return strings.Join(titles, " > ")
}

func (s *headingStack) currentLevel() int {
	if len(s.stack) == 0 {
		return 0
	}
	return s.stack[len(s.stack)-1].Level
}

func (s *headingStack) currentTitle() string {
	if len(s.stack) == 0 {
		return ""
	}
	return s.stack[len(s.stack)-1].Title
}

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
	headings := &headingStack{}

	// 当前正构建中的 chunk 的生效 heading 快照。
	currentSection := ""
	currentHeadingPath := []string{}
	currentHeadingLevel := 0
	snapshotHeading := func() {
		currentSection = headings.section()
		currentHeadingPath = headings.path()
		currentHeadingLevel = headings.currentLevel()
	}

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
		chunk := NewChunk(chunkID, index, content)
		// 将当前生效的语义元数据写入 chunk Metadata。
		if currentSection != "" {
			chunk.Metadata["section"] = currentSection
			chunk.Metadata["heading_path"] = currentHeadingPath
			chunk.Metadata["heading_level"] = currentHeadingLevel
		}
		chunks = append(chunks, chunk)
		index++
		return nil
	}

	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}

		// 处理标题块：更新 heading 栈然后开新 chunk。
		if isHeadingBlock(block) {
			level, title := parseHeading(block)
			if level > 0 {
				headings.push(markdownHeading{Level: level, Title: title})
			}
			if err := flush(); err != nil {
				return nil, err
			}
			snapshotHeading()
			builder.WriteString(block)
			continue
		}

		// 检测代码块语言标签。
		codeLang := detectCodeBlockLanguage(block)

		// 新 chunk 的第一个普通块。
		if builder.Len() == 0 {
			snapshotHeading()
			builder.WriteString(block)
			if codeLang != "" {
				// 标记当前正构建的 chunk 包含代码。
				// 无法在 flush 前预知，用延迟写入方式处理。
				_ = codeLang
			}
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
			snapshotHeading()
			builder.WriteString(block)
			continue
		}

		// 单 block 超长时降级为固定大小切分。
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
			if currentSection != "" {
				sub.Metadata["section"] = currentSection
				sub.Metadata["heading_path"] = currentHeadingPath
				sub.Metadata["heading_level"] = currentHeadingLevel
			}
			if codeLang != "" {
				sub.Metadata["code_language"] = codeLang
			}
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
	fenceMarker := ""

	flush := func() {
		content := strings.TrimSpace(current.String())
		current.Reset()
		if content != "" {
			blocks = append(blocks, content)
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 进入或退出代码围栏。
		if isCodeFenceLine(trimmed) {
			if inCodeFence && fenceMarker != "" && strings.HasPrefix(trimmed, fenceMarker) {
				// 匹配的闭合围栏。
				current.WriteString("\n")
				current.WriteString(line)
				flush()
				inCodeFence = false
				fenceMarker = ""
				continue
			}
			if !inCodeFence {
				flush()
				fenceMarker = strings.TrimRight(trimmed, "` \t")[:3]
				inCodeFence = true
				current.WriteString(line)
				continue
			}
		}

		if inCodeFence {
			current.WriteString("\n")
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

// parseHeading 从标题块的第一行提取级别和标题文本。
func parseHeading(block string) (int, string) {
	// 只取第一行，避免把后续段落内容混入标题。
	line := block
	if idx := strings.Index(block, "\n"); idx >= 0 {
		line = block[:idx]
	}
	line = strings.TrimSpace(line)
	level := 0
	for _, r := range line {
		if r == '#' {
			level++
			continue
		}
		break
	}
	if level == 0 || level > 6 {
		return 0, ""
	}
	title := strings.TrimSpace(line[level:])
	return level, title
}

func isCodeFenceLine(line string) bool {
	return strings.HasPrefix(line, "```")
}

// detectCodeBlockLanguage 检测代码块的编程语言标签。
func detectCodeBlockLanguage(block string) string {
	if !strings.HasPrefix(block, "```") {
		return ""
	}
	firstLine := block
	if idx := strings.Index(block, "\n"); idx >= 0 {
		firstLine = block[:idx]
	}
	tag := strings.TrimPrefix(firstLine, "```")
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return ""
	}
	return tag
}

func utf8Len(text string) int {
	return len([]rune(text))
}
