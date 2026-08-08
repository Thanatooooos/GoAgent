package citation

import (
	"regexp"
	"strings"

	"local/rag-project/internal/framework/convention"
)

// ChunkReference carries the durable identity and display metadata for a
// registered knowledge chunk. Handles never persist; this struct is the
// registry-side mirror used to expand <ref/> into public <kb/> tags.
type ChunkReference struct {
	ChunkID         string
	DocumentID      string
	KnowledgeBaseID string
	DocumentTitle   string
}

var shortHandleRE = regexp.MustCompile(`(?i)^[cdb][1-9][0-9]*$`)

// Registry maps durable identifiers to request-local handles for one chat
// request. It is never persisted or accepted across requests.
type Registry struct {
	chunks *handleTable[ChunkReference]
	docs   *handleTable[struct{}]
	kbs    *handleTable[struct{}]
}

func NewRegistry() *Registry {
	return &Registry{
		chunks: newHandleTable[ChunkReference]("c"),
		docs:   newHandleTable[struct{}]("d"),
		kbs:    newHandleTable[struct{}]("b"),
	}
}

// RegisterChunk returns the cN handle for the chunk. A model-emitted
// handle-shaped ID is echoed back only when it already exists.
func (r *Registry) RegisterChunk(ref ChunkReference) string {
	if r == nil {
		return ""
	}
	ref.ChunkID = strings.TrimSpace(ref.ChunkID)
	if ref.ChunkID == "" {
		return ""
	}
	if shortHandleRE.MatchString(ref.ChunkID) {
		return r.knownChunk(ref.ChunkID)
	}
	return r.chunks.register(ref.ChunkID, ref)
}

func (r *Registry) knownChunk(handle string) string {
	if r.chunks.has(handle) {
		return handle
	}
	return ""
}

func (r *Registry) RegisterDocument(id string) string {
	if r == nil {
		return ""
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if shortHandleRE.MatchString(id) {
		return r.knownDocument(id)
	}
	return r.docs.register(id, struct{}{})
}

func (r *Registry) knownDocument(handle string) string {
	if r.docs.has(handle) {
		return handle
	}
	return ""
}

func (r *Registry) RegisterKnowledgeBase(id string) string {
	if r == nil {
		return ""
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ""
	}
	if shortHandleRE.MatchString(id) {
		return r.knownKB(id)
	}
	return r.kbs.register(id, struct{}{})
}

func (r *Registry) knownKB(handle string) string {
	if r.kbs.has(handle) {
		return handle
	}
	return ""
}

// RegisterChunks registers retrieved chunks together with their document and
// knowledge-base identities.
func (r *Registry) RegisterChunks(chunks []convention.RetrievedChunk) {
	for _, chunk := range chunks {
		r.RegisterKnowledgeBase(chunk.KnowledgeBaseID)
		r.RegisterDocument(chunk.DocumentID)
		r.RegisterChunk(ChunkReference{
			ChunkID:         chunk.ID,
			DocumentID:      chunk.DocumentID,
			KnowledgeBaseID: chunk.KnowledgeBaseID,
			DocumentTitle:   readMetadataString(chunk.Metadata, "document_title"),
		})
	}
}

// ResolveChunk returns the durable chunk identity for a cN handle.
func (r *Registry) ResolveChunk(handle string) (ChunkReference, bool) {
	if r == nil {
		return ChunkReference{}, false
	}
	key, value, ok := r.chunks.resolve(handle)
	if !ok {
		return ChunkReference{}, false
	}
	value.ChunkID = key
	return value, true
}

func readMetadataString(metadata map[string]any, key string) string {
	if metadata == nil {
		return ""
	}
	value, ok := metadata[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
