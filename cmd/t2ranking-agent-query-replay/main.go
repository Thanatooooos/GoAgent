// Command t2ranking-agent-query-replay exports the actual knowledge queries
// from the latest agent hardset run as deterministic retrieval-eval samples.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	postgresrepo "local/rag-project/internal/adapter/repository/postgres"
	rageval "local/rag-project/internal/app/rag/evaluation"
	"local/rag-project/internal/framework/config"
)

type sourceFile struct {
	Samples []sourceSample `json:"samples"`
}
type sourceSample struct {
	Name             string   `json:"name"`
	ExpectedIDs      []string `json:"expectedIds"`
	KnowledgeBaseIDs []string `json:"knowledgeBaseIds"`
}
type sessionRow struct {
	ID            string `gorm:"column:id"`
	UserMessageID string `gorm:"column:user_message_id"`
}
type journalRow struct {
	EventType string `gorm:"column:event_type"`
	ToolName  string `gorm:"column:tool_name"`
	Detail    string `gorm:"column:detail"`
}

func main() {
	input := flag.String("input", "tmp/t2ranking-dev/eval-hardset-50-primary.json", "source evaluation samples")
	output := flag.String("output", "tmp/t2ranking-dev/agent-hardset-50-query-replay.json", "replay samples output")
	flag.Parse()
	sources, err := loadSources(*input)
	if err != nil {
		fatalf("load source samples: %v", err)
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
	var sessions []sessionRow
	if err := db.Table("t_runtime_session").Select("id, user_message_id").Where("user_id = ? AND user_message_id LIKE ?", "t2ranking-agent-eval", "t2agent%").Order("create_time DESC").Limit(len(sources)).Find(&sessions).Error; err != nil {
		fatalf("list sessions: %v", err)
	}
	byIndex := map[int]string{}
	for _, session := range sessions {
		if index, ok := sampleIndex(session.UserMessageID); ok {
			byIndex[index] = session.ID
		}
	}
	result := make([]rageval.Sample, 0)
	for index, source := range sources {
		if byIndex[index] == "" || len(source.ExpectedIDs) == 0 {
			continue
		}
		var entries []journalRow
		if err := db.Table("t_runtime_journal").Select("event_type, tool_name, detail").Where("runtime_session_id = ?", byIndex[index]).Order("sequence ASC").Find(&entries).Error; err != nil {
			fatalf("load journal %d: %v", index, err)
		}
		round := 0
		for _, entry := range entries {
			if entry.EventType != "tool_pending" || entry.ToolName != "retrieve_knowledge" {
				continue
			}
			var args struct {
				Query string `json:"query"`
				TopK  int    `json:"top_k"`
			}
			if err := json.Unmarshal([]byte(entry.Detail), &args); err != nil || strings.TrimSpace(args.Query) == "" {
				fatalf("parse query for sample %d round %d: %v", index, round+1, err)
			}
			if args.TopK < 1 {
				args.TopK = 5
			}
			round++
			result = append(result, rageval.Sample{Name: fmt.Sprintf("agent_%02d_round_%d", index+1, round), Query: args.Query, Tags: []string{"agent_query_replay", fmt.Sprintf("agent_round_%d", round)}, Target: rageval.TargetDocument, ExpectedIDs: []string{source.ExpectedIDs[0]}, ExpectedRelevance: map[string]int{source.ExpectedIDs[0]: 3}, KnowledgeBaseIDs: source.KnowledgeBaseIDs, TopK: args.TopK})
		}
	}
	data, err := json.MarshalIndent(struct {
		Samples []rageval.Sample `json:"samples"`
	}{result}, "", "  ")
	if err != nil {
		fatalf("marshal replay samples: %v", err)
	}
	if err := os.WriteFile(*output, data, 0o644); err != nil {
		fatalf("write replay samples: %v", err)
	}
	fmt.Printf("samples=%d output=%s\n", len(result), *output)
}

func loadSources(path string) ([]sourceSample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var file sourceFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, err
	}
	return file.Samples, nil
}
func sampleIndex(value string) (int, bool) {
	index, err := strconv.Atoi(value[strings.LastIndex(value, "-")+1:])
	return index, err == nil
}
func fatalf(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...); os.Exit(1) }
