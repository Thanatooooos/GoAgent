package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
)

type labelsFile struct{ Decisions []label }
type label struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Positives []string `json:"positiveChildIds"`
}
type decisionsFile struct{ Decisions []negativeDecision }
type negativeDecision struct {
	QuestionID string   `json:"questionId"`
	Negatives  []string `json:"negativeChildIds"`
}
type candidateQuestion struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Query string `json:"query"`
	Same  []struct {
		ID string `json:"chunkId"`
	} `json:"sameDocumentCandidates"`
	Cross []struct {
		ID string `json:"chunkId"`
	} `json:"crossDocumentCandidates"`
}
type evaluationQuestion struct {
	ID                   string   `json:"id"`
	Title                string   `json:"title"`
	Query                string   `json:"query"`
	PositiveChildIDs     []string `json:"positiveChildIds"`
	HardNegativeChildIDs []string `json:"hardNegativeChildIds"`
	NoSafeHardNegative   bool     `json:"noSafeHardNegative"`
}
type evaluationSet struct {
	Name      string               `json:"name"`
	Version   string               `json:"version"`
	Scoring   string               `json:"scoring"`
	Questions []evaluationQuestion `json:"questions"`
}
type retrievalSample struct {
	Name                 string   `json:"name"`
	Query                string   `json:"query"`
	Tags                 []string `json:"tags"`
	Target               string   `json:"target"`
	ExpectedIDs          []string `json:"expectedIds"`
	KnowledgeBaseIDs     []string `json:"knowledgeBaseIds"`
	SearchMode           string   `json:"searchMode"`
	TopK                 int      `json:"topK"`
	ChunkStrategy        string   `json:"chunkStrategy"`
	HardNegativeChildIDs []string `json:"hardNegativeChildIds"`
}
type retrievalSamplesFile struct {
	Samples []retrievalSample `json:"samples"`
}

func main() {
	labelsPath := flag.String("labels", "tmp/legal-dc/pilot-10-manual-labels.json", "manual positive labels")
	decisionsPath := flag.String("decisions", "tmp/legal-dc/pilot-10-confusion-decisions.json", "manual hard-negative decisions")
	candidatesPath := flag.String("candidates", "tmp/legal-dc/pilot-10-confusion-candidates.json", "candidate provenance")
	outputPath := flag.String("output", "tmp/legal-dc/pilot-10-retrieval-eval.json", "evaluation-set output")
	samplesOutputPath := flag.String("samples-output", "tmp/legal-dc/pilot-10-retrieval-samples.json", "retrieve-eval compatible samples output")
	kbID := flag.String("knowledge-base-id", "37733184477131009", "pilot knowledge base ID")
	flag.Parse()

	var labels labelsFile
	readJSON(*labelsPath, &labels)
	var decisions decisionsFile
	readJSON(*decisionsPath, &decisions)
	var candidates []candidateQuestion
	readJSON(*candidatesPath, &candidates)

	negativeByQuestion := map[string][]string{}
	for _, decision := range decisions.Decisions {
		negativeByQuestion[decision.QuestionID] = decision.Negatives
	}
	candidateByQuestion := map[string]candidateQuestion{}
	for _, candidate := range candidates {
		candidateByQuestion[candidate.ID] = candidate
	}

	set := evaluationSet{
		Name:    "legal-dc-pilot-10-retrieval",
		Version: "manual-v1",
		Scoring: "A query succeeds when any positive child chunk is retrieved; hard negatives are diagnostic-only and never count as success.",
	}
	positiveCount, negativeCount := 0, 0
	samples := retrievalSamplesFile{}
	for _, item := range labels.Decisions {
		if item.Status != "accepted" && item.Status != "accepted_with_outdated_term" {
			continue
		}
		candidate, ok := candidateByQuestion[item.ID]
		if !ok {
			fatal("missing candidate provenance for " + item.ID)
		}
		negatives := negativeByQuestion[item.ID]
		checkIDs(item.ID, item.Positives, negatives, candidate)
		set.Questions = append(set.Questions, evaluationQuestion{
			ID: item.ID, Title: candidate.Title, Query: candidate.Query,
			PositiveChildIDs: item.Positives, HardNegativeChildIDs: negatives,
			NoSafeHardNegative: len(negatives) == 0,
		})
		samples.Samples = append(samples.Samples, retrievalSample{
			Name: item.ID, Query: candidate.Query,
			Tags:   []string{"legal-dc", "manual-reviewed", "confusion-pool", "semantic"},
			Target: "chunk", ExpectedIDs: item.Positives,
			KnowledgeBaseIDs: []string{*kbID}, SearchMode: "semantic", TopK: 30,
			ChunkStrategy: "parent-1200-child-300-overlap-60", HardNegativeChildIDs: negatives,
		})
		positiveCount += len(item.Positives)
		negativeCount += len(negatives)
	}
	sort.Slice(set.Questions, func(i, j int) bool { return set.Questions[i].ID < set.Questions[j].ID })
	if len(set.Questions) != 50 || positiveCount != 73 || negativeCount != 140 {
		fatal(fmt.Sprintf("unexpected totals questions=%d positives=%d negatives=%d", len(set.Questions), positiveCount, negativeCount))
	}
	writeJSON(*outputPath, set)
	writeJSON(*samplesOutputPath, samples)
	fmt.Printf("PASS questions=%d positives=%d hard_negatives=%d output=%s samples=%s\n", len(set.Questions), positiveCount, negativeCount, *outputPath, *samplesOutputPath)
}

func checkIDs(questionID string, positives, negatives []string, candidate candidateQuestion) {
	pool := map[string]bool{}
	for _, item := range candidate.Same {
		pool[item.ID] = true
	}
	for _, item := range candidate.Cross {
		pool[item.ID] = true
	}
	positive := map[string]bool{}
	for _, id := range positives {
		positive[id] = true
	}
	for _, id := range negatives {
		if positive[id] {
			fatal(questionID + " has overlapping positive and hard negative " + id)
		}
		if !pool[id] {
			fatal(questionID + " hard negative outside candidate pool " + id)
		}
	}
}

func readJSON(path string, target any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal(err.Error())
	}
	if err := json.Unmarshal(data, target); err != nil {
		fatal(err.Error())
	}
}
func writeJSON(path string, value any) {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		fatal(err.Error())
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fatal(err.Error())
	}
}
func fatal(message string) { fmt.Fprintln(os.Stderr, message); os.Exit(1) }
