package citation

import (
	"html"
	"regexp"
)

var (
	publicKBTagRE = regexp.MustCompile(`(?is)<kb\b[^>]*>`)
	chunkAttrRE   = regexp.MustCompile(`(?i)\bchunk_id\s*=\s*"([^"]+)"`)
	kbAttrRE      = regexp.MustCompile(`(?i)\bkb_id\s*=\s*"([^"]*)"`)
	docAttrRE     = regexp.MustCompile(`(?i)\bdoc\s*=\s*"([^"]*)"`)
	kindAttrRE    = regexp.MustCompile(`(?i)\bkind\s*=\s*"([^"]*)"`)
)

// CompactPublicCitations folds canonical <kb/> tags from prior assistant
// turns back into this request's private <ref/> handles, registering each
// chunk so historical citations stay resolvable for the current request.
func (r *Registry) CompactPublicCitations(text string) string {
	if r == nil || text == "" {
		return text
	}
	return publicKBTagRE.ReplaceAllStringFunc(text, func(tag string) string {
		chunkID := attr(chunkAttrRE, tag)
		if chunkID == "" {
			return tag
		}
		handle := r.RegisterChunk(ChunkReference{
			ChunkID:         chunkID,
			KnowledgeBaseID: attr(kbAttrRE, tag),
			DocumentTitle:   attr(docAttrRE, tag),
			Kind:            attr(kindAttrRE, tag),
		})
		if handle == "" {
			return tag
		}
		return `<ref id="` + handle + `"/>`
	})
}

func attr(expression *regexp.Regexp, tag string) string {
	match := expression.FindStringSubmatch(tag)
	if len(match) != 2 {
		return ""
	}
	return html.UnescapeString(match[1])
}
