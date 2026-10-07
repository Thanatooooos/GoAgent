// Command t2ranking-agent-failures classifies completed agent evaluation runs
// from their durable runtime journals without invoking a model.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	"local/rag-project/internal/framework/config"
)

type sampleFile struct {
	Samples []sample `json:"samples"`
}

type sample struct {
	Name        string   `json:"name"`
	Query       string   `json:"query"`
	ExpectedIDs []string `json:"expectedIds"`
}

type sessionRow struct {
	ID            string `gorm:"column:id"`
	UserMessageID string `gorm:"column:user_message_id"`
}

type journalRow struct {
	Sequence     int64  `gorm:"column:sequence"`
	EventType    string `gorm:"column:event_type"`
	ToolCallID   string `gorm:"column:tool_call_id"`
	ToolName     string `gorm:"column:tool_name"`
	Detail       string `gorm:"column:detail"`
	EvidenceJSON string `gorm:"column:evidence_json"`
}

type evidence struct {
	ID         string
	DocumentID string
}

type round struct {
	Query  string
	Rank   int
	Chunks []evidence
}

type item struct {
	Index       int
	Name        string
	Question    string
	ExpectedDoc string
	Rounds      []round
	AnyHit      bool
	Cited       bool
}

func main() {
	input := flag.String("input", "tmp/t2ranking-dev/eval-hardset-50-primary.json", "evaluation sample file")
	output := flag.String("output", "tmp/t2ranking-dev/agent-hardset-50-failures.md", "markdown output")
	createdAfter := flag.String("created-after", "", "include sessions created at or after this RFC3339 time")
	createdBefore := flag.String("created-before", "", "include sessions created before this RFC3339 time")
	flag.Parse()
	samples, err := loadSamples(*input)
	if err != nil {
		fatalf("load samples: %v", err)
	}
	if err := config.LoadConfig("configs"); err != nil {
		fatalf("load config: %v", err)
	}
	db, err := postgresrepo.NewGormDB(config.Get().Spring.Datasource)
	if err != nil {
		fatalf("open database: %v", err)
	}
	defer func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	}()

	query := db.Table("t_runtime_session").Select("id, user_message_id").
		Where("user_id = ? AND user_message_id LIKE ?", "t2ranking-agent-eval", "t2agent%")
	if *createdAfter != "" {
		value, err := time.Parse(time.RFC3339, *createdAfter)
		if err != nil {
			fatalf("parse created-after: %v", err)
		}
		query = query.Where("create_time >= ?", value)
	}
	if *createdBefore != "" {
		value, err := time.Parse(time.RFC3339, *createdBefore)
		if err != nil {
			fatalf("parse created-before: %v", err)
		}
		query = query.Where("create_time < ?", value)
	}
	var sessions []sessionRow
	if err := query.Order("create_time DESC").Limit(len(samples)).Find(&sessions).Error; err != nil {
		fatalf("list sessions: %v", err)
	}
	byIndex := make(map[int]string, len(sessions))
	for _, session := range sessions {
		if index, ok := sampleIndex(session.UserMessageID); ok {
			byIndex[index] = session.ID
		}
	}
	items := make([]item, 0, len(samples))
	for index, sample := range samples {
		if len(sample.ExpectedIDs) == 0 || byIndex[index] == "" {
			continue
		}
		items = append(items, inspectSession(db, byIndex[index], index, sample))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Index < items[j].Index })
	if err := os.WriteFile(*output, []byte(render(items)), 0o644); err != nil {
		fatalf("write report: %v", err)
	}
	fmt.Printf("samples=%d failures=%d output=%s\n", len(items), countFailures(items), *output)
}

func inspectSession(db *gorm.DB, sessionID string, index int, sample sample) item {
	result := item{Index: index, Name: sample.Name, Question: sample.Query, ExpectedDoc: sample.ExpectedIDs[0]}
	var rows []journalRow
	if err := db.Table("t_runtime_journal").Select("sequence, event_type, tool_call_id, tool_name, detail, evidence_json").Where("runtime_session_id = ?", sessionID).Order("sequence ASC").Find(&rows).Error; err != nil {
		return result
	}
	byCallID := map[string]int{}
	answer := ""
	for _, row := range rows {
		if row.EventType == "tool_pending" && row.ToolName == "retrieve_knowledge" {
			byCallID[row.ToolCallID] = len(result.Rounds)
			result.Rounds = append(result.Rounds, round{Query: queryText(row.Detail)})
			continue
		}
		if row.EventType == "tool_settled" {
			index, ok := byCallID[row.ToolCallID]
			if !ok {
				continue
			}
			var refs []evidence
			if json.Unmarshal([]byte(row.EvidenceJSON), &refs) != nil {
				continue
			}
			result.Rounds[index].Chunks = refs
			for rank, ref := range refs {
				if ref.DocumentID == result.ExpectedDoc {
					result.Rounds[index].Rank = rank + 1
					result.AnyHit = true
					break
				}
			}
		}
		if row.EventType == "answer_final" {
			answer = row.Detail
		}
	}
	for _, round := range result.Rounds {
		for _, ref := range round.Chunks {
			if ref.DocumentID == result.ExpectedDoc && strings.Contains(answer, `chunk_id="`+ref.ID+`"`) {
				result.Cited = true
			}
		}
	}
	return result
}

func queryText(raw string) string {
	var args struct {
		Query string `json:"query"`
	}
	if json.Unmarshal([]byte(raw), &args) == nil && strings.TrimSpace(args.Query) != "" {
		return args.Query
	}
	return raw
}

func sampleIndex(value string) (int, bool) {
	part := value[strings.LastIndex(value, "-")+1:]
	index, err := strconv.Atoi(part)
	return index, err == nil
}

func countFailures(items []item) int {
	count := 0
	for _, item := range items {
		if !item.Cited {
			count++
		}
	}
	return count
}

func render(items []item) string {
	var b strings.Builder
	noRecall, lostEvidence := 0, 0
	for _, item := range items {
		if !item.AnyHit {
			noRecall++
		} else if !item.Cited {
			lostEvidence++
		}
	}
	fmt.Fprintf(&b, "# Agent hardset 50 failures\n\n- samples: %d\n- no target in any retrieval round: %d\n- target retrieved but not cited in final answer: %d\n\n", len(items), noRecall, lostEvidence)
	for _, item := range items {
		if item.Cited {
			continue
		}
		kind := "all rounds missed target"
		if item.AnyHit {
			kind = "target retrieved but not cited"
		}
		fmt.Fprintf(&b, "## %d. %s — %s\n\nQuestion: %s\n\n", item.Index+1, item.Name, kind, item.Question)
		for number, round := range item.Rounds {
			fmt.Fprintf(&b, "- round %d: rank=%d, query=%s\n", number+1, round.Rank, round.Query)
		}
		b.WriteString("\n")
	}
	return b.String()
}

func loadSamples(path string) ([]sample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var wrapped sampleFile
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, err
	}
	return wrapped.Samples, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
