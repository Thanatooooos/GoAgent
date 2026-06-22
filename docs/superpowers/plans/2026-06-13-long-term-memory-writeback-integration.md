# Long-Term Memory Writeback Integration Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Connect the Phase 1 preference extraction pipeline to the chat success path so user messages can asynchronously produce `pending` preference candidates without affecting the main response.

**Architecture:** Keep the existing read-path unchanged and add a separate writeback dependency to `RagChatService`. The new writeback runner will orchestrate `pre-filter -> structured extraction -> post-filter -> TryPersistPendingPreferenceCandidate` inside a fail-open async hook triggered only after a successful chat response has been persisted.

**Tech Stack:** Go, existing `RagChatService`, `longtermmemory/extraction`, `PreferenceCandidateLifecycleService`, existing logs/metrics hooks, Go test.

---

## File Structure

- Create: `internal/app/rag/service/longtermmemory/writeback/service.go`
  - Own the chat-facing writeback orchestration interface and pipeline runner.
- Create: `internal/app/rag/service/longtermmemory/writeback/service_test.go`
  - Unit tests for trigger handling, extraction outcomes, post-filter rejection, persistence, and fail-open semantics.
- Modify: `internal/app/rag/service/chat/deps.go`
  - Add an optional chat dependency for long-term memory writeback.
- Modify: `internal/app/rag/service/chat/execute_orchestrator.go`
  - Trigger async writeback after assistant response persistence succeeds.
- Modify: `internal/app/rag/service/chat/rag_chat_service_test.go`
  - Add focused chat tests that verify writeback is triggered on success and does not break chat on failure.
- Modify: `internal/bootstrap/rag/runtime_build_chat.go`
  - Wire the writeback runner into the chat service using existing AI chat + lifecycle services.

### Task 1: Add a Chat Writeback Dependency Surface

**Files:**
- Modify: `internal/app/rag/service/chat/deps.go`
- Modify: `internal/app/rag/service/chat/rag_chat_service_test.go`
- Test: `internal/app/rag/service/chat/rag_chat_service_test.go`

- [ ] **Step 1: Write the failing chat dependency test**

```go
func TestHandleSucceededResultTriggersLongTermMemoryWriteback(t *testing.T) {
	writeback := &longTermMemoryWritebackStub{}
	service, _ := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "以后默认用中文回答", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryWriteback = writeback
		},
	)

	sink := &fallbackSinkStub{}
	err := service.handleSucceededResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: "以后默认用中文回答"},
		ragChatRuntimeState{
			meta:    RagChatMeta{ConversationID: "conv-1", TaskID: "task-1"},
			title:   "title",
			traceID: "trace-1",
		},
		ragChatTaskResult{content: "answer"},
		sink,
	)
	if err != nil {
		t.Fatalf("handleSucceededResult returned error: %v", err)
	}
	if writeback.calls != 1 {
		t.Fatalf("expected writeback to be triggered once, got %d", writeback.calls)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/rag/service/chat -run TestHandleSucceededResultTriggersLongTermMemoryWriteback`

Expected: FAIL because `RagChatOptions`/`RagChatService` do not yet expose a writeback dependency.

- [ ] **Step 3: Add the minimal dependency surface**

```go
type LongTermMemoryWriteback interface {
	CapturePreferenceCandidate(ctx context.Context, input LongTermMemoryWritebackInput)
}

type RagChatOptions struct {
	// existing fields...
	LongTermMemoryWriteback LongTermMemoryWriteback
}

type RagChatService struct {
	// existing fields...
	longTermMemoryWriteback LongTermMemoryWriteback
}
```

- [ ] **Step 4: Wire the option through construction**

```go
service.longTermMemoryWriteback = opts.LongTermMemoryWriteback
```

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/app/rag/service/chat -run TestHandleSucceededResultTriggersLongTermMemoryWriteback`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/rag/service/chat/deps.go internal/app/rag/service/chat/rag_chat_service_test.go
git commit -m "test: add chat writeback dependency surface"
```

### Task 2: Implement the Writeback Pipeline Runner

**Files:**
- Create: `internal/app/rag/service/longtermmemory/writeback/service.go`
- Create: `internal/app/rag/service/longtermmemory/writeback/service_test.go`
- Test: `internal/app/rag/service/longtermmemory/writeback/service_test.go`

- [ ] **Step 1: Write the failing pipeline tests**

```go
func TestServiceCapturePreferenceCandidatePersistsPendingCandidate(t *testing.T) {
	repo := newPreferenceCandidateLifecycleRepoStub()
	repo.createFn = func(_ context.Context, item domain.MemoryItem) (domain.MemoryItem, error) {
		item.ID = "cand-1"
		return item, nil
	}

	lifecycle := longtermmemory.NewPreferenceCandidateLifecycleService(newMemoryServiceForWritebackTest(repo))
	writer := NewService(
		extraction.NewObservedLLMPreferenceExtractor(&stubLLMService{
			response: `{"scope_type":"global","memory_type":"preference","canonical_key":"response.language","summary":"以后默认用中文回答","content":"以后默认用中文回答","confidence":0.94}`,
		}, nil),
		lifecycle,
		nil,
	)

	result := writer.capturePreferenceCandidateSync(context.Background(), LongTermMemoryWritebackInput{
		UserID:          "user-1",
		Message:         "以后默认用中文回答",
		SourceMessageID: "msg-1",
	})
	if !result.Persisted {
		t.Fatalf("expected persisted candidate, got %+v", result)
	}
}
```

