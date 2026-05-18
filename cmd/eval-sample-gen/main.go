package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

type manifestFile struct {
	KnowledgeBase markdownKnowledgeBaseRef `json:"knowledgeBase"`
	ChunkStrategy string                   `json:"chunkStrategy"`
	Documents     []manifestDocument       `json:"documents"`
}

type markdownKnowledgeBaseRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type manifestDocument struct {
	DocumentID   string          `json:"documentId"`
	DocumentName string          `json:"documentName"`
	RelativePath string          `json:"relativePath"`
	Chunks       []manifestChunk `json:"chunks"`
}

type manifestChunk struct {
	ChunkID  string         `json:"chunkId"`
	Index    int            `json:"index"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata"`
}

type sampleFile struct {
	Samples []rageval.Sample `json:"samples"`
}

type sectionGroup struct {
	Name   string
	Chunks []manifestChunk
}

func main() {
	manifestPath := flag.String("manifest", "", "path to a markdown corpus manifest JSON file")
	outputPath := flag.String("output", "", "output path for generated evaluation samples")
	topK := flag.Int("top-k", 10, "default TopK for generated samples")
	sectionsPerDoc := flag.Int("sections-per-doc", 2, "max section-based query groups generated per document")
	flag.Parse()

	if strings.TrimSpace(*manifestPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/eval-sample-gen -manifest <manifest.json> [-output samples.json] [-top-k 10] [-sections-per-doc 2]")
		os.Exit(1)
	}
	if *sectionsPerDoc <= 0 {
		fmt.Fprintln(os.Stderr, "sections-per-doc must be positive")
		os.Exit(1)
	}
	if *topK <= 0 {
		fmt.Fprintln(os.Stderr, "top-k must be positive")
		os.Exit(1)
	}

	manifest, err := loadManifest(*manifestPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load manifest failed: %v\n", err)
		os.Exit(1)
	}
	if strings.TrimSpace(*outputPath) == "" {
		*outputPath = defaultSamplesPath(*manifestPath)
	}

	samples := generateSamples(manifest, *topK, *sectionsPerDoc)
	if len(samples) == 0 {
		fmt.Fprintln(os.Stderr, "no samples generated")
		os.Exit(1)
	}

	if err := writeJSON(*outputPath, sampleFile{Samples: samples}); err != nil {
		fmt.Fprintf(os.Stderr, "write samples failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "generated %d samples -> %s\n", len(samples), *outputPath)
}

func loadManifest(path string) (manifestFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return manifestFile{}, err
	}
	var manifest manifestFile
	if err := json.Unmarshal(data, &manifest); err != nil {
		return manifestFile{}, err
	}
	return manifest, nil
}

func generateSamples(manifest manifestFile, topK int, sectionsPerDoc int) []rageval.Sample {
	chunkStrategy := strings.TrimSpace(manifest.ChunkStrategy)
	if chunkStrategy == "" {
		chunkStrategy = "unknown"
	}

	samples := make([]rageval.Sample, 0)
	for _, doc := range manifest.Documents {
		if strings.TrimSpace(doc.DocumentID) == "" || strings.TrimSpace(doc.DocumentName) == "" {
			continue
		}

		kbIDs := []string{}
		if strings.TrimSpace(manifest.KnowledgeBase.ID) != "" {
			kbIDs = []string{strings.TrimSpace(manifest.KnowledgeBase.ID)}
		}

		samples = append(samples, rageval.Sample{
			Name:             "file_lookup_" + doc.DocumentID,
			Query:            fmt.Sprintf("查找文件 %s", doc.DocumentName),
			Tags:             []string{"markdown", chunkStrategy, "keyword", "file_name", "metadata_title"},
			Target:           rageval.TargetSourceFileName,
			ExpectedIDs:      []string{doc.DocumentName},
			KnowledgeBaseIDs: kbIDs,
			SearchMode:       "keyword",
			TopK:             topK,
			ChunkStrategy:    chunkStrategy,
		})

		sections := groupSections(doc.Chunks)
		limit := min(sectionsPerDoc, len(sections))
		for i := 0; i < limit; i++ {
			section := sections[i]
			samples = append(samples,
				buildSectionMetadataSample(doc, section, kbIDs, topK, chunkStrategy),
				buildSectionSemanticSample(doc, section, kbIDs, topK, chunkStrategy),
			)
		}

		if len(sections) == 0 && len(doc.Chunks) > 0 {
			samples = append(samples, buildFallbackChunkSample(doc, doc.Chunks[0], kbIDs, topK, chunkStrategy))
		}
	}
	return samples
}

func buildSectionMetadataSample(doc manifestDocument, section sectionGroup, kbIDs []string, topK int, chunkStrategy string) rageval.Sample {
	return rageval.Sample{
		Name:             fmt.Sprintf("section_lookup_%s_%s", doc.DocumentID, sampleToken(section.Name)),
		Query:            fmt.Sprintf("查找 %s 这一节", section.Name),
		Tags:             []string{"markdown", chunkStrategy, "keyword", "section", "metadata_title"},
		Target:           rageval.TargetSection,
		ExpectedIDs:      []string{section.Name},
		KnowledgeBaseIDs: kbIDs,
		SearchMode:       "keyword",
		TopK:             topK,
		ChunkStrategy:    chunkStrategy,
	}
}

func buildSectionSemanticSample(doc manifestDocument, section sectionGroup, kbIDs []string, topK int, chunkStrategy string) rageval.Sample {
	expectedIDs := make([]string, 0, len(section.Chunks))
	expectedRelevance := make(map[string]int, len(section.Chunks))
	for i, chunk := range section.Chunks {
		expectedIDs = append(expectedIDs, chunk.ChunkID)
		grade := 2
		if i == 0 {
			grade = 3
		}
		expectedRelevance[chunk.ChunkID] = grade
	}
	return rageval.Sample{
		Name:              fmt.Sprintf("section_semantic_%s_%s", doc.DocumentID, sampleToken(section.Name)),
		Query:             fmt.Sprintf("%s 主要讲什么", lastSectionSegment(section.Name)),
		Tags:              []string{"markdown", chunkStrategy, "semantic", "section"},
		Target:            rageval.TargetChunk,
		ExpectedIDs:       expectedIDs,
		KnowledgeBaseIDs:  kbIDs,
		SearchMode:        "semantic",
		TopK:              topK,
		ChunkStrategy:     chunkStrategy,
		ExpectedRelevance: expectedRelevance,
	}
}

func buildFallbackChunkSample(doc manifestDocument, chunk manifestChunk, kbIDs []string, topK int, chunkStrategy string) rageval.Sample {
	return rageval.Sample{
		Name:             fmt.Sprintf("chunk_fallback_%s_%s", doc.DocumentID, sampleToken(chunk.ChunkID)),
		Query:            fallbackChunkQuery(chunk),
		Tags:             []string{"markdown", chunkStrategy, "semantic", "fallback"},
		Target:           rageval.TargetChunk,
		ExpectedIDs:      []string{chunk.ChunkID},
		KnowledgeBaseIDs: kbIDs,
		SearchMode:       "semantic",
		TopK:             topK,
		ChunkStrategy:    chunkStrategy,
	}
}

func groupSections(chunks []manifestChunk) []sectionGroup {
	grouped := make(map[string][]manifestChunk)
	order := make([]string, 0)
	for _, chunk := range chunks {
		section := metadataString(chunk.Metadata, "section")
		if section == "" {
			continue
		}
		if _, ok := grouped[section]; !ok {
			order = append(order, section)
		}
		grouped[section] = append(grouped[section], chunk)
	}

	result := make([]sectionGroup, 0, len(order))
	for _, name := range order {
		items := grouped[name]
		sort.Slice(items, func(i, j int) bool {
			return items[i].Index < items[j].Index
		})
		result = append(result, sectionGroup{Name: name, Chunks: items})
	}
	sort.SliceStable(result, func(i, j int) bool {
		if len(result[i].Chunks) == len(result[j].Chunks) {
			return result[i].Name < result[j].Name
		}
		return len(result[i].Chunks) > len(result[j].Chunks)
	})
	return result
}

func fallbackChunkQuery(chunk manifestChunk) string {
	if section := metadataString(chunk.Metadata, "section"); section != "" {
		return fmt.Sprintf("%s 主要讲什么", lastSectionSegment(section))
	}
	content := stripMarkdownSyntax(chunk.Content)
	content = normalizeWhitespace(content)
	content = trimRunes(content, 36)
	if content == "" {
		return "这个内容主要讲什么"
	}
	return fmt.Sprintf("%s 讲了什么", content)
}

func lastSectionSegment(section string) string {
	parts := strings.Split(section, ">")
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return strings.TrimSpace(section)
	}
	return last
}

func metadataString(metadata map[string]any, key string) string {
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

func stripMarkdownSyntax(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	cleaned := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.TrimLeft(line, "#-*`> ")
		line = strings.TrimSpace(line)
		if line != "" {
			cleaned = append(cleaned, line)
		}
	}
	return strings.Join(cleaned, " ")
}

func normalizeWhitespace(text string) string {
	var buf bytes.Buffer
	space := false
	for _, r := range text {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if space {
				continue
			}
			space = true
			buf.WriteRune(' ')
			continue
		}
		space = false
		buf.WriteRune(r)
	}
	return strings.TrimSpace(buf.String())
}

func trimRunes(text string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit])
}

func sampleToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	replacer := strings.NewReplacer(" ", "_", "/", "_", "\\", "_", ">", "_", ":", "_")
	value = replacer.Replace(value)
	value = strings.Trim(value, "_")
	if value == "" {
		return "sample"
	}
	return value
}

func writeJSON(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func defaultSamplesPath(manifestPath string) string {
	trimmed := strings.TrimSuffix(manifestPath, filepath.Ext(manifestPath))
	return trimmed + "_samples.json"
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
