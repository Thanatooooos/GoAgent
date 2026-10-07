package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"gorm.io/gorm"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	"local/rag-project/internal/framework/config"
)

var (
	recordPattern         = regexp.MustCompile(`(?s)"query"\s*:\s*"(?P<query>.*?)",\s*"answer"\s*:\s*"(?P<answer>.*?)",\s*"document"\s*:\s*\[(?P<evidence>.*?)\],\s*"title"\s*:\s*"(?P<title>.*?)",\s*(?:"class"\s*:\s*".*?",\s*)?"document_start"\s*:\s*(?P<start>\d+)`)
	recordSeparator       = regexp.MustCompile(`\r?\n\s*},\s*\r?\n\s*{`)
	structuralQuoteRepair = regexp.MustCompile(`\?(\s*[,}\]])`)
)

type sourceQuestion struct {
	Query    string
	Answer   string
	Evidence []string
	Title    string
	Start    int
}

type chunk struct {
	ID            string
	DocumentID    string
	ParentChunkID string
	Index         int
	CharCount     int
	Content       string
}

type document struct {
	ID   string
	Name string
}

type candidate struct {
	ChunkID       string `json:"chunkId"`
	ParentChunkID string `json:"parentChunkId"`
	ChunkIndex    int    `json:"chunkIndex"`
	CharCount     int    `json:"charCount"`
	EvidenceIndex int    `json:"evidenceIndex"`
	Exact         bool   `json:"exactEvidenceMatch"`
	LongestRun    int    `json:"longestCommonRunChars"`
	Content       string `json:"content"`
}

type reviewQuestion struct {
	ID                    string      `json:"id"`
	Title                 string      `json:"title"`
	Query                 string      `json:"query"`
	Answer                string      `json:"answer"`
	Evidence              []string    `json:"datasetEvidence"`
	DatasetEvidenceStart  int         `json:"datasetEvidenceStart"`
	ExactEvidenceChildIDs []string    `json:"exactEvidenceChildIds"`
	Candidates            []candidate `json:"candidatesForHumanReview"`
}

type reviewReport struct {
	KnowledgeBaseName string           `json:"knowledgeBaseName"`
	QuestionCount     int              `json:"questionCount"`
	ExactMappedCount  int              `json:"questionsWithExactEvidenceChild"`
	ReviewRequired    int              `json:"questionsWithoutExactEvidenceChild"`
	Questions         []reviewQuestion `json:"questions"`
}

