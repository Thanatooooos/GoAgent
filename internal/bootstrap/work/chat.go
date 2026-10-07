package work

import (
	"context"
	"fmt"

	postgresruntime "local/rag-project/internal/adapter/repository/postgres/runtime"
	postgreswork "local/rag-project/internal/adapter/repository/postgres/work"
	conversationruntime "local/rag-project/internal/app/runtime"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/framework/stream"
)

func (r *Runtime) ConfigureChat(base *conversationruntime.Runtime, conversations conversationruntime.Conversations, messages conversationruntime.ConversationMessages, streams stream.StreamManager) error {
	if base == nil || streams == nil {
		return fmt.Errorf("Work requires the existing runtime and stream manager")
	}
	tools := capability.NewRegistry()
	if err := r.registerTools(tools); err != nil {
		return err
	}
	for _, id := range []string{capability.WebSearchID, capability.WebFetchID, capability.RetrieveKnowledgeID} {
		def, ok := base.Tools.Get(id)
		if ok {
			if id == capability.RetrieveKnowledgeID {
				def = r.scopeRetrieval(def)
			}
			if err := tools.Register(def); err != nil {
				return err
			}
		}
	}
	r.Kernel = &conversationruntime.Runtime{ExecutionLeaseDuration: base.ExecutionLeaseDuration, Model: base.Model, Lifecycle: base.Lifecycle, TaskJournal: base.TaskJournal, Tools: tools, History: postgreswork.History{Store: r.Store}, ContextTokenBudget: base.ContextTokenBudget,
		Sources: conversationruntime.NewContextSources(conversationruntime.ContextSource{Key: "work/instructions", Render: func(conversationruntime.SourceContext) string {
			return workSystemInstruction
		}}, conversationruntime.ContextSource{Key: "work/document-schema", Render: func(conversationruntime.SourceContext) string {
			return workDocumentSchemaInstruction
		}}, conversationruntime.Date(), conversationruntime.ContextSource{Key: "work/snapshot", Render: func(s conversationruntime.SourceContext) string {
			if s.Request.Work == nil {
				return ""
			}
			w := s.Request.Work
			return fmt.Sprintf(workSnapshotContextTemplate, w.Action, w.ItemID, w.ArtifactID, w.ArtifactRevision, w.Snapshot)
		}})}
	r.Chat = conversationruntime.NewChatService(conversations, messages, r.Kernel)
	if preparer, ok := messages.(postgresruntime.PublicationPreparer); ok {
		r.Chat.SetPublication(postgresruntime.NewChatPublisher(r.DB, preparer))
	}
	r.Chat.SetConversationGuard(func(ctx context.Context, user, conversation string) error {
		return r.Store.ValidateWorkConversation(ctx, user, conversation)
	})
	r.Streams = streams
	return nil
}
