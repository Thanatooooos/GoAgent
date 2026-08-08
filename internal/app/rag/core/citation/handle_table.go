package citation

import (
	"strconv"
	"strings"
)

// handleTable maps durable identifiers to request-local handles and back.
// Handles are per-request and never persisted.
type handleTable[M any] struct {
	prefix   string
	byKey    map[string]string
	byHandle map[string]handleEntry[M]
	next     int
}

type handleEntry[M any] struct {
	key   string
	value M
}

func newHandleTable[M any](prefix string) *handleTable[M] {
	return &handleTable[M]{
		prefix:   prefix,
		byKey:    map[string]string{},
		byHandle: map[string]handleEntry[M]{},
		next:     1,
	}
}

// register returns the handle for key, reusing an existing one on duplicate.
func (t *handleTable[M]) register(key string, value M) string {
	if handle, ok := t.byKey[key]; ok {
		return handle
	}
	handle := t.prefix + strconv.Itoa(t.next)
	t.next++
	t.byKey[key] = handle
	t.byHandle[handle] = handleEntry[M]{key: key, value: value}
	return handle
}

func (t *handleTable[M]) has(handle string) bool {
	_, ok := t.byHandle[strings.ToLower(strings.TrimSpace(handle))]
	return ok
}

// resolve returns (durableKey, value, ok). Handle matching is case-insensitive.
func (t *handleTable[M]) resolve(handle string) (string, M, bool) {
	entry, ok := t.byHandle[strings.ToLower(strings.TrimSpace(handle))]
	if !ok {
		var zero M
		return "", zero, false
	}
	return entry.key, entry.value, true
}
