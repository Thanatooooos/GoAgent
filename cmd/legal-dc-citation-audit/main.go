// Command legal-dc-citation-audit turns failed agent citations into a compact
// human-review record. It only reads prior evaluation artifacts.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

type samplesFile struct {
	Samples []sample `json:"samples"`
}

type sample struct {
	Name        string   `json:"name"`
	Query       string   `json:"query"`
	ExpectedIDs []string `json:"expectedIds"`
}

type agentReport struct {
	Items []agentItem `json:"items"`
}

type agentItem struct {
	Name       string `json:"name"`
	Question   string `json:"question"`
	AnyRound   bool   `json:"anyRoundHit"`
	FinalRound bool   `json:"finalRoundHit"`
	Cited      bool   `json:"finalAnswerUsesExpectedEvidence"`
	Answer     string `json:"finalAnswer"`
}

var chunkIDPattern = regexp.MustCompile(`chunk_id="([^"]+)"`)

func main() {
	samplesPath := flag.String("samples", "tmp/legal-dc/pilot-10-retrieval-samples.json", "retrieval evaluation samples")
	reportPath := flag.String("report", "tmp/legal-dc/pilot-10-agent-hybrid-w025.json", "agent evaluation report")
	outputPath := flag.String("output", "tmp/legal-dc/pilot-10-agent-citation-audit.md", "markdown audit output")
	flag.Parse()

	var samples samplesFile
	readJSON(*samplesPath, &samples)
	byName := make(map[string]sample, len(samples.Samples))
	for _, item := range samples.Samples {
		byName[item.Name] = item
	}
	var report agentReport
	readJSON(*reportPath, &report)

	failures := make([]agentItem, 0)
	citationPasses := 0
	for _, item := range report.Items {
		sample, ok := byName[item.Name]
		if !ok {
			fatal("missing sample " + item.Name)
		}
		if answerCitesAnyExpected(item.Answer, sample.ExpectedIDs) {
			citationPasses++
			continue
		}
		if !item.Cited {
			failures = append(failures, item)
			continue
		}
		fatal("report citation result conflicts with extracted citations for " + item.Name)
	}
	sort.Slice(failures, func(i, j int) bool { return failures[i].Name < failures[j].Name })

	var b strings.Builder
	fmt.Fprintf(&b, "# Legal-DC Agent Citation Audit\n\n")
	fmt.Fprintf(&b, "- Input report: `%s`\n- Citation passes after current manual labels: %d/%d\n- Failed strict citations: %d\n- Review rule: mark **add positive** only when the actual cited child directly supports the answer; otherwise mark **citation error**.\n\n", *reportPath, citationPasses, len(report.Items), len(failures))
	for _, item := range failures {
		sample, ok := byName[item.Name]
		if !ok {
			fatal("missing sample " + item.Name)
		}
		cited := unique(chunkIDPattern.FindAllStringSubmatch(item.Answer, -1))
		fmt.Fprintf(&b, "## %s\n\n", item.Name)
		fmt.Fprintf(&b, "- Question: %s\n- Manual positive child IDs: `%s`\n- Final-answer cited child IDs: `%s`\n- Any-round / final-round positive retrieved: %t / %t\n- Reviewer decision: `pending`\n- Reviewer rationale: `pending`\n\n", sample.Query, strings.Join(sample.ExpectedIDs, "`, `"), strings.Join(cited, "`, `"), item.AnyRound, item.FinalRound)
		fmt.Fprintf(&b, "### Final answer\n\n%s\n\n", item.Answer)
	}
	if err := os.WriteFile(*outputPath, []byte(b.String()), 0o644); err != nil {
		fatal(err.Error())
	}
	fmt.Printf("audit_cases=%d output=%s\n", len(failures), *outputPath)
}

func unique(matches [][]string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 || seen[match[1]] {
			continue
		}
		seen[match[1]] = true
		result = append(result, match[1])
	}
	return result
}

func answerCitesAnyExpected(answer string, expectedIDs []string) bool {
	expected := make(map[string]bool, len(expectedIDs))
	for _, id := range expectedIDs {
		expected[id] = true
	}
	for _, id := range unique(chunkIDPattern.FindAllStringSubmatch(answer, -1)) {
		if expected[id] {
			return true
		}
	}
	return false
}

func readJSON(path string, target any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fatal(err.Error())
	}
	if err := json.Unmarshal(data, target); err != nil {
		fatal(fmt.Sprintf("parse %s: %v", path, err))
	}
}

func fatal(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
