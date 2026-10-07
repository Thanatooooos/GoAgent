package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"unicode"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	"local/rag-project/internal/framework/config"
)

type labelsFile struct{ Decisions []decision }
type decision struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Positives []string `json:"positiveChildIds"`
}
type candidateReport struct{ Questions []question }
type question struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Query string `json:"query"`
}
type document struct{ ID, Name string }
type chunk struct {
	ID         string
	DocumentID string `gorm:"column:document_id"`
	Index      int
	Content    string
}
type confusionCandidate struct {
	ChunkID              string `json:"chunkId"`
	DocumentName         string `json:"documentName"`
	ChunkIndex           int    `json:"chunkIndex"`
	LexicalBigramOverlap int    `json:"lexicalBigramOverlap"`
	Content              string `json:"content"`
}
type confusionQuestion struct {
	ID                      string               `json:"id"`
	Title                   string               `json:"title"`
	Query                   string               `json:"query"`
	SameDocumentCandidates  []confusionCandidate `json:"sameDocumentCandidates"`
	CrossDocumentCandidates []confusionCandidate `json:"crossDocumentCandidates"`
}

func main() {
	labelsPath := flag.String("labels", "tmp/legal-dc/pilot-10-manual-labels.json", "manual positive labels")
	reportPath := flag.String("report", "tmp/legal-dc/pilot-10-label-candidates.json", "question audit report")
	outputPath := flag.String("output", "tmp/legal-dc/pilot-10-confusion-candidates.json", "candidate output")
	flag.Parse()
	var labels labelsFile
	readJSON(*labelsPath, &labels)
	var report candidateReport
	readJSON(*reportPath, &report)
	if err := config.LoadConfig("configs"); err != nil {
		fatal(err)
	}
	db, err := postgresrepo.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		fatal(err)
	}
	sqlDB, _ := db.DB()
	defer sqlDB.Close()
	var kb struct{ ID string }
	if err := db.Raw(`SELECT id FROM t_knowledge_base WHERE name = 'legal-dc-pilot-10' AND deleted = 0`).Scan(&kb).Error; err != nil || kb.ID == "" {
		fatal(fmt.Errorf("pilot knowledge base not found: %w", err))
	}
	var docs []document
	if err := db.Raw(`SELECT id, doc_name AS name FROM t_knowledge_document WHERE kb_id = ? AND deleted = 0`, kb.ID).Scan(&docs).Error; err != nil {
		fatal(err)
	}
	var chunks []chunk
	if err := db.Raw(`SELECT id, doc_id AS document_id, chunk_index AS "index", content FROM t_knowledge_chunk WHERE kb_id = ? AND deleted = 0 AND record_type = 'child'`, kb.ID).Scan(&chunks).Error; err != nil {
		fatal(err)
	}
	docIDByTitle, docNameByID := map[string]string{}, map[string]string{}
	for _, d := range docs {
		title := strings.TrimSuffix(d.Name, ".docx")
		docIDByTitle[title] = d.ID
		docNameByID[d.ID] = d.Name
	}
	questionByID := map[string]question{}
	for _, q := range report.Questions {
		questionByID[q.ID] = q
	}
	output := make([]confusionQuestion, 0, 50)
	for _, label := range labels.Decisions {
		if !strings.HasPrefix(label.Status, "accepted") {
			continue
		}
		q := questionByID[label.ID]
		positive := map[string]bool{}
		for _, id := range label.Positives {
			positive[id] = true
		}
		same, cross := []confusionCandidate{}, []confusionCandidate{}
		for _, c := range chunks {
			if positive[c.ID] {
				continue
			}
			candidate := confusionCandidate{ChunkID: c.ID, DocumentName: docNameByID[c.DocumentID], ChunkIndex: c.Index, LexicalBigramOverlap: overlap(q.Query, c.Content), Content: c.Content}
			if c.DocumentID == docIDByTitle[q.Title] {
				same = append(same, candidate)
			} else if candidate.LexicalBigramOverlap > 0 {
				cross = append(cross, candidate)
			}
		}
		sortCandidates(same)
		sortCandidates(cross)
		if len(same) > 5 {
			same = same[:5]
		}
		if len(cross) > 3 {
			cross = cross[:3]
		}
		output = append(output, confusionQuestion{ID: q.ID, Title: q.Title, Query: q.Query, SameDocumentCandidates: same, CrossDocumentCandidates: cross})
	}
	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*outputPath, data, 0644); err != nil {
		fatal(err)
	}
	fmt.Printf("PASS questions=%d output=%s\n", len(output), *outputPath)
}

func overlap(a, b string) int {
	x, y := grams(a), grams(b)
	n := 0
	for g := range x {
		if y[g] {
			n++
		}
	}
	return n
}
func grams(s string) map[string]bool {
	r := []rune(strings.Map(func(x rune) rune {
		if unicode.IsLetter(x) || unicode.IsDigit(x) {
			return x
		}
		return -1
	}, s))
	out := map[string]bool{}
	for i := 0; i+1 < len(r); i++ {
		out[string(r[i:i+2])] = true
	}
	return out
}
func sortCandidates(items []confusionCandidate) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].LexicalBigramOverlap != items[j].LexicalBigramOverlap {
			return items[i].LexicalBigramOverlap > items[j].LexicalBigramOverlap
		}
		return items[i].ChunkID < items[j].ChunkID
	})
}
func readJSON(path string, target any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal(err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		fatal(err)
	}
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
