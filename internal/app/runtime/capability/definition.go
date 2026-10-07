// Package capability is runtime's sole model-visible tool catalog.
package capability

import (
	"context"
	"encoding/json"

	"local/rag-project/internal/app/rag/core/citation"
	"local/rag-project/internal/app/runtime/persistence"
)

type Value = json.RawMessage

// Def follows the Miso tool shape. GoAgent supplies the concrete validation,
// operation description, and execution functions for each capability.
type Def struct {
	ID          string
	Description string
	JSONSchema  json.RawMessage
	Validate    func(Value) error
	Describe    func(Value, Context) (Operation, error)
	Execute     func(args Value, ctx Context) (Result, error)
}

// Context carries only runtime-approved facts. Tool arguments must never set
// identity, KB scope, or policy themselves.
type Context struct {
	Context                 context.Context
	RuntimeSessionID        string
	ConversationID          string
	UserMessageID           string
	UserID                  string
	Question                string
	KnowledgeBaseIDs        []string
	AllowedWebDomains       []string
	RetrieveSearchMode      string
	Timezone                string
	AllowKnowledgeRetrieval bool
	AllowMemoryRecall       bool
	AllowMemoryMutation     bool
	AllowWebSearch          bool
	AllowEpisodeArchive     bool
	AllowScheduledTasks     bool
	Citations               *citation.Registry
	Work                    *WorkScope
	ToolCallID              string
}

// WorkScope is server-owned accepted input, never model-supplied arguments.
type WorkScope struct {
	TopicID          string
	ItemID           string
	TurnID           string
	ArtifactID       string
	ArtifactRevision int
	Action           string
	Snapshot         string
}

type Operation struct {
	ID      string
	Summary string
	Input   Value
}

type Result struct {
	Content  string
	Value    Value
	Evidence []persistence.EvidenceRef
}

// ModelDefinition is the callback-free projection sent to an LLM.
type ModelDefinition struct {
	ID          string
	Description string
	JSONSchema  json.RawMessage
}
