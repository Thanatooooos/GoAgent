package agent

import "testing"

func TestNewRuntimeSession_SeedsToolStageContext(t *testing.T) {
	session := newRuntimeSession(Request{
		Question: "why did doc fail",
		UserID:   "u1",
		TraceID:  "trace-1",
		ToolStage: &ToolStageContext{
			ConversationID:    "conv-1",
			KnowledgeBaseIDs:  []string{"kb-1", "kb-2"},
			RewrittenQuestion: "why did doc fail in indexer",
			SubQuestions:      []string{"indexer failure", "vector store health"},
			NeedRetrieval:     true,
			KnowledgeContext:  "indexer failed because vector store refused connection",
			SearchChannels:    []string{"vector_global", "keyword"},
			HistorySummary:    "user: doc_fail_01 failed || assistant: checking",
		},
	}, 2, "final_answer", "agent_runtime_test")

	if session.Snapshot.Request.ConversationID != "conv-1" {
		t.Fatalf("expected conversation id seed, got %+v", session.Snapshot.Request)
	}
	if len(session.Snapshot.Request.KnowledgeBaseIDs) != 2 {
		t.Fatalf("expected knowledge base ids seed, got %+v", session.Snapshot.Request)
	}
	if session.Snapshot.Context.RewrittenQuery != "why did doc fail in indexer" {
		t.Fatalf("expected rewritten query seed, got %+v", session.Snapshot.Context)
	}
	if session.Snapshot.Context.SearchQuery != "why did doc fail in indexer" {
		t.Fatalf("expected search query to prefer rewritten query, got %+v", session.Snapshot.Context)
	}
	if len(session.Snapshot.Context.Notes) == 0 {
		t.Fatalf("expected tool-stage notes to be seeded, got %+v", session.Snapshot.Context)
	}
}
