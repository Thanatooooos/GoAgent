package evaluation

import (
	"context"
	"fmt"
	"math"
	"strings"
)

const defaultRewriteSemanticThreshold = 0.65

type QueryEmbedder interface {
	Embed(text string) ([]float32, error)
	EmbedBatch(texts []string) ([][]float32, error)
}

type SubQuestionSimilarity struct {
	Text       string  `json:"text"`
	Similarity float64 `json:"similarity"`
}

type RewriteSemanticEvaluation struct {
	OriginalText            string                  `json:"original_text"`
	RewrittenText           string                  `json:"rewritten_text"`
	RewriteSimilarity       float64                 `json:"rewrite_similarity"`
	SubQuestionSimilarities []SubQuestionSimilarity `json:"sub_question_similarities,omitempty"`
	SubSimilarityMin        float64                 `json:"sub_similarity_min,omitempty"`
	SubSimilarityAvg        float64                 `json:"sub_similarity_avg,omitempty"`
	SemanticScore           float64                 `json:"semantic_score"`
	Threshold               float64                 `json:"threshold"`
	PassedThreshold         bool                    `json:"passed_threshold"`
}

func EvaluateRewriteSemantic(_ context.Context, embedder QueryEmbedder, sample RewriteSample) (RewriteSemanticEvaluation, error) {
	if embedder == nil {
		return RewriteSemanticEvaluation{}, fmt.Errorf("embedder is required")
	}
	originalText := buildRewriteEmbeddingOriginalText(sample)
	rewrittenText := strings.TrimSpace(sample.RewrittenQuery)
	if strings.TrimSpace(originalText) == "" || rewrittenText == "" {
		return RewriteSemanticEvaluation{}, fmt.Errorf("original and rewritten text are required")
	}

	texts := []string{originalText, rewrittenText}
	subTexts := make([]string, 0, len(sample.SubQuestions))
	for _, question := range sample.SubQuestions {
		question = strings.TrimSpace(question)
		if question == "" {
			continue
		}
		subTexts = append(subTexts, question)
		texts = append(texts, question)
	}

	vectors, err := embedder.EmbedBatch(texts)
	if err != nil {
		return RewriteSemanticEvaluation{}, fmt.Errorf("embed rewrite texts: %w", err)
	}
	if len(vectors) != len(texts) {
		return RewriteSemanticEvaluation{}, fmt.Errorf("embed batch size mismatch: got %d vectors for %d texts", len(vectors), len(texts))
	}

	rewriteSimilarity, err := cosineSimilarity(vectors[0], vectors[1])
	if err != nil {
		return RewriteSemanticEvaluation{}, err
	}

	evaluation := RewriteSemanticEvaluation{
		OriginalText:      originalText,
		RewrittenText:     rewriteCandidateText(sample.RewrittenQuery, sample.SubQuestions),
		RewriteSimilarity: roundSummaryScore(rewriteSimilarity),
		Threshold:         defaultRewriteSemanticThreshold,
	}
	for i, question := range subTexts {
		vectorIndex := i + 2
		similarity, simErr := cosineSimilarity(vectors[0], vectors[vectorIndex])
		if simErr != nil {
			continue
		}
		evaluation.SubQuestionSimilarities = append(evaluation.SubQuestionSimilarities, SubQuestionSimilarity{
			Text:       question,
			Similarity: roundSummaryScore(similarity),
		})
	}
	if len(evaluation.SubQuestionSimilarities) > 0 {
		evaluation.SubSimilarityMin = roundSummaryScore(minSubQuestionSimilarity(evaluation.SubQuestionSimilarities))
		evaluation.SubSimilarityAvg = roundSummaryScore(avgSubQuestionSimilarity(evaluation.SubQuestionSimilarities))
	}
	evaluation.SemanticScore = roundSummaryScore(buildRewriteSemanticScore(evaluation))
	evaluation.PassedThreshold = evaluation.SemanticScore >= evaluation.Threshold
	return evaluation, nil
}

func buildRewriteEmbeddingOriginalText(sample RewriteSample) string {
	query := strings.TrimSpace(sample.Query)
	if len(sample.History) == 0 {
		return query
	}
	var parts []string
	for _, message := range sample.History {
		content := strings.TrimSpace(message.Content)
		if content == "" {
			continue
		}
		role := strings.TrimSpace(message.Role)
		if role == "" {
			role = "context"
		}
		parts = append(parts, role+": "+content)
	}
	if query != "" {
		parts = append(parts, "user: "+query)
	}
	return strings.Join(parts, "\n")
}

func buildRewriteSemanticScore(evaluation RewriteSemanticEvaluation) float64 {
	if len(evaluation.SubQuestionSimilarities) == 0 {
		return evaluation.RewriteSimilarity
	}
	return 0.5*evaluation.RewriteSimilarity + 0.3*evaluation.SubSimilarityAvg + 0.2*evaluation.SubSimilarityMin
}

func minSubQuestionSimilarity(values []SubQuestionSimilarity) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0].Similarity
	for _, value := range values[1:] {
		if value.Similarity < min {
			min = value.Similarity
		}
	}
	return min
}

func avgSubQuestionSimilarity(values []SubQuestionSimilarity) float64 {
	if len(values) == 0 {
		return 0
	}
	total := 0.0
	for _, value := range values {
		total += value.Similarity
	}
	return total / float64(len(values))
}

func cosineSimilarity(a, b []float32) (float64, error) {
	if len(a) == 0 || len(b) == 0 {
		return 0, fmt.Errorf("empty embedding vector")
	}
	if len(a) != len(b) {
		return 0, fmt.Errorf("embedding dimension mismatch: %d vs %d", len(a), len(b))
	}
	var dot, normA, normB float64
	for i := range a {
		av := float64(a[i])
		bv := float64(b[i])
		dot += av * bv
		normA += av * av
		normB += bv * bv
	}
	if normA == 0 || normB == 0 {
		return 0, nil
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB)), nil
}