func main() {
	sourcePath := flag.String("source", "tmp/legal-dc/Legal-DC/data/pro_LawQA.json", "Legal-DC QA source")
	kbName := flag.String("kb", "legal-dc-pilot-10", "knowledge base name")
	output := flag.String("output", "tmp/legal-dc/pilot-10-label-candidates.json", "JSON audit report")
	markdownOutput := flag.String("markdown-output", "tmp/legal-dc/pilot-10-label-candidates.md", "Markdown audit report")
	flag.Parse()

	questions, err := loadQuestions(*sourcePath)
	if err != nil {
		fatalf("load Legal-DC questions: %v", err)
	}
	if err := config.LoadConfig("configs"); err != nil {
		fatalf("load config: %v", err)
	}
	db, err := postgresrepo.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		fatalf("connect database: %v", err)
	}
	defer closeDB(db)

	var kb struct{ ID string }
	if err := db.Raw(`SELECT id FROM t_knowledge_base WHERE name = ? AND deleted = 0`, *kbName).Scan(&kb).Error; err != nil {
		fatalf("find knowledge base: %v", err)
	}
	if kb.ID == "" {
		fatalf("knowledge base %q not found", *kbName)
	}
	var documents []document
	if err := db.Raw(`SELECT id, doc_name AS name FROM t_knowledge_document WHERE kb_id = ? AND deleted = 0`, kb.ID).Scan(&documents).Error; err != nil {
		fatalf("load documents: %v", err)
	}
	var chunks []chunk
	if err := db.Raw(`SELECT id, doc_id AS document_id, parent_chunk_id, chunk_index AS "index", char_count, content FROM t_knowledge_chunk WHERE kb_id = ? AND deleted = 0 AND record_type = 'child'`, kb.ID).Scan(&chunks).Error; err != nil {
		fatalf("load child chunks: %v", err)
	}

	byTitle := make(map[string]document, len(documents))
	for _, doc := range documents {
		key := titleKey(strings.TrimSuffix(doc.Name, filepath.Ext(doc.Name)))
		if _, exists := byTitle[key]; exists {
			fatalf("ambiguous normalized document title %q", key)
		}
		byTitle[key] = doc
	}
	chunksByDocument := make(map[string][]chunk)
	for _, item := range chunks {
		chunksByDocument[item.DocumentID] = append(chunksByDocument[item.DocumentID], item)
	}

	report := reviewReport{KnowledgeBaseName: *kbName}
	matchedByTitle := make(map[string]int)
	for _, question := range questions {
		doc, ok := byTitle[titleKey(question.Title)]
		if !ok {
			continue
		}
		matchedByTitle[titleKey(question.Title)]++
		entry := buildReviewQuestion(len(report.Questions)+1, question, chunksByDocument[doc.ID])
		if len(entry.ExactEvidenceChildIDs) > 0 {
			report.ExactMappedCount++
		} else {
			report.ReviewRequired++
		}
		report.Questions = append(report.Questions, entry)
	}
	report.QuestionCount = len(report.Questions)
	if report.QuestionCount != 52 {
		var documentCounts []string
		for title := range byTitle {
			documentCounts = append(documentCounts, fmt.Sprintf("%s (%d)", title, matchedByTitle[title]))
		}
		sort.Strings(documentCounts)
		fatalf("expected 52 selected questions, got %d; document title counts: %s", report.QuestionCount, strings.Join(documentCounts, "; "))
	}
	writeJSON(*output, report)
	writeMarkdown(*markdownOutput, report)
	fmt.Printf("PASS questions=%d exact_mapped=%d review_required=%d json=%s markdown=%s\n", report.QuestionCount, report.ExactMappedCount, report.ReviewRequired, *output, *markdownOutput)
}

func loadQuestions(path string) ([]sourceQuestion, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	// The public QA file has a few broken closing quotes rendered as '?' directly
	// before JSON delimiters. Repair only that structural form in memory.
	repaired := structuralQuoteRepair.ReplaceAllString(string(raw), `"$1`)
	records := recordSeparator.Split(repaired, -1)
	if len(records) != 2474 {
		return nil, fmt.Errorf("split %d records, want 2474", len(records))
	}
	names := recordPattern.SubexpNames()
	index := make(map[string]int, len(names))
	for i, name := range names {
		index[name] = i
	}
	result := make([]sourceQuestion, 0, len(records))
	for recordIndex, record := range records {
		match := recordPattern.FindStringSubmatch(record)
		if match == nil {
			return nil, fmt.Errorf("parse record %d", recordIndex+1)
		}
		var evidence []string
		if err := json.Unmarshal([]byte("["+match[index["evidence"]]+"]"), &evidence); err != nil {
			return nil, fmt.Errorf("decode evidence for %q: %w", match[index["title"]], err)
		}
		result = append(result, sourceQuestion{
			Query: match[index["query"]], Answer: match[index["answer"]], Evidence: evidence,
			Title: match[index["title"]], Start: parseInt(match[index["start"]]),
		})
	}
	return result, nil
}