```go
func TestServiceCapturePreferenceCandidateSkipsOneOffWithoutTrigger(t *testing.T) {
	writer := NewService(nil, nil, nil)

	result := writer.capturePreferenceCandidateSync(context.Background(), LongTermMemoryWritebackInput{
		UserID:          "user-1",
		Message:         "帮我计算 17*23 等于多少",
		SourceMessageID: "msg-1",
	})
	if !result.Skipped || result.SkipReason != extraction.SkipReasonCalculation {
		t.Fatalf("expected calculation skip, got %+v", result)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/app/rag/service/longtermmemory/writeback -run "TestServiceCapturePreferenceCandidate"`

Expected: FAIL because the writeback package does not exist yet.

- [ ] **Step 3: Add the minimal runner and input/result types**

```go
type LifecyclePersister interface {
	TryPersistPendingPreferenceCandidate(ctx context.Context, input longtermmemory.PersistPreferenceCandidateInput) (longtermmemory.PreferenceCandidate, bool)
}

type Extractor interface {
	Extract(input extraction.ExtractInput) extraction.ExtractResult
}

type Service struct {
	extractor Extractor
	persister LifecyclePersister
	metrics   *cachemetrics.Service
}
```

- [ ] **Step 4: Implement the synchronous pipeline helper**

```go
func (s *Service) capturePreferenceCandidateSync(ctx context.Context, input LongTermMemoryWritebackInput) CaptureResult {
	pre := extraction.EvaluateObservedPreFilter(s.metrics, extraction.PreFilterInput{Message: input.Message})
	if pre.Skip {
		return CaptureResult{Skipped: true, SkipReason: pre.SkipReason}
	}

	extracted := s.extractor.Extract(extraction.ExtractInput{Message: input.Message})
	if extracted.Candidate == nil || extracted.Rejected || extracted.Failed {
		return CaptureResult{Failed: extracted.Failed, Rejected: extracted.Rejected}
	}

	post := extraction.ApplyObservedPreferencePostFilter(s.metrics, *extracted.Candidate)
	if post.Candidate == nil || post.Rejected {
		return CaptureResult{Rejected: post.Rejected}
	}

	_, ok := s.persister.TryPersistPendingPreferenceCandidate(ctx, longtermmemory.PersistPreferenceCandidateInput{
		UserID: input.UserID,
		Candidate: longtermmemory.PreferenceCandidate{
			ScopeType:        post.Candidate.ScopeType,
			MemoryType:       post.Candidate.MemoryType,
			CanonicalKey:     post.Candidate.CanonicalKey,
			Summary:          post.Candidate.Summary,
			Content:          post.Candidate.Content,
			SourceMessageID:  input.SourceMessageID,
			ExtractionMethod: domain.MemoryExtractionMethodLLM,
			Confidence:       post.Candidate.Confidence,
		},
	})
	return CaptureResult{Persisted: ok}
}
```

- [ ] **Step 5: Add fail-open async entrypoint**

```go
func (s *Service) CapturePreferenceCandidate(ctx context.Context, input LongTermMemoryWritebackInput) {
	go func() {
		result := s.capturePreferenceCandidateSync(ctx, input)
		log.FromContext(ctx).Infow("long-term memory writeback finished", "persisted", result.Persisted, "skipped", result.Skipped, "skip_reason", result.SkipReason)
	}()
}
```

- [ ] **Step 6: Run the package tests to verify they pass**

