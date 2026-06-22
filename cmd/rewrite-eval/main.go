package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	ragrewrite "local/rag-project/internal/app/rag/core/rewrite"
	rageval "local/rag-project/internal/app/rag/evaluation"
	ragbootstrap "local/rag-project/internal/bootstrap/rag"
	"local/rag-project/internal/framework/config"
)

type sampleFile struct {
	Samples []rageval.RewriteSample `json:"samples"`
}

func main() {
	inputPath := flag.String("input", "", "path to rewrite evaluation samples JSON")
	execute := flag.Bool("execute", false, "call rewrite service for each sample (requires LLM API when enabled)")
	configDir := flag.String("config-dir", "configs", "config directory used with -execute")
	jsonOutput := flag.Bool("json", false, "print summary as JSON")
	outputPath := flag.String("output", "", "write summary to file instead of stdout")
	flag.Parse()

	if strings.TrimSpace(*inputPath) == "" {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/rewrite-eval -input <samples.json> [-execute] [-json] [-output result.json]")
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

	if *execute {
		if err := config.LoadConfig(*configDir); err != nil {
			fmt.Fprintf(os.Stderr, "load config failed: %v\n", err)
			os.Exit(1)
		}
		runtime, err := ragbootstrap.NewRuntime(context.Background(), ragbootstrap.RuntimeOptions{})
		if err != nil {
			fmt.Fprintf(os.Stderr, "build rag runtime failed: %v\n", err)
			os.Exit(1)
		}
		defer func() { _ = runtime.Close() }()
		if runtime.Rewrite == nil {
			fmt.Fprintln(os.Stderr, "rewrite service unavailable (check rag.query-rewrite.enabled in config)")
			os.Exit(1)
		}
		if err := executeSamples(context.Background(), runtime.Rewrite, samples); err != nil {
			fmt.Fprintf(os.Stderr, "execute rewrite samples failed: %v\n", err)
			os.Exit(1)
		}
	}

	summary, err := rageval.EvaluateRewriteSamples(samples)
	if err != nil {
		fmt.Fprintf(os.Stderr, "evaluate rewrite samples failed: %v\n", err)
		os.Exit(1)
	}

	if err := emitSummary(summary, *jsonOutput, strings.TrimSpace(*outputPath)); err != nil {
		fmt.Fprintf(os.Stderr, "emit summary failed: %v\n", err)
		os.Exit(1)
	}
}

func executeSamples(ctx context.Context, rewrite ragrewrite.Service, samples []rageval.RewriteSample) error {
	for i := range samples {
		if err := rageval.ExecuteRewriteSample(ctx, &samples[i], rewrite); err != nil {
			return err
		}
	}
	return nil
}

func loadSamples(path string) ([]rageval.RewriteSample, error) {
	data, err := rageval.LoadRawSampleFile(path)
	if err != nil {
		return nil, err
	}
	rawSamples, err := rageval.ExtractSampleArray(json.RawMessage(data))
	if err != nil {
		return nil, err
	}
	return rageval.ParseRewriteSamples(rawSamples)
}

func emitSummary(summary rageval.RewriteSummary, jsonOutput bool, outputPath string) error {
	var (
		data []byte
		err  error
	)
	if jsonOutput {
		var buf bytes.Buffer
		encoder := json.NewEncoder(&buf)
		encoder.SetIndent("", "  ")
		if err = encoder.Encode(summary); err != nil {
			return err
		}
		data = buf.Bytes()
	} else {
		data = []byte(renderSummaryText(summary))
	}

	if outputPath == "" {
		_, err = os.Stdout.Write(data)
		return err
	}
	if err := os.WriteFile(outputPath, data, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "summary written to %s\n", outputPath)
	return nil
}

func renderSummaryText(summary rageval.RewriteSummary) string {
	var buf bytes.Buffer
	o := summary.Overall
	fmt.Fprintf(&buf, "samples=%d pass=%.4f term_preservation=%.4f need_retrieval_acc=%.4f subquestion_ok=%.4f constraint_guard=%.4f\n",
		o.SampleCount, o.PassRate, o.TermPreservationRate, o.NeedRetrievalAccuracy, o.SubQuestionComplianceRate, o.ConstraintGuardPassRate)
	if len(summary.ByTag) > 0 {
		fmt.Fprintln(&buf)
		fmt.Fprintln(&buf, "by_tag:")
		for _, item := range summary.ByTag {
			fmt.Fprintf(&buf, "- %s samples=%d pass=%.4f term=%.4f need=%.4f\n",
				item.Tag, item.Metrics.SampleCount, item.Metrics.PassRate, item.Metrics.TermPreservationRate, item.Metrics.NeedRetrievalAccuracy)
		}
	}
	fmt.Fprintln(&buf)
	fmt.Fprintln(&buf, "samples_detail:")
	for _, sample := range summary.Samples {
		fmt.Fprintf(&buf, "- %s passed=%t rewritten=%q subs=%d needRetrieval=%t\n",
			sample.Name, sample.Checks.Passed, sample.RewrittenQuery, len(sample.SubQuestions), sample.NeedRetrieval)
		if !sample.Checks.Passed {
			if len(sample.Checks.MissingTerms) > 0 {
				fmt.Fprintf(&buf, "  missingTerms=%v\n", sample.Checks.MissingTerms)
			}
			if sample.Checks.NeedRetrievalEvaluated && !sample.Checks.NeedRetrievalMatch {
				fmt.Fprintf(&buf, "  needRetrieval mismatch\n")
			}
			if !sample.Checks.SubQuestionCountOK {
				fmt.Fprintf(&buf, "  subQuestionCount out of range\n")
			}
		}
	}
	return buf.String()
}