func buildReviewQuestion(number int, question sourceQuestion, chunks []chunk) reviewQuestion {
	entry := reviewQuestion{
		ID: fmt.Sprintf("legal-dc-pilot-10-q%02d", number), Title: question.Title, Query: question.Query,
		Answer: question.Answer, Evidence: question.Evidence, DatasetEvidenceStart: question.Start,
	}
	seenExact := make(map[string]bool)
	for evidenceIndex, evidence := range question.Evidence {
		normalizedEvidence := normalize(evidence)
		for _, item := range chunks {
			normalizedChunk := normalize(item.Content)
			exact := normalizedEvidence != "" && strings.Contains(normalizedChunk, normalizedEvidence)
			run := longestCommonRun(normalizedEvidence, normalizedChunk)
			if !exact && run < 24 {
				continue
			}
			entry.Candidates = append(entry.Candidates, candidate{
				ChunkID: item.ID, ParentChunkID: item.ParentChunkID, ChunkIndex: item.Index, CharCount: item.CharCount,
				EvidenceIndex: evidenceIndex, Exact: exact, LongestRun: run, Content: item.Content,
			})
			if exact && !seenExact[item.ID] {
				entry.ExactEvidenceChildIDs = append(entry.ExactEvidenceChildIDs, item.ID)
				seenExact[item.ID] = true
			}
		}
	}
	sort.Slice(entry.Candidates, func(i, j int) bool {
		if entry.Candidates[i].Exact != entry.Candidates[j].Exact {
			return entry.Candidates[i].Exact
		}
		if entry.Candidates[i].LongestRun != entry.Candidates[j].LongestRun {
			return entry.Candidates[i].LongestRun > entry.Candidates[j].LongestRun
		}
		return entry.Candidates[i].ChunkID < entry.Candidates[j].ChunkID
	})
	if len(entry.Candidates) > 8 {
		entry.Candidates = entry.Candidates[:8]
	}
	return entry
}

func normalize(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '\u00a0' {
			return -1
		}
		return r
	}, text)
}

func titleKey(title string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || strings.ContainsRune("《》〈〉()（）", r) {
			return -1
		}
		return r
	}, title)
}

func longestCommonRun(left, right string) int {
	a, b := []rune(left), []rune(right)
	previous := make([]int, len(b)+1)
	best := 0
	for _, ar := range a {
		current := make([]int, len(b)+1)
		for j, br := range b {
			if ar != br {
				continue
			}
			current[j+1] = previous[j] + 1
			if current[j+1] > best {
				best = current[j+1]
			}
		}
		previous = current
	}
	return best
}

func writeJSON(path string, report reviewReport) {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		fatalf("encode report: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatalf("write report: %v", err)
	}
}

func writeMarkdown(path string, report reviewReport) {
	var out strings.Builder
	fmt.Fprintf(&out, "# Legal-DC 10 篇试点：候选正例审查\n\n题目：%d；有完整证据子块：%d；需人工确认：%d。\n\n", report.QuestionCount, report.ExactMappedCount, report.ReviewRequired)
	for _, question := range report.Questions {
		fmt.Fprintf(&out, "## %s｜%s\n\n问题：%s\n\n", question.ID, question.Title, question.Query)
		fmt.Fprintf(&out, "答案：%s\n\n", question.Answer)
		for i, evidence := range question.Evidence {
			fmt.Fprintf(&out, "数据集证据 %d：%s\n\n", i+1, evidence)
		}
		if len(question.ExactEvidenceChildIDs) == 0 {
			out.WriteString("完整证据子块：无，以下仅是待审候选。\n\n")
		} else {
			fmt.Fprintf(&out, "完整证据子块：`%s`\n\n", strings.Join(question.ExactEvidenceChildIDs, "`, `"))
		}
		for _, item := range question.Candidates {
			kind := "待审高重叠"
			if item.Exact {
				kind = "完整证据命中"
			}
			fmt.Fprintf(&out, "- %s｜子块 `%s`，父块 `%s`，证据 %d，连续重叠 %d 字\n\n  %s\n\n", kind, item.ChunkID, item.ParentChunkID, item.EvidenceIndex+1, item.LongestRun, item.Content)
		}
	}
	if err := os.WriteFile(path, []byte(out.String()), 0o644); err != nil {
		fatalf("write markdown report: %v", err)
	}
}

func parseInt(value string) int {
	var result int
	_, _ = fmt.Sscanf(value, "%d", &result)
	return result
}

func closeDB(db *gorm.DB) {
	if db == nil {
		return
	}
	sqlDB, err := db.DB()
	if err == nil {
		_ = sqlDB.Close()
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
