package process

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	corechunk "local/rag-project/internal/app/core/chunk"
	"local/rag-project/internal/app/knowledge/domain"
	"local/rag-project/internal/app/knowledge/port"
)

type documentEnrichment struct {
	Summary        string
	SummaryStatus  string
	SummaryError   string
	ChunkSummaries map[int]string
	Questions      map[int][]string
}

func (s *DocumentProcessService) enrichDocument(ctx context.Context, document domain.KnowledgeDocument, text string, chunks []corechunk.Chunk) documentEnrichment {
	result := documentEnrichment{ChunkSummaries: make(map[int]string), Questions: make(map[int][]string)}
	if s.chat == nil {
		result.SummaryStatus = "degraded"
		result.SummaryError = "chat service is unavailable"
		return result
	}
	summary, err := s.chat.Chat(documentSummaryPrompt + truncateRunes(text, 12000))
	if err != nil {
		result.SummaryStatus = "failed"
		result.SummaryError = err.Error()
	} else {
		result.Summary = strings.TrimSpace(summary)
		result.SummaryStatus = "success"
	}
	for _, chunk := range chunks {
		chunkSummary, err := s.chat.Chat(chunkSummaryPrompt + chunk.Text)
		if err == nil {
			result.ChunkSummaries[chunk.Index] = truncateRunes(strings.TrimSpace(chunkSummary), 180)
		}
		prompt := fmt.Sprintf(chunkQuestionsPromptTemplate, chunk.Text)
		response, err := s.chat.Chat(prompt)
		if err != nil {
			continue
		}
		questions := normalizeQuestions(strings.Split(response, "\n"))
		if len(questions) > 0 {
			result.Questions[chunk.Index] = questions
		}
	}
	return result
}

func normalizeQuestions(values []string) []string {
	result := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(strings.TrimLeft(value, "-•0123456789.、) "))
		if value == "" || utf8.RuneCountInString(value) > 120 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == 2 {
			break
		}
	}
	return result
}

func truncateRunes(value string, limit int) string {
	value = strings.TrimSpace(value)
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

func keywords(value string) []string {
	result := make([]string, 0, 5)
	seen := map[string]struct{}{}
	for _, token := range strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r > 127)
	}) {
		token = strings.TrimSpace(token)
		if utf8.RuneCountInString(token) < 2 {
			continue
		}
		if _, ok := seen[token]; ok {
			continue
		}
		seen[token] = struct{}{}
		result = append(result, token)
		if len(result) == 5 {
			break
		}
	}
	return result
}

func (s *DocumentProcessService) buildQuestionVectors(ctx context.Context, document domain.KnowledgeDocument, modelID string, chunks []corechunk.Chunk, enrichment documentEnrichment, parentIDs map[int]string, generation string) ([]port.ChunkVector, error) {
	texts := make([]string, 0)
	type ref struct{ chunkIndex, questionIndex int }
	refs := make([]ref, 0)
	sourceContent := make(map[int]string, len(chunks))
	for _, chunk := range chunks {
		sourceContent[chunk.Index] = chunk.Text
		for questionIndex, question := range enrichment.Questions[chunk.Index] {
			texts = append(texts, question)
			refs = append(refs, ref{chunk.Index, questionIndex})
		}
	}
	if len(texts) == 0 {
		return nil, nil
	}
	embeddings, err := s.embedding.EmbedBatchWithModel(texts, modelID)
	if err != nil {
		return nil, err
	}
	if len(embeddings) != len(refs) {
		return nil, fmt.Errorf("question embedding count mismatch")
	}
	vectors := make([]port.ChunkVector, 0, len(refs))
	for index, item := range refs {
		sourceID := versionedTextID(generation, "c", item.chunkIndex)
		metadata := map[string]any{"record_type": "question", "source_chunk_id": sourceID, "source_content": sourceContent[item.chunkIndex], "document_id": document.ID, "document_name": document.Name, "knowledge_base_id": document.KnowledgeBaseID}
		if parentID := parentIDs[item.chunkIndex]; parentID != "" {
			metadata["parent_chunk_id"] = parentID
		}
		vectors = append(vectors, port.ChunkVector{ChunkID: fmt.Sprintf("q-%s-%d-%d", strings.ReplaceAll(generation, "-", ""), item.chunkIndex, item.questionIndex), DocumentID: document.ID, KnowledgeBaseID: document.KnowledgeBaseID, Index: item.chunkIndex, Text: texts[index], Embedding: embeddings[index], Metadata: metadata})
	}
	return vectors, nil
}
