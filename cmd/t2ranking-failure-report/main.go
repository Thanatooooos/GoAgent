package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	rageval "local/rag-project/internal/app/rag/evaluation"
)

func main() {
	input := flag.String("input", "tmp/t2ranking-dev/eval-hardset-50-primary-result.json", "evaluation result JSON")
	output := flag.String("output", "tmp/t2ranking-dev/hardset-top1-failures.md", "Markdown report output")
	flag.Parse()
	data, err := os.ReadFile(*input)
	if err != nil {
		fatal(err)
	}
	var summary rageval.Summary
	if err := json.Unmarshal(data, &summary); err != nil {
		fatal(err)
	}
	failures := make([]rageval.SampleResult, 0)
	for _, sample := range summary.Samples {
		if sample.FirstRelevantRank != 1 {
			failures = append(failures, sample)
		}
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].FirstRelevantRank < failures[j].FirstRelevantRank })
	var out strings.Builder
	fmt.Fprintf(&out, "# Hardset Top1 failures\n\ncount: %d\n", len(failures))
	for _, sample := range failures {
		fmt.Fprintf(&out, "\n## %s\n\nquery: %s\n\nfirst relevant rank: %d\n\nexpected document: %s\n", sample.Name, sample.Query, sample.FirstRelevantRank, strings.Join(sample.ExpectedIDs, ", "))
		for i, hit := range sample.Retrieved {
			if i >= 3 {
				break
			}
			fmt.Fprintf(&out, "\n%d. document=%s score=%.4f file=%v\n", i+1, hit.DocumentID, hit.Score, hit.Metadata["document_name"])
		}
	}
	if err := os.WriteFile(*output, []byte(out.String()), 0o644); err != nil {
		fatal(err)
	}
	fmt.Printf("top1_failures=%d output=%s\n", len(failures), *output)
}
func fatal(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
