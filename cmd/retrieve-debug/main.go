package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
)

type sampleFile struct {
	Samples []searchModeSample `json:"samples"`
}

type searchModeSample struct {
	Name         string `json:"name"`
	Query        string `json:"query"`
	ExpectedMode string `json:"expectedMode"`
}

type replayResult struct {
	Name         string   `json:"name"`
	Query        string   `json:"query"`
	ExpectedMode string   `json:"expectedMode,omitempty"`
	ResolvedMode string   `json:"resolvedMode"`
	Reason       string   `json:"reason"`
	Signals      []string `json:"signals"`
	Status       string   `json:"status"`
}

type replaySummary struct {
	Total   int            `json:"total"`
	Fail    int            `json:"fail"`
	Pass    int            `json:"pass"`
	Results []replayResult `json:"results"`
}

func main() {
	inputPath := flag.String("input", "", "path to a JSON file containing search mode samples")
	jsonOutput := flag.Bool("json", false, "print replay results as JSON")
	flag.Parse()

	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/retrieve-debug -input <samples.json>")
		os.Exit(1)
	}

	samples, err := loadSamples(*inputPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load samples failed: %v\n", err)
		os.Exit(1)
	}
	if len(samples) == 0 {
		fmt.Fprintln(os.Stderr, "no samples found")
		os.Exit(1)
	}

	failures := 0
	results := make([]replayResult, 0, len(samples))
	for index, sample := range samples {
		status := "INFO"
		decision := ragretrieve.AnalyzeSearchMode(ragretrieve.Request{
			Query:      strings.TrimSpace(sample.Query),
			SearchMode: ragretrieve.SearchModeAuto,
		})

		expectedMode := normalizeMode(sample.ExpectedMode)
		if expectedMode != "" {
			if decision.ResolvedMode == expectedMode {
				status = "PASS"
			} else {
				status = "FAIL"
				failures++
			}
		}

		name := strings.TrimSpace(sample.Name)
		if name == "" {
			name = fmt.Sprintf("sample-%02d", index+1)
		}

		result := replayResult{
			Name:         name,
			Query:        strings.TrimSpace(sample.Query),
			ExpectedMode: expectedMode,
			ResolvedMode: decision.ResolvedMode,
			Reason:       decision.Reason,
			Signals:      append([]string(nil), decision.Signals...),
			Status:       status,
		}
		results = append(results, result)

		if !*jsonOutput {
			fmt.Printf("[%s] %s\n", status, name)
			fmt.Printf("query=%s\n", result.Query)
			if expectedMode != "" {
				fmt.Printf("expectedMode=%s\n", expectedMode)
			}
			fmt.Printf("resolvedMode=%s\n", decision.ResolvedMode)
			fmt.Printf("reason=%s\n", decision.Reason)
			fmt.Printf("signals=%s\n", strings.Join(decision.Signals, ", "))
			fmt.Println()
		}
	}

	summary := replaySummary{
		Total:   len(samples),
		Fail:    failures,
		Pass:    len(samples) - failures,
		Results: results,
	}
	if *jsonOutput {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(summary); err != nil {
			fmt.Fprintf(os.Stderr, "encode replay result failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		fmt.Printf("summary: total=%d fail=%d pass=%d\n", summary.Total, summary.Fail, summary.Pass)
	}
	if failures > 0 {
		os.Exit(2)
	}
}

func loadSamples(path string) ([]searchModeSample, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var wrapped sampleFile
	if err := json.Unmarshal(data, &wrapped); err == nil && len(wrapped.Samples) > 0 {
		return wrapped.Samples, nil
	}

	var plain []searchModeSample
	if err := json.Unmarshal(data, &plain); err != nil {
		return nil, err
	}
	return plain, nil
}

func normalizeMode(mode string) string {
	mode = strings.TrimSpace(strings.ToLower(mode))
	switch mode {
	case ragretrieve.SearchModeSemantic, ragretrieve.SearchModeKeyword, ragretrieve.SearchModeHybrid:
		return mode
	default:
		return ""
	}
}
