package citation

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	refTagRE       = regexp.MustCompile(`(?i)<ref\s+id\s*=\s*"([^"]+)"\s*/?>`)
	refCandidateRE = regexp.MustCompile(`(?is)<ref\b[^>]*>`)
	modelKBTagRE   = regexp.MustCompile(`(?is)<(?:kb|web)\b[^>]*>`)
)

// stripCitationMarkup removes private <ref> and output-only <kb> tags.
func stripCitationMarkup(text string) string {
	text = modelKBTagRE.ReplaceAllString(text, "")
	return refCandidateRE.ReplaceAllString(text, "")
}

// ExpandText converts the private <ref/> protocol into the public <kb/> tag
// contract. Unknown handles fail closed and disappear; model-written <kb>
// tags are dropped because public tags are output-only.
func (r *Registry) ExpandText(text string, enabled bool) string {
	if r == nil || text == "" {
		return text
	}
	text = modelKBTagRE.ReplaceAllString(text, "")
	if !enabled {
		return stripCitationMarkup(text)
	}
	return refCandidateRE.ReplaceAllStringFunc(text, func(tag string) string {
		match := refTagRE.FindStringSubmatch(tag)
		if len(match) != 2 {
			return ""
		}
		chunk, web, kind := r.ResolveHandle(match[1])
		switch kind {
		case "web":
			return renderWebTag(web)
		case "chunk":
			return renderKBTag(chunk)
		default:
			return ""
		}
	})
}

func renderWebTag(ref WebReference) string {
	attrs := fmt.Sprintf(`url="%s"`, escapeAttr(ref.URL))
	if strings.TrimSpace(ref.Title) != "" {
		attrs += fmt.Sprintf(` title="%s"`, escapeAttr(ref.Title))
	}
	return "<web " + attrs + " />"
}

func renderKBTag(ref ChunkReference) string {
	attrs := fmt.Sprintf(`doc="%s" chunk_id="%s"`, escapeAttr(ref.DocumentTitle), escapeAttr(ref.ChunkID))
	if ref.KnowledgeBaseID != "" {
		attrs += fmt.Sprintf(` kb_id="%s"`, escapeAttr(ref.KnowledgeBaseID))
	}
	return "<kb " + attrs + " />"
}

// StreamExpander expands <ref/> tags while streaming, holding back tail bytes
// that could be part of a partial tag so SSE never leaks a half tag.
type StreamExpander struct {
	registry *Registry
	enabled  bool
	pending  string
}

func NewStreamExpander(registry *Registry, enabled bool) *StreamExpander {
	return &StreamExpander{registry: registry, enabled: enabled}
}

func (d *StreamExpander) Feed(chunk string) string {
	if d == nil {
		return chunk
	}
	if d.registry == nil && d.enabled {
		return chunk
	}
	data := d.pending + chunk
	d.pending = ""
	var out strings.Builder
	for data != "" {
		idx := strings.Index(data, "<")
		if idx < 0 {
			out.WriteString(data)
			break
		}
		out.WriteString(data[:idx])
		data = data[idx:]
		lower := strings.ToLower(data)
		if isSourceTagPending(lower) && !strings.Contains(data, ">") {
			d.pending = data
			break
		}
		if isRefTagStart(lower) {
			end := strings.IndexByte(data, '>')
			if end < 0 {
				d.pending = data
				break
			}
			tag := data[:end+1]
			if d.registry != nil {
				out.WriteString(d.registry.ExpandText(tag, d.enabled))
			} else {
				out.WriteString(stripCitationMarkup(tag))
			}
			data = data[end+1:]
			continue
		}
		if isNamedTagStart(lower, "kb") || isNamedTagStart(lower, "web") {
			end := strings.IndexByte(data, '>')
			if end < 0 {
				d.pending = data
				break
			}
			data = data[end+1:]
			continue
		}
		out.WriteByte('<')
		data = data[1:]
	}
	return out.String()
}

func (d *StreamExpander) Flush() string {
	if d == nil {
		return ""
	}
	pending := d.pending
	d.pending = ""
	if pending == "" {
		return ""
	}
	return ""
}

func isRefTagStart(value string) bool {
	return isNamedTagStart(value, "ref")
}

func isNamedTagStart(value, name string) bool {
	prefix := "<" + name
	if !strings.HasPrefix(value, prefix) {
		return false
	}
	if len(value) == len(prefix) {
		return true
	}
	next := value[len(prefix)]
	return next == ' ' || next == '\t' || next == '\r' || next == '\n' || next == '>'
}

func isSourceTagPending(value string) bool {
	for _, name := range []string{"ref", "kb", "web"} {
		prefix := "<" + name
		if (len(value) <= len(prefix) && strings.HasPrefix(prefix, value)) || isNamedTagStart(value, name) {
			return true
		}
	}
	return false
}
