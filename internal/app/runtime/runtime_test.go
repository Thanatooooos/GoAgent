package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/app/runtime/capability"
	"local/rag-project/internal/app/runtime/persistence"
	"local/rag-project/internal/framework/convention"
)

func TestRuntimeProjectsDurableToolTurnIntoNextRequest(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	tools := capability.NewRegistry()
	if err := tools.Register(testCapability("lookup", nil)); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{{ToolCalls: []ToolCall{{ID: "call-1", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}}, {Content: "final"}}}
	result, err := (&Runtime{Model: model, Sources: NewContextSources(ContextSource{Key: "core", Render: func(SourceContext) string { return "core" }}), Tools: tools, Lifecycle: lifecycle}).Run(context.Background(), validRunRequest(), nil)
	if err != nil || result.AssistantContent != "final" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(model.requests) != 2 || len(model.requests[1].Messages) != 3 || model.requests[1].Messages[1].Role != ModelRoleAssistant || model.requests[1].Messages[2].Role != ModelRoleTool {
		t.Fatalf("requests=%#v", model.requests)
	}
}

func TestRuntimePublishesAllVisibleTurnsWithoutThinkingOrToolResults(t *testing.T) {
	store := newMemoryStore()
	tools := capability.NewRegistry()
	if err := tools.Register(testCapability("lookup", nil)); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{
		{Thinking: "private reasoning", Content: "先查询。\n\n", ToolCalls: []ToolCall{{ID: "first", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}},
		{Content: "再核对。\n\n", ToolCalls: []ToolCall{{ID: "second", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}},
		{Thinking: "more reasoning", Content: "最终结论。"},
	}}
	sink := &journalSink{}
	result, err := (&Runtime{Model: model, Tools: tools, Lifecycle: NewLifecycle(store, sequentialIDs())}).Run(context.Background(), validRunRequest(), sink)
	if err != nil {
		t.Fatal(err)
	}
	want := "先查询。\n\n再核对。\n\n最终结论。"
	var streamed, final string
	for _, entry := range sink.entries {
		if entry.EventType == EventAnswerDelta {
			streamed += entry.Detail
		}
		if entry.EventType == EventAnswerFinal {
			final = entry.Detail
		}
	}
	if streamed != want || final != want || result.AssistantContent != want {
		t.Fatalf("streamed=%q final=%q result=%q", streamed, final, result.AssistantContent)
	}
	// Model protocol keeps individual assistant/tool turns instead of feeding
	// the aggregated presentation body back into the tool loop.
	last := model.requests[2].Messages
	if len(last) != 5 || last[1].Content != "先查询。\n\n" || last[2].Role != ModelRoleTool || last[3].Content != "再核对。\n\n" || last[4].Role != ModelRoleTool {
		t.Fatalf("model history=%#v", last)
	}
}

func TestRuntimeUsesRequestThinkingForEveryModelTurn(t *testing.T) {
	for _, thinking := range []bool{false, true} {
		tools := capability.NewRegistry()
		if err := tools.Register(testCapability("lookup", nil)); err != nil {
			t.Fatal(err)
		}
		model := &streamSequence{turns: []Turn{
			{ToolCalls: []ToolCall{{ID: "call", CapabilityID: "lookup", Arguments: capability.Value(`{}`)}}},
			{Content: "answer"},
		}}
		request := validRunRequest()
		request.DeepThinking = thinking
		kernel := &Runtime{Model: model, Tools: tools, Lifecycle: NewLifecycle(newMemoryStore(), sequentialIDs())}
		if _, err := kernel.Run(context.Background(), request, nil); err != nil {
			t.Fatal(err)
		}
		for _, call := range model.requests {
			if call.Thinking == nil || *call.Thinking != thinking {
				t.Fatalf("thinking=%t request=%+v", thinking, call)
			}
		}
	}
}

func TestRuntimeRefreshesCoreMemorySourceAfterMutation(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	tools := capability.NewRegistry()
	refreshed := false
	if err := tools.Register(testCapability(capability.MemoryAddID, nil, func() { refreshed = true })); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{
		{ToolCalls: []ToolCall{{ID: "memory-1", CapabilityID: capability.MemoryAddID, Arguments: capability.Value(`{}`)}}},
		{Content: "final"},
	}}
	request := validRunRequest()
	request.Policy.AllowMemoryMutation = true
	_, err := (&Runtime{
		Model: model, Tools: tools, Lifecycle: NewLifecycle(store, sequentialIDs()),
		Sources: NewContextSources(ContextSource{Key: "memory", Render: func(source SourceContext) string { return source.CoreMemoryContext }}),
		CoreMemoryContextLoader: func(context.Context, string) (string, error) {
			if refreshed {
				return "after mutation", nil
			}
			return "before mutation", nil
		},
	}).Run(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 2 || !strings.Contains(strings.Join(model.requests[1].System, "\n"), "after mutation") {
		t.Fatalf("second request did not refresh memory source: %#v", model.requests)
	}
}

func TestRuntimeInjectsConversationHistoryBeforeCurrentTurnAndJournal(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	tools := capability.NewRegistry()
	model := &streamSequence{turns: []Turn{{Content: "final"}}}
	history := historyStub{messages: []ModelMessage{{Role: ModelRoleUser, Content: "earlier question"}, {Role: ModelRoleAssistant, Content: "earlier answer"}}}
	_, err := (&Runtime{Model: model, History: history, Tools: tools, Lifecycle: lifecycle}).Run(context.Background(), validRunRequest(), nil)
	if err != nil {
		t.Fatal(err)
	}
	got := model.requests[0].Messages
	if len(got) != 3 || got[0].Content != "earlier question" || got[1].Content != "earlier answer" || got[2].Content != "hello" {
		t.Fatalf("messages = %#v", got)
	}
}

func TestRuntimeCompactsDurableHistoryBeforeTheModelTurn(t *testing.T) {
	store := newMemoryStore()
	history := &compactionHistoryStub{snapshot: HistorySnapshot{Messages: []HistoryMessage{
		{ID: "m1", Role: ModelRoleUser, Content: strings.Repeat("old question ", 800)},
		{ID: "m2", Role: ModelRoleAssistant, Content: strings.Repeat("old answer ", 800)},
	}}}
	model := &streamSequence{turns: []Turn{
		{Content: `{"schema_version":1,"goal":"finish the task","recent_progress":["old work completed"]}`},
		{Content: "final"},
	}}
	result, err := (&Runtime{Model: model, History: history, Tools: capability.NewRegistry(), Lifecycle: NewLifecycle(store, sequentialIDs()), ContextTokenBudget: 100}).Run(context.Background(), validRunRequest(), nil)
	if err != nil || result.AssistantContent != "final" {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if len(history.stored) != 1 || history.stored[0].CoveredFromMessageID != "m1" || history.stored[0].CoveredToMessageID != "m2" || history.stored[0].SourceMessageCount != 2 {
		t.Fatalf("stored summaries=%#v", history.stored)
	}
	if len(model.requests) != 2 || len(model.requests[0].Tools) != 0 {
		t.Fatalf("requests=%#v", model.requests)
	}
	if len(model.requests[1].Messages) != 2 || model.requests[1].Messages[0].Role != ModelRoleSystem || model.requests[1].Messages[1].Content != "hello" {
		t.Fatalf("rebuilt request=%#v", model.requests[1])
	}
}

func TestRuntimeKeepsOriginalHistoryWhenCompressionOutputIsInvalid(t *testing.T) {
	store := newMemoryStore()
	history := &compactionHistoryStub{snapshot: HistorySnapshot{Messages: []HistoryMessage{
		{ID: "m1", Role: ModelRoleUser, Content: strings.Repeat("old question ", 800)},
		{ID: "m2", Role: ModelRoleAssistant, Content: strings.Repeat("old answer ", 800)},
	}}}
	model := &streamSequence{turns: []Turn{{Content: "not json"}, {Content: "final"}}}
	_, err := (&Runtime{Model: model, History: history, Tools: capability.NewRegistry(), Lifecycle: NewLifecycle(store, sequentialIDs()), ContextTokenBudget: 100}).Run(context.Background(), validRunRequest(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(history.stored) != 0 || len(model.requests) != 2 {
		t.Fatalf("stored=%#v requests=%#v", history.stored, model.requests)
	}
	if len(model.requests[1].Messages) != 3 || model.requests[1].Messages[0].Content != history.snapshot.Messages[0].Content {
		t.Fatalf("original history was not retained: %#v", model.requests[1].Messages)
	}
}

func TestRuntimePersistsStreamDeltasOutsideModelHistory(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	tools := capability.NewRegistry()
	model := &streamSequence{turns: []Turn{{Thinking: "reason", Content: "final"}}}
	result, err := (&Runtime{Model: model, Tools: tools, Lifecycle: lifecycle}).Run(context.Background(), validRunRequest(), nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := store.entries(result.RuntimeSessionID)
	var sawThinking, sawAnswer, sawTurn bool
	for _, entry := range entries {
		sawThinking = sawThinking || entry.EventType == EventThinkingDelta
		sawAnswer = sawAnswer || entry.EventType == EventAnswerDelta
		sawTurn = sawTurn || entry.EventType == EventAssistantTurn
	}
	if !sawThinking || !sawAnswer || !sawTurn {
		t.Fatalf("journal does not contain stream and completed turn facts: %#v", entries)
	}
}

func TestRuntimeOnlyExposesToolsAllowedByEffectivePolicy(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	tools := capability.NewRegistry()
	if err := tools.Register(testCapability(capability.RetrieveKnowledgeID, nil)); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(testCapability(capability.WebSearchID, nil)); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(testCapability(capability.SearchConversationHistoryID, nil)); err != nil {
		t.Fatal(err)
	}
	if err := tools.Register(testCapability(capability.ArchiveConversationEpisodeID, nil)); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{{Content: "final"}}}
	request := validRunRequest()
	request.KnowledgeBaseIDs = nil
	request.Policy.AllowWebSearch = true
	_, err := (&Runtime{Model: model, Tools: tools, Lifecycle: lifecycle}).Run(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(model.requests) != 1 || len(model.requests[0].Tools) != 1 || model.requests[0].Tools[0].ID != capability.WebSearchID {
		t.Fatalf("model tools = %#v", model.requests)
	}
}

func TestRuntimeExpandsRetrievedChunkCitationsForStreamAndFinalAnswer(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	tools := capability.NewRegistry()
	if err := tools.Register(capability.RetrieveKnowledge(citationRetrieveStub{})); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{
		{ToolCalls: []ToolCall{{ID: "call-1", CapabilityID: capability.RetrieveKnowledgeID, Arguments: capability.Value(`{"query":"release"}`)}}},
		{Content: `Evidence found.<ref id="c1"/>` + "\n\n", ToolCalls: []ToolCall{{ID: "call-2", CapabilityID: capability.RetrieveKnowledgeID, Arguments: capability.Value(`{"query":"details"}`)}}},
		{Content: `The release is ready.<ref id="c1"/>`},
	}}
	sink := &journalSink{}
	request := validRunRequest()
	request.KnowledgeBaseIDs = []string{"kb-1"}
	request.Policy.AllowKnowledgeRetrieval = true
	result, err := (&Runtime{Model: model, Tools: tools, Lifecycle: NewLifecycle(store, sequentialIDs())}).Run(context.Background(), request, sink)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.AssistantContent, `<kb doc="Release notes" chunk_id="chunk-1" kb_id="kb-1" />`) {
		t.Fatalf("assistant content = %q", result.AssistantContent)
	}
	if len(model.requests) != 3 || !strings.Contains(model.requests[1].Messages[2].Content, `<chunk id="c1"`) {
		t.Fatalf("tool context = %#v", model.requests)
	}
	if !strings.Contains(strings.Join(model.requests[1].System, "\n"), `<ref id="cN"/>`) {
		t.Fatalf("citation protocol missing from system: %#v", model.requests[1].System)
	}
	var streamed string
	for _, entry := range sink.entries {
		if entry.EventType == EventAnswerDelta {
			streamed += entry.Detail
		}
	}
	if streamed != result.AssistantContent {
		t.Fatalf("streamed=%q final=%q", streamed, result.AssistantContent)
	}
}

func TestRuntimeAllowsOnlyOneEpisodeArchivePerUserMessage(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	episodes := &runtimeEpisodeStore{}
	tools := capability.NewRegistry()
	episodeService := capability.NewEpisodeService(episodes, runtimeEmbeddingStub{}, capability.EpisodeIDFactory(sequentialIDs()))
	if err := tools.Register(capability.ArchiveConversationEpisode(episodeService)); err != nil {
		t.Fatal(err)
	}
	model := &streamSequence{turns: []Turn{
		{ToolCalls: []ToolCall{{ID: "archive-1", CapabilityID: capability.ArchiveConversationEpisodeID, Arguments: capability.Value(`{"summary":"decision","topics":["release"],"importance":"normal"}`)}}},
		{Content: "final"},
	}}
	request := validRunRequest()
	request.Policy.AllowEpisodeArchive = true
	_, err := (&Runtime{Model: model, Tools: tools, Lifecycle: NewLifecycle(store, sequentialIDs()), Episodes: episodes}).Run(context.Background(), request, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes.created) != 1 {
		t.Fatalf("episodes=%#v", episodes.created)
	}
	if len(model.requests) != 2 || len(model.requests[1].Tools) != 0 {
		t.Fatalf("second-turn tools=%#v", model.requests[1].Tools)
	}
}

func TestRuntimeReplayOnlyReturnsTheTaskOwnersJournal(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	session, err := lifecycle.StartSession(context.Background(), RunRequest{ConversationID: "c", UserID: "u", UserMessageID: "m", Question: "q", TraceID: "task"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := lifecycle.RecordEvent(context.Background(), session, EventAnswerDelta, "answer"); err != nil {
		t.Fatal(err)
	}
	sink := &journalSink{}
	found, err := (&Runtime{Lifecycle: lifecycle}).Replay(context.Background(), "u", "task", sink)
	if err != nil || !found || len(sink.entries) != 1 || sink.entries[0].Detail != "answer" {
		t.Fatalf("found=%v entries=%#v err=%v", found, sink.entries, err)
	}
	found, err = (&Runtime{Lifecycle: lifecycle}).Replay(context.Background(), "other", "task", sink)
	if err != nil || found {
		t.Fatalf("cross-user replay found=%v err=%v", found, err)
	}
}

type journalSink struct{ entries []JournalEntry }

func (s *journalSink) Append(_ context.Context, entry JournalEntry) error {
	s.entries = append(s.entries, entry)
	return nil
}

func TestRuntimeRecordsExplicitCancellationAsCancelled(t *testing.T) {
	t.Parallel()
	store := newMemoryStore()
	lifecycle := NewLifecycle(store, sequentialIDs())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := (&Runtime{Model: cancelModel{}, Tools: capability.NewRegistry(), Lifecycle: lifecycle}).Run(ctx, validRunRequest(), nil)
	if !errors.Is(err, context.Canceled) || result.Status != StatusCancelled {
		t.Fatalf("result=%#v err=%v", result, err)
	}
	if store.sessions[result.RuntimeSessionID].Status != StatusCancelled {
		t.Fatalf("session status = %q", store.sessions[result.RuntimeSessionID].Status)
	}
}

type streamSequence struct {
	turns    []Turn
	requests []ModelRequest
}

type historyStub struct{ messages []ModelMessage }

type compactionHistoryStub struct {
	snapshot HistorySnapshot
	stored   []HistorySummary
}

type citationRetrieveStub struct{}

type runtimeEmbeddingStub struct{}

func (runtimeEmbeddingStub) Embed(string) ([]float32, error) { return []float32{.1}, nil }

type runtimeEpisodeStore struct{ created []persistence.Episode }

func (s *runtimeEpisodeStore) CreateEpisode(_ context.Context, episode persistence.Episode, _ []float32) error {
	s.created = append(s.created, episode)
	return nil
}
func (*runtimeEpisodeStore) CompleteEpisodes(context.Context, string, string) error { return nil }
func (*runtimeEpisodeStore) DiscardEpisodes(context.Context, string) error          { return nil }
func (*runtimeEpisodeStore) SearchEpisodes(context.Context, persistence.EpisodeSearch) ([]persistence.EpisodeHit, error) {
	return nil, nil
}

func (citationRetrieveStub) Retrieve(context.Context, ragretrieve.Request) (ragretrieve.Result, error) {
	return ragretrieve.Result{Chunks: []convention.RetrievedChunk{{
		ID: "chunk-1", Text: "The release is ready.", DocumentID: "doc-1", KnowledgeBaseID: "kb-1", Metadata: map[string]any{"document_name": "Release notes"},
	}}}, nil
}

func (citationRetrieveStub) RetrieveByVector(context.Context, []float32, ragretrieve.Request) (ragretrieve.Result, error) {
	return ragretrieve.Result{}, nil
}

func (s historyStub) Messages(context.Context, RunRequest) ([]ModelMessage, error) {
	return s.messages, nil
}

func (s *compactionHistoryStub) Messages(_ context.Context, _ RunRequest) ([]ModelMessage, error) {
	result := make([]ModelMessage, 0, len(s.snapshot.Messages)+1)
	if s.snapshot.Summary.Content != "" {
		result = append(result, ModelMessage{Role: ModelRoleSystem, Content: "Conversation summary:\n" + s.snapshot.Summary.Content})
	}
	for _, message := range s.snapshot.Messages {
		result = append(result, ModelMessage{Role: message.Role, Content: message.Content})
	}
	return result, nil
}

func (s *compactionHistoryStub) Snapshot(context.Context, RunRequest) (HistorySnapshot, error) {
	return s.snapshot, nil
}

func (s *compactionHistoryStub) StoreSummary(_ context.Context, _ RunRequest, summary HistorySummary) (bool, error) {
	s.stored = append(s.stored, summary)
	s.snapshot.Summary = summary
	for index, message := range s.snapshot.Messages {
		if message.ID == summary.CoveredToMessageID {
			s.snapshot.Messages = append([]HistoryMessage(nil), s.snapshot.Messages[index+1:]...)
			break
		}
	}
	return true, nil
}

type cancelModel struct{}

func (cancelModel) Stream(ctx context.Context, _ ModelRequest, _ func(ModelEvent) error) (Turn, error) {
	return Turn{}, ctx.Err()
}

func (s *streamSequence) Stream(_ context.Context, request ModelRequest, emit func(ModelEvent) error) (Turn, error) {
	s.requests = append(s.requests, request)
	turn := s.turns[len(s.requests)-1]
	if turn.Thinking != "" {
		_ = emit(ModelEvent{Kind: ModelEventThinking, Text: turn.Thinking})
	}
	if turn.Content != "" {
		_ = emit(ModelEvent{Kind: ModelEventContent, Text: turn.Content})
	}
	return turn, nil
}
