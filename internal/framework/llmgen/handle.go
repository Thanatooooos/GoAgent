package llmgen

import (
	"strconv"
	"strings"
)

// HandleSet encodes durable identifiers as request-local short handles (e.g.
// r1, r2) for LLM prompts, and decodes model output back fail-closed. Handles
// are per-request and never persisted.
type HandleSet struct {
	prefix   string
	byKey    map[string]string
	byHandle map[string]string
	next     int
}

func NewHandleSet(prefix string) *HandleSet {
	if prefix == "" {
		prefix = "r"
	}
	return &HandleSet{
		prefix:   prefix,
		byKey:    map[string]string{},
		byHandle: map[string]string{},
		next:     1,
	}
}

// Encode returns the handle for a durable id, reusing an existing handle on
// duplicate. Empty ids are rejected.
func (h *HandleSet) Encode(durable string) (string, bool) {
	if h == nil {
		return "", false
	}
	durable = strings.TrimSpace(durable)
	if durable == "" {
		return "", false
	}
	if handle, ok := h.byKey[durable]; ok {
		return handle, true
	}
	handle := h.prefix + strconv.Itoa(h.next)
	h.next++
	h.byKey[durable] = handle
	h.byHandle[handle] = durable
	return handle, true
}

// Resolve decodes a handle back to its durable id. Unknown handles fail closed.
func (h *HandleSet) Resolve(handle string) (string, bool) {
	if h == nil {
		return "", false
	}
	id, ok := h.byHandle[strings.ToLower(strings.TrimSpace(handle))]
	return id, ok
}

// ResolveAll decodes a batch, returning resolved ids and unresolved handles.
func (h *HandleSet) ResolveAll(handles []string) (resolved, unresolved []string) {
	for _, handle := range handles {
		if id, ok := h.Resolve(handle); ok {
			resolved = append(resolved, id)
		} else {
			unresolved = append(unresolved, handle)
		}
	}
	return resolved, unresolved
}
