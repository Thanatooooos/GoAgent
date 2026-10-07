package runtime

import (
	"strings"

	"local/rag-project/internal/app/runtime/persistence"
)

// ContextSource is one independently rendered system-context block. Sources
// are stable runtime inputs; conversation and tool history are injected after
// these blocks, never folded into them.
type ContextSource struct {
	Key    string
	Render func(SourceContext) string
}

// SourceContext is deliberately small: a source may derive its block from the
// accepted request and durable session, but never from transport state.
type SourceContext struct {
	Request               RunRequest
	Session               persistence.Session
	CoreMemoryContext     string
	DerivedProfileContext string
}

type ContextSources struct{ items []ContextSource }

func NewContextSources(sources ...ContextSource) *ContextSources {
	set := &ContextSources{}
	for _, source := range sources {
		set.Append(source)
	}
	return set
}

// Append preserves insertion order. A later source with the same key replaces
// the earlier source in place, which allows a deployment to override a default.
func (s *ContextSources) Append(source ContextSource) {
	if s == nil || strings.TrimSpace(source.Key) == "" {
		return
	}
	for index, existing := range s.items {
		if existing.Key == source.Key {
			s.items[index] = source
			return
		}
	}
	s.items = append(s.items, source)
}

func (s *ContextSources) Blocks(context SourceContext) []string {
	if s == nil {
		return []string{}
	}
	blocks := make([]string, 0, len(s.items))
	for _, source := range s.items {
		if source.Render == nil {
			continue
		}
		if block := strings.TrimSpace(source.Render(context)); block != "" {
			blocks = append(blocks, block)
		}
	}
	return blocks
}
