package main

import (
	"encoding/json"
	"fmt"
	"os"
)

func main() {
	raw, err := os.ReadFile("testdata/evals/rewrite/v2_24_soft_gate.json")
	if err != nil {
		panic(err)
	}
	idx := -1
	marker := []byte(`{"suite"`)
	for i := len(raw) - len(marker); i >= 0; i-- {
		if string(raw[i:i+len(marker)]) == string(marker) {
			idx = i
			break
		}
	}
	if idx < 0 {
		panic("suite json not found")
	}
	var data struct {
		RunMetadata struct {
			RunAt string `json:"run_at"`
		} `json:"run_metadata"`
		Samples []struct {
			Name           string         `json:"name"`
			Passed         bool           `json:"passed"`
			FailureReasons []string       `json:"failure_reasons"`
			Scores         map[string]any `json:"scores"`
			RuleChecks     map[string]any `json:"rule_checks"`
		} `json:"samples"`
		Aggregate struct {
			PassRate float64 `json:"pass_rate"`
			Metrics  map[string]any `json:"metrics"`
		} `json:"aggregate"`
	}
	if err := json.Unmarshal(raw[idx:], &data); err != nil {
		panic(err)
	}
	passed := 0
	var ruleFailJudgePass, ruleFailSemanticPass, ruleFailBoth []string
	for _, s := range data.Samples {
		if s.Passed {
			passed++
			continue
		}
		judge, _ := s.Scores["judge_score"].(float64)
		semantic, _ := s.Scores["semantic_score"].(float64)
		if judge >= 0.65 {
			ruleFailJudgePass = append(ruleFailJudgePass, fmt.Sprintf("%s (judge=%.2f semantic=%.2f)", s.Name, judge, semantic))
		}
		if semantic >= 0.65 {
			ruleFailSemanticPass = append(ruleFailSemanticPass, s.Name)
		}
		if judge >= 0.65 && semantic >= 0.65 {
			ruleFailBoth = append(ruleFailBoth, s.Name)
		}
	}
	fmt.Println("RUN", data.RunMetadata.RunAt)
	fmt.Printf("RULE_PASS %d/%d (%.1f%%)\n", passed, len(data.Samples), data.Aggregate.PassRate*100)
	fmt.Printf("MRR uplift %v\n", data.Aggregate.Metrics["mrr_uplift"])
	fmt.Printf("avg_semantic %v\n", data.Aggregate.Metrics["avg_semantic_score"])
	fmt.Printf("semantic_judge_overrides %v\n", data.Aggregate.Metrics["semantic_judge_override_count"])

	byPath := map[string]int{}
	ruleOnlyPass := 0
	for _, s := range data.Samples {
		if path, ok := s.Scores["pass_path"].(string); ok && path != "" {
			byPath[path]++
		}
		if s.Passed {
			if rulePassed, ok := s.RuleChecks["rule_passed"].(bool); ok && rulePassed {
				ruleOnlyPass++
			}
		}
	}
	fmt.Printf("\nPASS_PATH %v\n", byPath)
	fmt.Printf("passed_with_rule_green %d/%d\n", ruleOnlyPass, passed)
	fmt.Printf("\nRULE_FAIL but JUDGE>=0.65: %d\n", len(ruleFailJudgePass))
	for _, line := range ruleFailJudgePass {
		fmt.Println(" ", line)
	}
	fmt.Printf("\nRULE_FAIL but SEMANTIC>=0.65: %d\n", len(ruleFailSemanticPass))
	for _, name := range ruleFailSemanticPass {
		fmt.Println(" ", name)
	}
	fmt.Printf("\nRULE_FAIL but BOTH>=0.65: %d\n", len(ruleFailBoth))
	for _, name := range ruleFailBoth {
		fmt.Println(" ", name)
	}
	fmt.Println("\nRULE_FAILURES")
	for _, s := range data.Samples {
		if !s.Passed {
			fmt.Printf("  %s: %v\n", s.Name, s.FailureReasons)
		}
	}
}
