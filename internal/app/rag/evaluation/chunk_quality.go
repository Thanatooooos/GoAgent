package evaluation

import (
	"math"
	"strings"
)

// ChunkSample is a minimal representation of a chunk for quality measurement.
type ChunkSample struct {
	ID             string
	Index          int
	Text           string
	Strategy       string
	Metadata       map[string]any
	DocumentID     string
	SourceFileName string
}

// ChunkStrategyQuality holds per-strategy quality metrics.
type ChunkStrategyQuality struct {
	Strategy             string  `json:"strategy"`
	ChunkCount           int     `json:"chunkCount"`
	AverageSize          float64 `json:"averageSize"`
	SizeStdDev           float64 `json:"sizeStdDev"`
	MinSize              int     `json:"minSize"`
	MaxSize              int     `json:"maxSize"`
	OversizedCount       int     `json:"oversizedCount"`       // > expectedSize
	UndersizedCount      int     `json:"undersizedCount"`      // < minChunkSize
	BoundaryQuality      float64 `json:"boundaryQuality"`      // 0-1, higher = more chunks start at natural boundaries
	SectionCoverage      float64 `json:"sectionCoverage"`      // % with non-empty section metadata
	HeadingPathCoverage  float64 `json:"headingPathCoverage"`  // % with non-empty heading_path
	CodeLangCoverage     float64 `json:"codeLangCoverage"`     // % of code chunks with code_language detected
}

// ChunkQualityReport aggregates chunk quality across strategies.
type ChunkQualityReport struct {
	TotalChunks   int                    `json:"totalChunks"`
	TotalDocs     int                    `json:"totalDocuments"`
	ByStrategy    []ChunkStrategyQuality `json:"byStrategy"`
	Overall       ChunkStrategyQuality   `json:"overall"`
}

// ChunkQualityOptions controls thresholds for quality measurement.
type ChunkQualityOptions struct {
	ExpectedChunkSize int // default 800
	MinChunkSize      int // default 100
	MaxChunkSize      int // alias for ExpectedChunkSize, used for oversized detection
}

// EvaluateChunkQuality computes chunk quality metrics from a set of chunk samples.
func EvaluateChunkQuality(chunks []ChunkSample, opts ChunkQualityOptions) ChunkQualityReport {
	if opts.ExpectedChunkSize <= 0 {
		opts.ExpectedChunkSize = 800
	}
	if opts.MinChunkSize <= 0 {
		opts.MinChunkSize = 100
	}

	docs := map[string]struct{}{}
	for _, c := range chunks {
		if c.DocumentID != "" {
			docs[c.DocumentID] = struct{}{}
		}
	}

	grouped := groupChunksByStrategy(chunks)
	summaries := make([]ChunkStrategyQuality, 0, len(grouped))
	var allSizes []int

	for strategy, group := range grouped {
		summary := computeStrategyQuality(strategy, group, opts)
		summaries = append(summaries, summary)
		for _, c := range group {
			allSizes = append(allSizes, len([]rune(c.Text)))
		}
	}

	return ChunkQualityReport{
		TotalChunks: len(chunks),
		TotalDocs:   len(docs),
		ByStrategy:  summaries,
		Overall:     computeOverallQuality(summaries, allSizes, opts),
	}
}

func groupChunksByStrategy(chunks []ChunkSample) map[string][]ChunkSample {
	grouped := map[string][]ChunkSample{}
	for _, c := range chunks {
		strategy := strings.TrimSpace(c.Strategy)
		if strategy == "" {
			strategy = "unknown"
		}
		grouped[strategy] = append(grouped[strategy], c)
	}
	return grouped
}

func computeStrategyQuality(strategy string, chunks []ChunkSample, opts ChunkQualityOptions) ChunkStrategyQuality {
	sizes := make([]int, len(chunks))
	oversized := 0
	undersized := 0
	boundaryHits := 0
	sectionCount := 0
	headingPathCount := 0
	codeChunks := 0
	codeLangDetected := 0

	for i, c := range chunks {
		textLen := len([]rune(c.Text))
		sizes[i] = textLen
		if textLen > opts.ExpectedChunkSize {
			oversized++
		}
		if textLen < opts.MinChunkSize {
			undersized++
		}
		if startsAtNaturalBoundary(c.Text) {
			boundaryHits++
		}
		if metadataString(c.Metadata, "section") != "" {
			sectionCount++
		}
		if hasNonEmptySlice(c.Metadata, "heading_path") {
			headingPathCount++
		}
		if hasNonEmptySlice(c.Metadata, "code_language") || metadataString(c.Metadata, "code_language") != "" {
			codeLangDetected++
		}
		if isCodeChunk(c.Metadata) {
			codeChunks++
		}
	}

	n := len(chunks)
	avg := mean(sizes)
	std := stddev(sizes, avg)

	boundaryQual := 0.0
	if n > 0 {
		boundaryQual = float64(boundaryHits) / float64(n)
	}
	sectionCov := 0.0
	if n > 0 {
		sectionCov = float64(sectionCount) / float64(n)
	}
	headingCov := 0.0
	if n > 0 {
		headingCov = float64(headingPathCount) / float64(n)
	}
	codeCov := 0.0
	if codeChunks > 0 {
		codeCov = float64(codeLangDetected) / float64(codeChunks)
	}

	return ChunkStrategyQuality{
		Strategy:            strategy,
		ChunkCount:          n,
		AverageSize:         avg,
		SizeStdDev:          std,
		MinSize:             minInt(sizes),
		MaxSize:             maxInt(sizes),
		OversizedCount:      oversized,
		UndersizedCount:     undersized,
		BoundaryQuality:     boundaryQual,
		SectionCoverage:     sectionCov,
		HeadingPathCoverage: headingCov,
		CodeLangCoverage:    codeCov,
	}
}

