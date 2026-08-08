package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
)

type failingDocumentEnricher struct{}

type promptCompleterStub struct{ prompts []string }

func (s *promptCompleterStub) Chat(prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	return "Question one?\nQuestion two?", nil
}

func (failingDocumentEnricher) Summarize(context.Context, string, EnrichmentOptions) (string, error) {
	return "", errors.New("model unavailable")
}

func (failingDocumentEnricher) GenerateQuestions(context.Context, string, string, EnrichmentOptions) ([]string, error) {
	return nil, errors.New("model unavailable")
}

func TestNormalizeGeneratedQuestionsRemovesDuplicatesAndInvalidValues(t *testing.T) {
	got := normalizeGeneratedQuestions([]string{
		" 如何配置缓存？ ",
		"如何配置缓存？",
		"",
		strings.Repeat("x", 41),
	}, 2, 40)
	if len(got) != 1 || got[0] != "如何配置缓存？" {
		t.Fatalf("unexpected normalized questions: %#v", got)
	}
}

func TestLLMDocumentEnricherGeneratesQuestionsThroughPromptCompleter(t *testing.T) {
	client := &promptCompleterStub{}
	enricher := NewLLMDocumentEnricher(client)
	questions, err := enricher.GenerateQuestions(context.Background(), "Guide", "Chunk content", EnrichmentOptions{QuestionCount: 2, MaxQuestionLength: 80})
	if err != nil {
		t.Fatalf("generate questions: %v", err)
	}
	if len(questions) != 2 || questions[0] != "Question one?" {
		t.Fatalf("unexpected questions: %#v", questions)
	}
	if len(client.prompts) != 1 || !strings.Contains(client.prompts[0], "Chunk content") {
		t.Fatalf("unexpected prompt: %#v", client.prompts)
	}
}

func TestEnhancerNodeRunnerLLMFailureDoesNotBlockChunks(t *testing.T) {
	runner := NewEnhancerNodeRunner(failingDocumentEnricher{})
	next, output, err := runner.Run(context.Background(), ingestionworkflow.ExecutionState{
		Parsed: ingestionworkflow.ParsedDocument{Title: "Doc", Content: "A retrievable document."},
		Chunks: []ingestionworkflow.ChunkPayload{{Index: 0, Content: "A retrievable document."}},
	}, ingestiondomain.PipelineNode{Settings: map[string]any{
		"tasks": []any{map[string]any{"type": "summary"}, map[string]any{"type": "questions"}}, "questionCount": 2,
	}})
	if err != nil {
		t.Fatalf("run enhancer: %v", err)
	}
	if next.Enrichment.SummaryStatus != "failed" {
		t.Fatalf("expected failed summary status, got %#v", next.Enrichment)
	}
	if len(next.Chunks[0].Questions) != 0 {
		t.Fatalf("expected no questions after LLM failure, got %#v", next.Chunks[0].Questions)
	}
	if output["mode"] != "llm_degraded" {
		t.Fatalf("expected degraded mode, got %#v", output["mode"])
	}
}

func TestSampleDocumentForSummaryKeepsHeadMiddleAndTail(t *testing.T) {
	content := "HEAD" + strings.Repeat("a", 100) + "MIDDLE" + strings.Repeat("b", 100) + "TAIL"
	got := sampleDocumentForSummary(content, 60)
	for _, marker := range []string{"HEAD", "MIDDLE", "TAIL"} {
		if !strings.Contains(got, marker) {
			t.Fatalf("sample should contain %s: %q", marker, got)
		}
	}
}
