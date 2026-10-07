// Command t2ranking-multi-positive-review produces an LLM-assisted first-pass
// review for the hardset. Manual decisions remain authoritative when merged.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
	"local/rag-project/internal/framework/convention"
	infraai "local/rag-project/internal/infra-ai"
	aichat "local/rag-project/internal/infra-ai/chat"
)

type plan struct {
	Items []planItem `json:"items"`
}
type planItem struct {
	QueryID     string   `json:"queryId"`
	PositiveID  string   `json:"positiveId"`
	NegativeIDs []string `json:"negativeIds"`
}
type modelOutput struct {
	Labels []label `json:"labels"`
}
type label struct {
	PassageID    json.RawMessage `json:"passageId"`
	Label        string          `json:"label"`
	AnswerPoints []string        `json:"answerPoints"`
	Rationale    string          `json:"rationale"`
}
type decision struct {
	SchemaVersion string   `json:"schemaVersion"`
	QueryID       string   `json:"queryId"`
	PassageID     string   `json:"passageId"`
	Label         string   `json:"label"`
	AnswerPoints  []string `json:"answerPoints"`
	Rationale     string   `json:"rationale"`
	Reviewer      string   `json:"reviewer"`
	ReviewedAt    string   `json:"reviewedAt"`
}

func main() {
	planPath := flag.String("plan", "tmp/t2ranking-dev/hardset-plan.json", "hardset plan")
	queriesPath := flag.String("queries", "tmp/t2ranking-dev/queries.dev.tsv", "queries TSV")
	corpusPath := flag.String("corpus", "tmp/t2ranking-dev/hardset-corpus.tsv", "hardset corpus TSV")
	outputPath := flag.String("output", "tmp/t2ranking-dev/multi-positive-model-decisions.jsonl", "model decisions JSONL")
	rawDir := flag.String("raw-dir", "tmp/t2ranking-dev/multi-positive-model-raw", "raw model reviews directory")
	start := flag.Int("start", 3, "plan item index to start at")
	limit := flag.Int("limit", 0, "number of plan items to review; zero means all remaining")
	flag.Parse()
	items := mustPlan(*planPath)
	queries := mustTSV(*queriesPath)
	passages := mustTSV(*corpusPath)
	if *start < 0 || *start >= len(items) {
		fatalf("start %d outside plan", *start)
	}
	end := len(items)
	if *limit > 0 && *start+*limit < end {
		end = *start + *limit
	}
	if err := config.LoadConfig("configs"); err != nil {
		fatalf("load config: %v", err)
	}
	runtime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{AIRuntime: infraai.NewRuntime()})
	if err != nil {
		fatalf("build runtime: %v", err)
	}
	defer runtime.Close()
	chat, ok := runtime.LLMChat.(aichat.ContextAwareLLMService)
	if !ok {
		fatalf("chat service lacks context support")
	}
	if err := os.MkdirAll(*rawDir, 0o755); err != nil {
		fatalf("create raw dir: %v", err)
	}
	out, err := os.OpenFile(*outputPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		fatalf("open output: %v", err)
	}
	defer out.Close()
	encoder := json.NewEncoder(out)
	for i := *start; i < end; i++ {
		item := items[i]
		ids := append([]string{item.PositiveID}, item.NegativeIDs...)
		for _, passageID := range ids {
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Minute)
			response, err := chat.ChatWithRequestContext(ctx, convention.ChatRequest{Messages: []convention.ChatMessage{convention.SystemMessage(relevanceReviewSystemPrompt), convention.UserMessage(singlePrompt(queries[item.QueryID], passages[passageID]))}, Temperature: ptr(0.0), MaxTokens: intPtr(600), JSONMode: boolPtr(true)})
			cancel()
			if err != nil {
				fatalf("review query %s passage %s: %v", item.QueryID, passageID, err)
			}
			if err := os.WriteFile(filepath.Join(*rawDir, item.QueryID+"-"+passageID+".json"), []byte(response), 0o644); err != nil {
				fatalf("write raw response: %v", err)
			}
			var value label
			if err := json.Unmarshal([]byte(stripFence(response)), &value); err != nil || !validLabel(value.Label) {
				fatalf("parse query %s passage %s response: %v", item.QueryID, passageID, err)
			}
			if err := encoder.Encode(decision{SchemaVersion: "1", QueryID: item.QueryID, PassageID: passageID, Label: value.Label, AnswerPoints: value.AnswerPoints, Rationale: value.Rationale, Reviewer: "qwen3-32b-first-pass", ReviewedAt: time.Now().UTC().Format(time.RFC3339)}); err != nil {
				fatalf("write decision: %v", err)
			}
		}
		fmt.Fprintf(os.Stderr, "reviewed %d/%d query=%s\n", i-*start+1, end-*start, item.QueryID)
	}
}

func singlePrompt(query, passage string) string {
	if len([]rune(passage)) > 6000 {
		passage = string([]rune(passage)[:6000])
	}
	return singlePassageReviewInstruction + query + singlePassageContextHeader + passage
}

func reviewPrompt(item planItem, query string, passages map[string]string) string {
	var b strings.Builder
	b.WriteString(candidateReviewInstruction + query + "\n")
	ids := append([]string{item.PositiveID}, item.NegativeIDs...)
	for _, id := range ids {
		text := passages[id]
		if len([]rune(text)) > 4500 {
			text = string([]rune(text)[:4500])
		}
		fmt.Fprintf(&b, candidatePassageTemplate, id, text)
	}
	b.WriteString(requiredPassageIDsHeader + strings.Join(ids, ",") + "\n")
	return b.String()
}
func mustPlan(path string) []planItem {
	data, err := os.ReadFile(path)
	if err != nil {
		fatalf("read plan: %v", err)
	}
	var p plan
	if err = json.Unmarshal(data, &p); err != nil {
		fatalf("parse plan: %v", err)
	}
	return p.Items
}
func mustTSV(path string) map[string]string {
	f, err := os.Open(path)
	if err != nil {
		fatalf("open TSV: %v", err)
	}
	defer f.Close()
	out := map[string]string{}
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1024), 8*1024*1024)
	for s.Scan() {
		p := strings.SplitN(s.Text(), "\t", 2)
		if len(p) == 2 {
			out[p[0]] = p[1]
		}
	}
	if err := s.Err(); err != nil {
		fatalf("read TSV: %v", err)
	}
	return out
}
func stripFence(v string) string {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "```json")
	v = strings.TrimPrefix(v, "```")
	return strings.TrimSuffix(strings.TrimSpace(v), "```")
}
func decodePassageID(raw json.RawMessage) string {
	var value string
	if json.Unmarshal(raw, &value) == nil {
		return value
	}
	return strings.Trim(string(raw), "\\\"")
}
func validLabel(v string) bool {
	return v == "strong_positive" || v == "weak_positive" || v == "negative"
}
func ptr(v float64) *float64    { return &v }
func intPtr(v int) *int         { return &v }
func boolPtr(v bool) *bool      { return &v }
func fatalf(f string, a ...any) { fmt.Fprintf(os.Stderr, f+"\n", a...); os.Exit(1) }