Run: `go test ./internal/app/rag/service/longtermmemory/writeback`

Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/app/rag/service/longtermmemory/writeback/service.go internal/app/rag/service/longtermmemory/writeback/service_test.go
git commit -m "feat: add long-term memory writeback pipeline"
```

### Task 3: Trigger Writeback from the Chat Success Path

**Files:**
- Modify: `internal/app/rag/service/chat/execute_orchestrator.go`
- Modify: `internal/app/rag/service/chat/rag_chat_service_test.go`
- Test: `internal/app/rag/service/chat/rag_chat_service_test.go`

- [ ] **Step 1: Write the failing fail-open chat test**

```go
func TestHandleSucceededResultWritebackFailureDoesNotBreakChat(t *testing.T) {
	writeback := &longTermMemoryWritebackStub{panicOnCall: true}
	service, createdMessage := newPrepareChatTestService(
		t,
		ragrewrite.Result{RewrittenQuestion: "以后默认用中文回答", NeedRetrieval: false},
		nil,
		&retrieveServiceStub{},
		func(_ *RagChatDeps, opts *RagChatOptions) {
			opts.LongTermMemoryWriteback = writeback
		},
	)

	sink := &fallbackSinkStub{}
	err := service.handleSucceededResult(
		context.Background(),
		RagChatInput{ConversationID: "conv-1", UserID: "user-1", Question: "以后默认用中文回答"},
		ragChatRuntimeState{meta: RagChatMeta{ConversationID: "conv-1", TaskID: "task-1"}, title: "title", traceID: "trace-1"},
		ragChatTaskResult{content: "answer"},
		sink,
	)
	if err != nil {
		t.Fatalf("expected chat success despite writeback failure, got %v", err)
	}
	if createdMessage == nil || sink.finishCalls != 1 {
		t.Fatalf("expected normal success path, got message=%+v finish=%d", createdMessage, sink.finishCalls)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/app/rag/service/chat -run TestHandleSucceededResultWritebackFailureDoesNotBreakChat`

Expected: FAIL because `handleSucceededResult()` does not invoke writeback yet.

- [ ] **Step 3: Trigger the async writeback after assistant persistence**

```go
if s.longTermMemoryWriteback != nil {
	s.longTermMemoryWriteback.CapturePreferenceCandidate(ctx, LongTermMemoryWritebackInput{
		UserID:          strings.TrimSpace(input.UserID),
		Message:         strings.TrimSpace(input.Question),
		SourceMessageID: state.userMessageID,
	})
}
```

- [ ] **Step 4: Guard the hook so it never breaks the success path**

```go
func safeStartLongTermMemoryWriteback(ctx context.Context, writer LongTermMemoryWriteback, input LongTermMemoryWritebackInput) {
	defer func() {
		if recovered := recover(); recovered != nil {
			log.FromContext(ctx).Warnw("rag chat long-term memory writeback panicked", "recovered", recovered)
		}
	}()
	writer.CapturePreferenceCandidate(ctx, input)
}
```

- [ ] **Step 5: Run the focused chat tests**

Run: `go test ./internal/app/rag/service/chat -run "TestHandleSucceededResult(TriggersLongTermMemoryWriteback|WritebackFailureDoesNotBreakChat)"`

Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/app/rag/service/chat/execute_orchestrator.go internal/app/rag/service/chat/rag_chat_service_test.go
git commit -m "feat: trigger long-term memory writeback after chat success"
```

### Task 4: Wire Runtime Construction and Verify Package-Level Integration

**Files:**
- Modify: `internal/bootstrap/rag/runtime_build_chat.go`
- Modify: `internal/app/rag/service/chat/deps.go`
- Test: `internal/app/rag/service/chat/rag_chat_service_test.go`
- Test: `internal/app/rag/service/longtermmemory/writeback/service_test.go`

- [ ] **Step 1: Write the failing runtime wiring test**

```go
func TestNewRagChatServiceWithDepsAppliesWritebackOption(t *testing.T) {
	tracer := NewChatTracer(nil, nil)
	writeback := &longTermMemoryWritebackStub{}
	service, err := NewRagChatServiceWithDeps(minimalDeps(tracer), RagChatOptions{
		LongTermMemoryWriteback: writeback,
	})
	if err != nil {
		t.Fatalf("NewRagChatServiceWithDeps returned error: %v", err)
	}
	if service.longTermMemoryWriteback != writeback {
		t.Fatal("expected writeback dependency to be assigned")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails if wiring is incomplete**

Run: `go test ./internal/app/rag/service/chat -run TestNewRagChatServiceWithDepsAppliesWritebackOption`

Expected: FAIL until option wiring is complete.

- [ ] **Step 3: Build the runtime writeback runner using existing services**

```go
writeback := writeback.NewService(
	extraction.NewObservedLLMPreferenceExtractor(aiRuntime.Chat, memory.memoryCacheMetrics),
	longtermmemory.NewPreferenceCandidateLifecycleService(memory.explicitMemoryService),
	memory.memoryCacheMetrics,
)
```

- [ ] **Step 4: Pass it into the chat service options**

```go
ragservice.RagChatOptions{
	// existing options...
	LongTermMemoryWriteback: writeback,
}
```

- [ ] **Step 5: Run the end-of-task verification commands**

Run: `go test ./internal/app/rag/service/longtermmemory/...`
Expected: PASS

Run: `go test ./internal/app/rag/service/chat`
Expected: PASS

Run: `go test ./internal/bootstrap/rag/...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/bootstrap/rag/runtime_build_chat.go internal/app/rag/service/chat/deps.go internal/app/rag/service/chat/rag_chat_service_test.go
git commit -m "feat: wire long-term memory writeback into chat runtime"
```

## Self-Review

- Spec coverage:
  - Covers the missing chat write path from current user message to `pending` preference candidate.
  - Preserves existing fail-open behavior and does not alter recall semantics.
  - Leaves confirm/reject UI, agent writeback, and V2 end-to-end validation out of scope.
- Placeholder scan:
  - No `TODO`/`TBD` placeholders remain.
  - Every task lists exact files, tests, commands, and expected outcomes.
- Type consistency:
  - Uses a single chat-facing interface name: `LongTermMemoryWriteback`.
  - Uses a single input type name: `LongTermMemoryWritebackInput`.
  - Uses the existing lifecycle entrypoint: `TryPersistPendingPreferenceCandidate`.