func computeOverallQuality(summaries []ChunkStrategyQuality, allSizes []int, opts ChunkQualityOptions) ChunkStrategyQuality {
	if len(summaries) == 0 {
		return ChunkStrategyQuality{Strategy: "overall"}
	}

	totalChunks := 0
	totalOversized := 0
	totalUndersized := 0
	totalBoundaryHits := 0
	totalSection := 0
	totalHeading := 0
	totalCodeChunks := 0
	totalCodeLang := 0

	for _, s := range summaries {
		totalChunks += s.ChunkCount
		totalOversized += s.OversizedCount
		totalUndersized += s.UndersizedCount
		totalBoundaryHits += int(s.BoundaryQuality * float64(s.ChunkCount))
		totalSection += int(s.SectionCoverage * float64(s.ChunkCount))
		totalHeading += int(s.HeadingPathCoverage * float64(s.ChunkCount))
		totalCodeChunks += s.ChunkCount // approximate; code chunks tracked per-strategy
		totalCodeLang += int(s.CodeLangCoverage * float64(s.ChunkCount))
	}

	avg := mean(allSizes)
	std := stddev(allSizes, avg)

	n := totalChunks
	return ChunkStrategyQuality{
		Strategy:            "overall",
		ChunkCount:          n,
		AverageSize:         avg,
		SizeStdDev:          std,
		MinSize:             minInt(allSizes),
		MaxSize:             maxInt(allSizes),
		OversizedCount:      totalOversized,
		UndersizedCount:     totalUndersized,
		BoundaryQuality:     safeDiv(float64(totalBoundaryHits), float64(n)),
		SectionCoverage:     safeDiv(float64(totalSection), float64(n)),
		HeadingPathCoverage: safeDiv(float64(totalHeading), float64(n)),
		CodeLangCoverage:    safeDiv(float64(totalCodeLang), float64(n)),
	}
}

// startsAtNaturalBoundary checks whether text starts at a sentence, paragraph,
// or heading boundary, which indicates the chunker split at a reasonable place.
func startsAtNaturalBoundary(text string) bool {
	if text == "" {
		return false
	}
	// blank line = paragraph boundary
	if strings.HasPrefix(text, "\n") {
		return true
	}
	// markdown heading (may be indented)
	trimmed := strings.TrimLeft(text, " \t")
	if strings.HasPrefix(trimmed, "#") {
		return true
	}
	// Chinese outline prefixes: 第X章、一、二、
	for _, prefix := range []string{"第", "一、", "二、", "三、", "四、", "五、", "六、", "七、", "八、", "九、", "十、"} {
		if strings.HasPrefix(trimmed, prefix) {
			return true
		}
	}
	return false
}

func isCodeChunk(metadata map[string]any) bool {
	if metadata == nil {
		return false
	}
	_, hasCodeLang := metadata["code_language"]
	if hasCodeLang {
		return true
	}
	_, hasHeading := metadata["heading_path"]
	_, hasSection := metadata["section"]
	// pure code blocks may have neither heading nor section
	return !hasHeading && !hasSection && metadataString(metadata, "document_name") != ""
}

// Helper functions

func mean(values []int) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += float64(v)
	}
	return sum / float64(len(values))
}

func stddev(values []int, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}
	var sumSquares float64
	for _, v := range values {
		diff := float64(v) - mean
		sumSquares += diff * diff
	}
	return math.Sqrt(sumSquares / float64(len(values)-1))
}

func minInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v < m {
			m = v
		}
	}
	return m
}

func maxInt(values []int) int {
	if len(values) == 0 {
		return 0
	}
	m := values[0]
	for _, v := range values[1:] {
		if v > m {
			m = v
		}
	}
	return m
}

func safeDiv(a, b float64) float64 {
	if b == 0 {
		return 0
	}
	return a / b
}

func hasNonEmptySlice(metadata map[string]any, key string) bool {
	if metadata == nil {
		return false
	}
	v, ok := metadata[key]
	if !ok || v == nil {
		return false
	}
	switch typed := v.(type) {
	case []string:
		for _, s := range typed {
			if strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	}
	return false
}
