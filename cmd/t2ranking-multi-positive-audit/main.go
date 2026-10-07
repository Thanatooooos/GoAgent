// Command t2ranking-multi-positive-audit creates an auditable, immutable-fact
// review ledger for every passage used in the hardset.
package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
)

type plan struct {
	Items []planItem `json:"items"`
}

type planItem struct {
	QueryID     string   `json:"queryId"`
	PositiveID  string   `json:"positiveId"`
	NegativeIDs []string `json:"negativeIds"`
}

type row struct {
	SchemaVersion string `json:"schemaVersion"`
	QueryID       string `json:"queryId"`
	Query         string `json:"query"`
	PassageID     string `json:"passageId"`
	OriginalRole  string `json:"originalRole"`
	PassageSHA256 string `json:"passageSha256"`
	Passage       string `json:"passage"`
	Label         string `json:"label"`
	AnswerPoints  string `json:"answerPoints"`
	Rationale     string `json:"rationale"`
}

func main() {
	planPath := flag.String("plan", "tmp/t2ranking-dev/hardset-plan.json", "hardset selection plan")
	queriesPath := flag.String("queries", "tmp/t2ranking-dev/queries.dev.tsv", "source queries TSV")
	corpusPath := flag.String("corpus", "tmp/t2ranking-dev/hardset-corpus.tsv", "hardset corpus TSV")
	outputPath := flag.String("output", "tmp/t2ranking-dev/multi-positive-labels.jsonl", "review ledger JSONL")
	flag.Parse()

	items := mustPlan(*planPath)
	queries := mustTSV(*queriesPath)
	passages := mustTSV(*corpusPath)
	out, err := os.Create(*outputPath)
	if err != nil {
		fatalf("create ledger: %v", err)
	}
	defer out.Close()
	encoder := json.NewEncoder(out)
	count := 0
	for _, item := range items {
		query, ok := queries[item.QueryID]
		if !ok {
			fatalf("missing query %s", item.QueryID)
		}
		writeRow(encoder, item.QueryID, query, item.PositiveID, "official_positive", passages)
		count++
		for _, passageID := range item.NegativeIDs {
			writeRow(encoder, item.QueryID, query, passageID, "constructed_candidate", passages)
			count++
		}
	}
	fmt.Printf("rows=%d output=%s\n", count, *outputPath)
}

func writeRow(encoder *json.Encoder, queryID, query, passageID, role string, passages map[string]string) {
	passage, ok := passages[passageID]
	if !ok {
		fatalf("missing passage %s", passageID)
	}
	sum := sha256.Sum256([]byte(passage))
	value := row{
		SchemaVersion: "1",
		QueryID:       queryID,
		Query:         query,
		PassageID:     passageID,
		OriginalRole:  role,
		PassageSHA256: hex.EncodeToString(sum[:]),
		Passage:       passage,
		Label:         "pending",
	}
	if err := encoder.Encode(value); err != nil {
		fatalf("write ledger: %v", err)
	}
}

func mustPlan(path string) []planItem {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("read plan: %v", err)
	}
	var value plan
	if err := json.Unmarshal(data, &value); err != nil {
		fatalf("parse plan: %v", err)
	}
	if len(value.Items) == 0 {
		fatalf("plan has no items")
	}
	return value.Items
}

func mustTSV(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		fatalf("open TSV: %v", err)
	}
	defer f.Close()
	result := map[string]string{}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1024), 8*1024*1024)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "\t", 2)
		if len(parts) == 2 {
			result[parts[0]] = parts[1]
		}
	}
	if err := scanner.Err(); err != nil {
		fatalf("read TSV: %v", err)
	}
	return result
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
