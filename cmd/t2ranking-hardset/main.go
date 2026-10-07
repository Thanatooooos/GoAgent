package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

type query struct{ ID, Text string }
type qrel struct {
	QueryID, PassageID string
	Relevance          int
}
type passage struct{ ID, Text string }
type planItem struct {
	QueryID     string   `json:"queryId"`
	PositiveID  string   `json:"positiveId"`
	NegativeIDs []string `json:"negativeIds"`
}
type plan struct {
	QueryCount   int        `json:"queryCount"`
	PassageCount int        `json:"passageCount"`
	Items        []planItem `json:"items"`
}

func main() {
	queriesPath := flag.String("queries", "tmp/t2ranking-dev/queries.dev.tsv", "T2Ranking queries TSV")
	qrelsPath := flag.String("qrels", "tmp/t2ranking-dev/qrels.dev.tsv", "T2Ranking qrels TSV")
	corpusPath := flag.String("corpus", "tmp/t2ranking-dev/subset/corpus.tsv", "candidate corpus TSV")
	queryLimit := flag.Int("queries-limit", 1000, "number of source queries to inspect")
	targetQueries := flag.Int("target-queries", 50, "number of hard-set questions")
	negatives := flag.Int("negatives-per-query", 9, "hard negatives per question")
	outputCorpus := flag.String("output-corpus", "tmp/t2ranking-dev/hardset-corpus.tsv", "selected corpus TSV")
	outputPlan := flag.String("output-plan", "tmp/t2ranking-dev/hardset-plan.json", "selection plan JSON")
	outputQueryIDs := flag.String("output-query-ids", "tmp/t2ranking-dev/hardset-query-ids.txt", "selected query IDs output")
	flag.Parse()
	queries := mustQueries(*queriesPath, *queryLimit)
	rels := mustQrels(*qrelsPath, queries)
	passages := mustPassages(*corpusPath)
	byID := map[string]passage{}
	for _, p := range passages {
		byID[p.ID] = p
	}
	byQuery := map[string][]qrel{}
	for _, r := range rels {
		if r.Relevance > 0 {
			if _, ok := byID[r.PassageID]; ok {
				byQuery[r.QueryID] = append(byQuery[r.QueryID], r)
			}
		}
	}
	chosen := map[string]passage{}
	items := make([]planItem, 0, *targetQueries)
	for _, q := range queries {
		if len(items) >= *targetQueries {
			break
		}
		positives := byQuery[q.ID]
		if len(positives) == 0 {
			continue
		}
		sort.Slice(positives, func(i, j int) bool {
			if positives[i].Relevance != positives[j].Relevance {
				return positives[i].Relevance > positives[j].Relevance
			}
			return positives[i].PassageID < positives[j].PassageID
		})
		positive := positives[0].PassageID
		forbidden := map[string]struct{}{positive: {}}
		candidates := rankCandidates(q.Text, passages, forbidden)
		negativeIDs := make([]string, 0, *negatives)
		for _, candidate := range candidates {
			if _, exists := chosen[candidate.ID]; exists {
				continue
			}
			chosen[candidate.ID] = candidate
			negativeIDs = append(negativeIDs, candidate.ID)
			if len(negativeIDs) == *negatives {
				break
			}
		}
		if len(negativeIDs) != *negatives {
			break
		}
		chosen[positive] = byID[positive]
		items = append(items, planItem{QueryID: q.ID, PositiveID: positive, NegativeIDs: negativeIDs})
	}
	if len(items) != *targetQueries {
		fatalf("selected %d questions, want %d", len(items), *targetQueries)
	}
	ids := make([]string, 0, len(chosen))
	for id := range chosen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	f, err := os.Create(*outputCorpus)
	if err != nil {
		fatalf("create corpus: %v", err)
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	defer w.Flush()
	for _, id := range ids {
		fmt.Fprintf(w, "%s\t%s\n", id, byID[id].Text)
	}
	data, _ := json.MarshalIndent(plan{QueryCount: len(items), PassageCount: len(ids), Items: items}, "", "  ")
	if err := os.WriteFile(*outputPlan, data, 0o644); err != nil {
		fatalf("write plan: %v", err)
	}
	queryIDs := make([]string, 0, len(items))
	for _, item := range items {
		queryIDs = append(queryIDs, item.QueryID)
	}
	if err := os.WriteFile(*outputQueryIDs, []byte(strings.Join(queryIDs, "\n")+"\n"), 0o644); err != nil {
		fatalf("write query IDs: %v", err)
	}
	fmt.Printf("questions=%d passages=%d corpus=%s plan=%s\n", len(items), len(ids), *outputCorpus, *outputPlan)
}
func rankCandidates(q string, passages []passage, forbidden map[string]struct{}) []passage {
	grams := bigrams(q)
	type scored struct {
		p passage
		s int
	}
	rows := make([]scored, 0, len(passages))
	for _, p := range passages {
		if _, ok := forbidden[p.ID]; ok {
			continue
		}
		s := 0
		for g := range grams {
			if strings.Contains(p.Text, g) {
				s++
			}
		}
		if s > 0 {
			rows = append(rows, scored{p, s})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].s != rows[j].s {
			return rows[i].s > rows[j].s
		}
		return rows[i].p.ID < rows[j].p.ID
	})
	out := make([]passage, len(rows))
	for i := range rows {
		out[i] = rows[i].p
	}
	return out
}
func bigrams(s string) map[string]struct{} {
	r := []rune(strings.ReplaceAll(strings.TrimSpace(s), " ", ""))
	out := map[string]struct{}{}
	for i := 0; i+1 < len(r); i++ {
		out[string(r[i:i+2])] = struct{}{}
	}
	return out
}
func mustQueries(path string, limit int) []query {
	f, e := os.Open(path)
	if e != nil {
		fatalf("open queries: %v", e)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Scan()
	out := []query{}
	for s.Scan() && len(out) < limit {
		p := strings.SplitN(s.Text(), "\t", 2)
		if len(p) == 2 && p[0] != "" && p[1] != "" {
			out = append(out, query{p[0], p[1]})
		}
	}
	return out
}
func mustQrels(path string, queries []query) []qrel {
	want := map[string]struct{}{}
	for _, q := range queries {
		want[q.ID] = struct{}{}
	}
	f, e := os.Open(path)
	if e != nil {
		fatalf("open qrels: %v", e)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Scan()
	out := []qrel{}
	for s.Scan() {
		p := strings.Split(s.Text(), "\t")
		if len(p) != 4 {
			continue
		}
		if _, ok := want[p[0]]; !ok {
			continue
		}
		n, e := strconv.Atoi(p[3])
		if e == nil {
			out = append(out, qrel{p[0], p[2], n})
		}
	}
	return out
}
func mustPassages(path string) []passage {
	f, e := os.Open(path)
	if e != nil {
		fatalf("open corpus: %v", e)
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1024), 8*1024*1024)
	out := []passage{}
	for s.Scan() {
		p := strings.SplitN(s.Text(), "\t", 2)
		if len(p) == 2 {
			out = append(out, passage{p[0], p[1]})
		}
	}
	return out
}
func fatalf(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...); os.Exit(1) }
