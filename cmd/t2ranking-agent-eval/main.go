// Command t2ranking-agent-eval measures the production conversation runtime's
// actual tool-selection and knowledge-retrieval behavior on fixed samples.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	rageval "local/rag-project/internal/app/rag/evaluation"
	conversationruntime "local/rag-project/internal/app/runtime"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
	infraai "local/rag-project/internal/infra-ai"
)

type sampleFile struct {
	Samples []rageval.Sample `json:"samples"`
}

type assistantTurn struct {
	ToolCalls []struct {
		ID           string          `json:"ID"`
		CapabilityID string          `json:"CapabilityID"`
		Arguments    json.RawMessage `json:"Arguments"`
	} `json:"tool_calls"`
}

type resultItem struct {
	Name             string                            `json:"name"`
	Question         string                            `json:"question"`
	ExpectedDocument string                            `json:"expectedDocument"`
	Status           string                            `json:"status"`
	Error            string                            `json:"error,omitempty"`
	ToolCalls        []toolCall                        `json:"toolCalls"`
	RetrievalRounds  []retrievalRound                  `json:"retrievalRounds"`
	Evidence         []conversationruntime.EvidenceRef `json:"evidence"`
	ExpectedDocRank  int                               `json:"expectedDocumentRank,omitempty"`
	KnowledgeCalled  bool                              `json:"knowledgeCalled"`
	AnyRoundHit      bool                              `json:"anyRoundHit"`
	FinalRoundHit    bool                              `json:"finalRoundHit"`
	FinalEvidenceHit bool                              `json:"finalAnswerUsesExpectedEvidence"`
	FinalAnswer      string                            `json:"finalAnswer,omitempty"`
	RuntimeSessionID string                            `json:"runtimeSessionId,omitempty"`
}

type toolCall struct {
	ID           string `json:"id,omitempty"`
	CapabilityID string `json:"capabilityId"`
	Arguments    string `json:"arguments"`
}

type retrievalRound struct {
	Query                string                            `json:"query"`
	TopK                 int                               `json:"topK,omitempty"`
	Evidence             []conversationruntime.EvidenceRef `json:"evidence"`
	ExpectedDocumentRank int                               `json:"expectedDocumentRank,omitempty"`
}

type report struct {
	GeneratedAt           time.Time    `json:"generatedAt"`
	SampleCount           int          `json:"sampleCount"`
	CompletedCount        int          `json:"completedCount"`
	KnowledgeCallRate     float64      `json:"knowledgeCallRate"`
	AnyRoundHitRate       float64      `json:"anyRoundHitRate"`
	FinalRoundHitRate     float64      `json:"finalRoundHitRate"`
	FinalEvidenceHitRate  float64      `json:"finalAnswerEvidenceHitRate"`
	AverageKnowledgeCalls float64      `json:"averageKnowledgeCalls"`
	Items                 []resultItem `json:"items"`
}

type collectingSink struct {
	mu      sync.Mutex
	entries []conversationruntime.JournalEntry
}

type journalRow struct {
	EventType    string `gorm:"column:event_type"`
	ToolCallID   string `gorm:"column:tool_call_id"`
	ToolName     string `gorm:"column:tool_name"`
	Detail       string `gorm:"column:detail"`
	EvidenceJSON string `gorm:"column:evidence_json"`
}

func (s *collectingSink) Append(_ context.Context, entry conversationruntime.JournalEntry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
	return nil
}

func main() {
	input := flag.String("input", "tmp/t2ranking-dev/eval-hardset-50-primary.json", "evaluation samples JSON")
	output := flag.String("output", "tmp/t2ranking-dev/agent-pilot.json", "agent evaluation JSON output")
	limit := flag.Int("limit", 1, "number of samples to run; zero means all")
	configDir := flag.String("config-dir", "configs", "config directory")
	searchMode := flag.String("search-mode", "", "server-controlled retrieval mode: semantic, hybrid, keyword, or auto")
	flag.Parse()

	samples, err := loadSamples(*input)
	if err != nil {
		fatalf("load samples: %v", err)
	}
	if *limit > 0 && *limit < len(samples) {
		samples = samples[:*limit]
	}
	if len(samples) == 0 {
		fatalf("no samples")
	}
	if err := config.LoadConfig(*configDir); err != nil {
		fatalf("load config: %v", err)
	}
	runtime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{AIRuntime: infraai.NewRuntime()})
	if err != nil {
		fatalf("build production runtime: %v", err)
	}
	defer func() { _ = runtime.Close() }()

	report := report{GeneratedAt: time.Now().UTC(), SampleCount: len(samples), Items: make([]resultItem, 0, len(samples))}
	for index, sample := range samples {
		item := runSample(context.Background(), runtime.ConversationRuntime, runtime.DB, sample, index, *searchMode)
		report.Items = append(report.Items, item)
		summarizeReport(&report)
		fmt.Fprintf(os.Stderr, "%d/%d %s status=%s tools=%d expectedRank=%d\n", index+1, len(samples), item.Name, item.Status, len(item.ToolCalls), item.ExpectedDocRank)
		if err := writeReport(*output, report); err != nil {
			fatalf("checkpoint report: %v", err)
		}
	}
	if err := writeReport(*output, report); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("samples=%d completed=%d knowledge_call_rate=%.2f any_round_hit_rate=%.2f final_evidence_hit_rate=%.2f output=%s\n", report.SampleCount, report.CompletedCount, report.KnowledgeCallRate, report.AnyRoundHitRate, report.FinalEvidenceHitRate, *output)
}

