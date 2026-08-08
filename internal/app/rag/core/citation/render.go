package citation

import (
	"fmt"
	"html"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

const retrievalHeader = "<retrieval type=\"knowledge\">"
const retrievalFooter = "</retrieval>"

// RenderStats reports how a budget-aware render behaved.
type RenderStats struct {
	CandidateChunks int  `json:"candidateChunks"`
	RetainedChunks  int  `json:"retainedChunks"`
	TokensBefore    int  `json:"tokensBefore"`
	TokensAfter     int  `json:"tokensAfter"`
	Truncated       bool `json:"truncated"`
}

// RenderKnowledgeContext renders retrieved chunks as a handle-carrying
// <retrieval> block. Every chunk is registered so the model can cite
// <ref id="cN"/>.
func RenderKnowledgeContext(registry *Registry, chunks []convention.RetrievedChunk) string {
	if registry == nil || len(chunks) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString(retrievalHeader)
	for _, chunk := range chunks {
		b.WriteString("\n  ")
		b.WriteString(renderChunkElement(registry, chunk, chunk.Text))
	}
	b.WriteString("\n")
	b.WriteString(retrievalFooter)
	return b.String()
}

// RenderKnowledgeContextWithBudget renders the same block, stopping (or
// truncating chunk text) once the token budget is exhausted.
func RenderKnowledgeContextWithBudget(registry *Registry, chunks []convention.RetrievedChunk, budget int, estimator tokenbudget.Estimator) (string, RenderStats) {
	if estimator == nil {
		estimator = tokenbudget.NewDefaultEstimator()
	}
	stats := RenderStats{CandidateChunks: len(chunks)}
	full := RenderKnowledgeContext(registry, chunks)
	stats.TokensBefore = estimator.EstimateTokens(full)
	if len(chunks) == 0 || budget <= 0 {
		stats.Truncated = len(chunks) > 0
		return "", stats
	}
	headerTokens := estimator.EstimateTokens(retrievalHeader)
	footerTokens := estimator.EstimateTokens(retrievalFooter)
	newlineTokens := estimator.EstimateTokens("\n")

	var b strings.Builder
	b.WriteString(retrievalHeader)
	used := headerTokens + footerTokens
	for _, chunk := range chunks {
		fullPart := renderChunkElement(registry, chunk, chunk.Text)
		if used+newlineTokens+estimator.EstimateTokens(fullPart) <= budget {
			b.WriteString("\n  ")
			b.WriteString(fullPart)
			used += newlineTokens + estimator.EstimateTokens(fullPart)
			stats.RetainedChunks++
			continue
		}
		prefix := renderChunkPrefix(registry, chunk)
		prefixTokens := estimator.EstimateTokens(prefix)
		if used+newlineTokens+prefixTokens+estimator.EstimateTokens("</chunk>") <= budget {
			textBudget := budget - used - newlineTokens - prefixTokens - estimator.EstimateTokens("</chunk>")
			truncatedText, _ := tokenbudget.TruncateText(chunk.Text, textBudget, estimator)
			if strings.TrimSpace(truncatedText) != "" {
				part := prefix + escapeText(truncatedText) + "</chunk>"
				b.WriteString("\n  ")
				b.WriteString(part)
				used += newlineTokens + estimator.EstimateTokens(part)
				stats.RetainedChunks++
			}
		}
		stats.Truncated = true
		break
	}
	b.WriteString("\n")
	b.WriteString(retrievalFooter)
	stats.TokensAfter = estimator.EstimateTokens(b.String())
	stats.Truncated = stats.Truncated || stats.RetainedChunks < stats.CandidateChunks
	return b.String(), stats
}

func renderChunkElement(registry *Registry, chunk convention.RetrievedChunk, content string) string {
	prefix := renderChunkPrefix(registry, chunk)
	return prefix + escapeText(strings.TrimSpace(content)) + "</chunk>"
}

func renderChunkPrefix(registry *Registry, chunk convention.RetrievedChunk) string {
	handle := registry.RegisterChunk(ChunkReference{
		ChunkID:         chunk.ID,
		DocumentID:      chunk.DocumentID,
		KnowledgeBaseID: chunk.KnowledgeBaseID,
		DocumentTitle:   readMetadataString(chunk.Metadata, "document_title"),
	})
	var b strings.Builder
	b.WriteString("<chunk id=\"")
	b.WriteString(handle)
	b.WriteString("\"")
	if chunk.ChunkIndex > 0 {
		fmt.Fprintf(&b, " index=\"%d\"", chunk.ChunkIndex)
	}
	if section := readMetadataString(chunk.Metadata, "section"); section != "" {
		b.WriteString(" section=\"")
		b.WriteString(escapeAttr(section))
		b.WriteString("\"")
	}
	if title := readMetadataString(chunk.Metadata, "document_title"); title != "" {
		b.WriteString(" title=\"")
		b.WriteString(escapeAttr(title))
		b.WriteString("\"")
	}
	b.WriteString(">")
	return b.String()
}

func escapeText(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func escapeAttr(value string) string { return html.EscapeString(value) }
