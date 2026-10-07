package citation

import (
	"fmt"
	"html"
	"strings"

	"local/rag-project/internal/app/rag/core/tokenbudget"
	"local/rag-project/internal/framework/convention"
)

const retrievalHeader = `<retrieval type="knowledge">
引用：仅输出 <ref id="cN"/>，N 为下方句柄。`
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
	if registry == nil || len(chunks) == 0 || budget <= 0 {
		stats.Truncated = len(chunks) > 0
		return "", stats
	}
	if estimator.EstimateTokens(retrievalHeader+"\n"+retrievalFooter) > budget {
		stats.Truncated = true
		return "", stats
	}

	var b strings.Builder
	b.WriteString(retrievalHeader)
	for _, chunk := range chunks {
		fullPart := renderChunkElement(registry, chunk, chunk.Text)
		candidate := b.String() + "\n  " + fullPart + "\n" + retrievalFooter
		if estimator.EstimateTokens(candidate) <= budget {
			b.WriteString("\n  " + fullPart)
			stats.RetainedChunks++
			continue
		}
		prefix := renderChunkPrefix(registry, chunk)
		fixedPortion := b.String() + "\n  " + prefix + "</chunk>" + "\n" + retrievalFooter
		textBudget := budget - estimator.EstimateTokens(fixedPortion)
		truncatedText, _ := tokenbudget.TruncateText(chunk.Text, textBudget, estimator)
		if strings.TrimSpace(truncatedText) != "" {
			part := prefix + escapeText(truncatedText) + "</chunk>"
			truncatedCandidate := b.String() + "\n  " + part + "\n" + retrievalFooter
			if estimator.EstimateTokens(truncatedCandidate) <= budget {
				b.WriteString("\n  " + part)
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
		DocumentTitle:   readDocumentTitle(chunk.Metadata),
		Kind:            readMetadataString(chunk.Metadata, "record_type"),
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
	if title := readDocumentTitle(chunk.Metadata); title != "" {
		b.WriteString(" title=\"")
		b.WriteString(escapeAttr(title))
		b.WriteString("\"")
	}
	b.WriteString(">")
	return b.String()
}

// readDocumentTitle resolves the document title from chunk metadata, preferring
// document_name (the key written during document processing) and falling back
// to document_title for older data.
func readDocumentTitle(metadata map[string]any) string {
	if title := readMetadataString(metadata, "document_name"); title != "" {
		return title
	}
	return readMetadataString(metadata, "document_title")
}

func escapeText(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return replacer.Replace(value)
}

func escapeAttr(value string) string { return html.EscapeString(value) }