func runSample(ctx context.Context, runtime conversationruntime.ConversationRuntime, db *gorm.DB, sample rageval.Sample, index int, searchMode string) resultItem {
	item := resultItem{Name: sample.Name, Question: sample.Query, ExpectedDocument: first(sample.ExpectedIDs)}
	sink := &collectingSink{}
	stamp := fmt.Sprintf("t2agent-%d-%d", time.Now().UnixNano(), index)
	result, err := runtime.Run(ctx, conversationruntime.RunRequest{
		ConversationID:   stamp,
		UserID:           "t2ranking-agent-eval",
		UserMessageID:    stamp,
		Question:         sample.Query,
		KnowledgeBaseIDs: append([]string(nil), sample.KnowledgeBaseIDs...),
		TraceID:          stamp,
		Policy: conversationruntime.Policy{
			AllowKnowledgeRetrieval: true,
			RetrieveSearchMode:      strings.TrimSpace(searchMode),
			MaxTurns:                3,
			MaxToolCalls:            5,
		},
	}, sink)
	item.Status, item.RuntimeSessionID, item.FinalAnswer = result.Status, result.RuntimeSessionID, result.AssistantContent
	item.Evidence = append([]conversationruntime.EvidenceRef(nil), result.Evidence...)
	if err != nil {
		item.Error = err.Error()
	}
	item.ToolCalls = extractToolCalls(sink.entries)
	if result.RuntimeSessionID != "" && db != nil {
		calls, evidence, rounds, journalErr := loadJournalFacts(ctx, db, result.RuntimeSessionID, sample.Target, sample.ExpectedIDs)
		if journalErr != nil && item.Error == "" {
			item.Error = "load runtime journal: " + journalErr.Error()
		}
		if len(calls) > 0 {
			item.ToolCalls = calls
		}
		if len(evidence) > 0 {
			item.Evidence = evidence
		}
		item.RetrievalRounds = rounds
	}
	for _, call := range item.ToolCalls {
		if call.CapabilityID == "retrieve_knowledge" {
			item.KnowledgeCalled = true
		}
	}
	for rank, evidence := range item.Evidence {
		if matchesExpectedEvidence(sample.Target, sample.ExpectedIDs, evidence) {
			item.ExpectedDocRank = rank + 1
			break
		}
	}
	for _, round := range item.RetrievalRounds {
		if round.ExpectedDocumentRank > 0 {
			item.AnyRoundHit = true
		}
	}
	if len(item.RetrievalRounds) > 0 {
		item.FinalRoundHit = item.RetrievalRounds[len(item.RetrievalRounds)-1].ExpectedDocumentRank > 0
	}
	item.FinalEvidenceHit = answerUsesExpectedEvidence(item.FinalAnswer, sample.Target, sample.ExpectedIDs, item.Evidence)
	return item
}

