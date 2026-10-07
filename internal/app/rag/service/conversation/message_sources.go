package conversation

import (
	"html"
	"regexp"
	"strings"

	"local/rag-project/internal/app/rag/domain"
)

var (
	publicSourceTag = regexp.MustCompile(`(?i)<(kb|web)\b[^>]*>`)
	sourceAttribute = regexp.MustCompile(`([a-zA-Z_]+)\s*=\s*"([^"]*)"`)
)

func messageSources(role, content string) []domain.MessageSource {
	sources := make([]domain.MessageSource, 0)
	if role != "assistant" {
		return sources
	}
	seen := make(map[string]bool)
	for _, tag := range publicSourceTag.FindAllStringSubmatch(content, -1) {
		attrs := make(map[string]string)
		for _, attr := range sourceAttribute.FindAllStringSubmatch(tag[0], -1) {
			attrs[strings.ToLower(attr[1])] = strings.TrimSpace(html.UnescapeString(attr[2]))
		}
		var source domain.MessageSource
		var key string
		switch strings.ToLower(tag[1]) {
		case "kb":
			if attrs["chunk_id"] == "" {
				continue
			}
			source = domain.MessageSource{Type: "kb", Title: attrs["doc"], ChunkID: attrs["chunk_id"], KnowledgeBaseID: attrs["kb_id"], Kind: attrs["kind"]}
			key = "kb:" + source.ChunkID
		case "web":
			if !strings.HasPrefix(attrs["url"], "https://") && !strings.HasPrefix(attrs["url"], "http://") {
				continue
			}
			source = domain.MessageSource{Type: "web", Title: attrs["title"], URL: attrs["url"]}
			key = "web:" + source.URL
		}
		if key != "" && !seen[key] {
			seen[key] = true
			sources = append(sources, source)
		}
	}
	return sources
}
