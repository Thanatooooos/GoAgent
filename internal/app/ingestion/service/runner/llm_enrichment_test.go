package runner

import (
	"context"
	"errors"
	"strings"
	"testing"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ingestionworkflow "local/rag-project/internal/app/ingestion/service/workflow"
	"local/rag-project/internal/framework/llmgen"
)

type failingDocumentEnricher struct{}

type promptCompleterStub struct{ prompts []string }

func (s *promptCompleterStub) Chat(prompt string) (string, error) {
	s.prompts = append(s.prompts, prompt)
	return "Question one? [[r1]]\nQuestion two?", nil
}

func (failingDocumentEnricher) Summarize(context.Context, string, EnrichmentOptions) (string, error) {
	return "", errors.New("model unavailable")
}

func (failingDocumentEnricher) GenerateQuestions(context.Context, string, string, EnrichmentOptions) (GenerateQuestionsResult, error) {
	return GenerateQuestionsResult{}, errors.New("model unavailable")
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
	res, err := enricher.GenerateQuestions(context.Background(), "Guide", "Chunk content", EnrichmentOptions{QuestionCount: 2, MaxQuestionLength: 80, SourceChunkID: "chunk-1"})
	if err != nil {
		t.Fatalf("generate questions: %v", err)
	}
	if len(res.Questions) != 2 || res.Questions[0].Text != "Question one?" {
		t.Fatalf("unexpected questions: %#v", res.Questions)
	}
	if res.Questions[0].SourceChunkID != "chunk-1" {
		t.Fatalf("source chunk not resolved: %#v", res.Questions[0])
	}
	if len(client.prompts) != 1 || !strings.Contains(client.prompts[0], "Chunk content") {
		t.Fatalf("unexpected prompt: %#v", client.prompts)
	}
	if !strings.Contains(client.prompts[0], "r1") {
		t.Fatalf("prompt should mention source handle r1: %#v", client.prompts[0])
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

func TestResolveGeneratedQuestionsStripsValidRef(t *testing.T) {
	handles := llmgen.NewHandleSet("r")
	handles.Encode("chunk-1")
	res := resolveGeneratedQuestions([]string{"问题一 [[r1]]", "问题二"}, handles, "chunk-1")
	if len(res.Questions) != 2 {
		t.Fatalf("questions = %#v", res.Questions)
	}
	if res.Questions[0].Text != "问题一" || res.Questions[0].SourceChunkID != "chunk-1" {
		t.Fatalf("valid ref not resolved: %+v", res.Questions[0])
	}
	if res.Questions[1].SourceChunkID != "" {
		t.Fatalf("no-ref question should have empty source: %+v", res.Questions[1])
	}
	if res.RejectedRefs != 0 {
		t.Fatalf("RejectedRefs = %d", res.RejectedRefs)
	}
}

func TestResolveGeneratedQuestionsRejectsHallucinatedRef(t *testing.T) {
	handles := llmgen.NewHandleSet("r")
	handles.Encode("chunk-1")
	res := resolveGeneratedQuestions([]string{"问题三 [[r2]]"}, handles, "chunk-1")
	if len(res.Questions) != 1 {
		t.Fatalf("questions = %#v", res.Questions)
	}
	if res.Questions[0].Text != "问题三" {
		t.Fatalf("hallucinated ref should be stripped, got %q", res.Questions[0].Text)
	}
	if res.Questions[0].SourceChunkID != "" {
		t.Fatalf("hallucinated ref must not set source: %+v", res.Questions[0])
	}
	if res.RejectedRefs != 1 {
		t.Fatalf("RejectedRefs = %d, want 1", res.RejectedRefs)
	}
}

func TestResolveGeneratedQuestionsWithoutSource(t *testing.T) {
	res := resolveGeneratedQuestions([]string{"问题四 [[r1]]"}, nil, "")
	if len(res.Questions) != 1 || res.Questions[0].Text != "问题四 [[r1]]" {
		t.Fatalf("no-source mode should pass through: %#v", res.Questions)
	}
}