func loadJournalFacts(ctx context.Context, db *gorm.DB, runtimeSessionID string, target rageval.Target, expectedIDs []string) ([]toolCall, []conversationruntime.EvidenceRef, []retrievalRound, error) {
	var rows []journalRow
	if err := db.WithContext(ctx).Table("t_runtime_journal").
		Select("event_type, tool_call_id, tool_name, detail, evidence_json").
		Where("runtime_session_id = ?", runtimeSessionID).
		Order("sequence ASC").Find(&rows).Error; err != nil {
		return nil, nil, nil, err
	}
	calls := []toolCall{}
	evidence := []conversationruntime.EvidenceRef{}
	rounds := []retrievalRound{}
	retrievalByCallID := map[string]int{}
	for _, row := range rows {
		if row.EventType == conversationruntime.EventAssistantTurn {
			var turn assistantTurn
			if err := json.Unmarshal([]byte(row.Detail), &turn); err == nil {
				for _, call := range turn.ToolCalls {
					calls = append(calls, toolCall{ID: call.ID, CapabilityID: call.CapabilityID, Arguments: string(call.Arguments)})
					if call.CapabilityID == "retrieve_knowledge" {
						round := retrievalRound{}
						var args struct {
							Query string `json:"query"`
							TopK  int    `json:"top_k"`
						}
						_ = json.Unmarshal(call.Arguments, &args)
						round.Query, round.TopK = args.Query, args.TopK
						retrievalByCallID[call.ID] = len(rounds)
						rounds = append(rounds, round)
					}
				}
			}
		}
		if row.EventType == conversationruntime.EventToolSettled && row.EvidenceJSON != "" {
			var refs []conversationruntime.EvidenceRef
			if err := json.Unmarshal([]byte(row.EvidenceJSON), &refs); err == nil {
				evidence = append(evidence, refs...)
				if index, ok := retrievalByCallID[row.ToolCallID]; ok {
					rounds[index].Evidence = refs
					for rank, ref := range refs {
						if matchesExpectedEvidence(target, expectedIDs, ref) {
							rounds[index].ExpectedDocumentRank = rank + 1
							break
						}
					}
				}
			}
		}
	}
	return calls, evidence, rounds, nil
}

func extractToolCalls(entries []conversationruntime.JournalEntry) []toolCall {
	result := []toolCall{}
	for _, entry := range entries {
		if entry.EventType != conversationruntime.EventAssistantTurn {
			continue
		}
		var turn assistantTurn
		if err := json.Unmarshal([]byte(entry.Detail), &turn); err != nil {
			continue
		}
		for _, call := range turn.ToolCalls {
			result = append(result, toolCall{ID: call.ID, CapabilityID: call.CapabilityID, Arguments: string(call.Arguments)})
		}
	}
	return result
}

func matchesExpectedEvidence(target rageval.Target, expectedIDs []string, ref conversationruntime.EvidenceRef) bool {
	for _, expectedID := range expectedIDs {
		expectedID = strings.TrimSpace(expectedID)
		if expectedID == "" {
			continue
		}
		switch target {
		case rageval.TargetDocument:
			if ref.DocumentID == expectedID {
				return true
			}
		default:
			if ref.ID == expectedID {
				return true
			}
		}
	}
	return false
}

func answerUsesExpectedEvidence(answer string, target rageval.Target, expectedIDs []string, evidence []conversationruntime.EvidenceRef) bool {
	if strings.TrimSpace(answer) == "" || len(expectedIDs) == 0 {
		return false
	}
	for _, ref := range evidence {
		if matchesExpectedEvidence(target, expectedIDs, ref) && strings.Contains(answer, `chunk_id="`+ref.ID+`"`) {
			return true
		}
	}
	return false
}

func writeReport(path string, report report) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func summarizeReport(report *report) {
	if report == nil {
		return
	}
	report.CompletedCount = 0
	report.KnowledgeCallRate = 0
	report.AnyRoundHitRate = 0
	report.FinalRoundHitRate = 0
	report.FinalEvidenceHitRate = 0
	report.AverageKnowledgeCalls = 0
	for _, item := range report.Items {
		if item.Status == conversationruntime.StatusCompleted {
			report.CompletedCount++
		}
		if item.KnowledgeCalled {
			report.KnowledgeCallRate++
		}
		if item.AnyRoundHit {
			report.AnyRoundHitRate++
		}
		if item.FinalRoundHit {
			report.FinalRoundHitRate++
		}
		if item.FinalEvidenceHit {
			report.FinalEvidenceHitRate++
		}
		report.AverageKnowledgeCalls += float64(len(item.RetrievalRounds))
	}
	if len(report.Items) == 0 {
		return
	}
	total := float64(len(report.Items))
	report.KnowledgeCallRate /= total
	report.AnyRoundHitRate /= total
	report.FinalRoundHitRate /= total
	report.FinalEvidenceHitRate /= total
	report.AverageKnowledgeCalls /= total
}

func loadSamples(path string) ([]rageval.Sample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrapped sampleFile
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Samples) > 0 {
		return wrapped.Samples, nil
	}
	var plain []rageval.Sample
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, err
	}
	return plain, nil
}

func first(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
