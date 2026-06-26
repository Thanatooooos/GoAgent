package state

import (
	"testing"
	"time"
)

func TestDefaultReducerApply_BackfillsSnapshotVersionAndPatternState(t *testing.T) {
	reducer := DefaultReducer{}

	next, err := reducer.Apply(StateSnapshot{}, StateDelta{})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if next.SchemaVersion != CurrentSnapshotVersion {
		t.Fatalf("expected schema version %d, got %d", CurrentSnapshotVersion, next.SchemaVersion)
	}

	next.Pattern.Name = "reactive"
	next.Pattern.Data = map[string]any{"iteration_mode": "observe"}
	cloned := CloneSnapshot(next)
	cloned.Pattern.Data["iteration_mode"] = "answer"

	if next.Pattern.Data["iteration_mode"] != "observe" {
		t.Fatalf("expected pattern data clone to avoid mutation, got %+v", next.Pattern.Data)
	}
}

func TestDefaultReducerApply_RequestConversationIDOnlyBackfills(t *testing.T) {
	reducer := DefaultReducer{}

	initial := StateSnapshot{
		Request: RequestState{
			Question:       "q",
			ConversationID: "conv-1",
		},
	}

	replacement := "conv-2"
	next, err := reducer.Apply(initial, StateDelta{
		Request: &RequestDelta{
			ConversationID: &replacement,
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if next.Request.ConversationID != "conv-1" {
		t.Fatalf("expected existing conversation id to be preserved, got %q", next.Request.ConversationID)
	}
}

func TestDefaultReducerApply_ContextOverwritesQueriesAndAppendsCollections(t *testing.T) {
	reducer := DefaultReducer{}

	initial := StateSnapshot{
		Context: ContextState{
			RewrittenQuery: "old rewrite",
			SearchQuery:    "old search",
			SearchResults: []SearchResultRef{
				{ID: "s1", Title: "existing"},
			},
			SeenURLs: []string{"https://existing.example"},
			Notes:    []string{"existing-note"},
		},
	}

	rewritten := "new rewrite"
	searchQuery := "new search"
	next, err := reducer.Apply(initial, StateDelta{
		Context: &ContextDelta{
			RewrittenQuery: &rewritten,
			SearchQuery:    &searchQuery,
			SearchResults: []SearchResultRef{
				{ID: "s2", Title: "new"},
			},
			SeenURLs: []string{"https://existing.example", "https://new.example"},
			Notes:    []string{"new-note"},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if next.Context.RewrittenQuery != "new rewrite" {
		t.Fatalf("expected rewritten query overwrite, got %q", next.Context.RewrittenQuery)
	}
	if next.Context.SearchQuery != "new search" {
		t.Fatalf("expected search query overwrite, got %q", next.Context.SearchQuery)
	}
	if len(next.Context.SearchResults) != 2 {
		t.Fatalf("expected appended search results, got %d", len(next.Context.SearchResults))
	}
	if len(next.Context.SeenURLs) != 2 {
		t.Fatalf("expected unique seen urls, got %+v", next.Context.SeenURLs)
	}
	if len(next.Context.Notes) != 2 {
		t.Fatalf("expected appended notes, got %d", len(next.Context.Notes))
	}
}

func TestDefaultReducerApply_EvidenceAppendsAndOverwritesSufficiency(t *testing.T) {
	reducer := DefaultReducer{}

	initial := StateSnapshot{
		Evidence: EvidenceState{
			Items:      []EvidenceItem{{ID: "e1", Content: "existing"}},
			Sufficient: false,
		},
	}

	sufficient := true
	reason := "enough evidence"
	newItemsThisRound := 1
	next, err := reducer.Apply(initial, StateDelta{
		Evidence: &EvidenceDelta{
			AddItems: []EvidenceItem{
				{ID: "e2", Content: "new"},
			},
			Sufficient:        &sufficient,
			SufficiencyReason: &reason,
			NewItemsThisRound: &newItemsThisRound,
			OpenQuestions:     []string{"what changed"},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if len(next.Evidence.Items) != 2 {
		t.Fatalf("expected appended evidence items, got %d", len(next.Evidence.Items))
	}
	if !next.Evidence.Sufficient {
		t.Fatal("expected sufficiency to be overwritten to true")
	}
	if next.Evidence.SufficiencyReason != "enough evidence" {
		t.Fatalf("expected sufficiency reason to update, got %q", next.Evidence.SufficiencyReason)
	}
	if next.Evidence.NewItemsThisRound != 1 {
		t.Fatalf("expected new-items-this-round update, got %d", next.Evidence.NewItemsThisRound)
	}
	if len(next.Evidence.OpenQuestions) != 1 {
		t.Fatalf("expected open questions append, got %d", len(next.Evidence.OpenQuestions))
	}
}

func TestDefaultReducerApply_EvidenceDeduplicatesItemsAndOpenQuestions(t *testing.T) {
	reducer := DefaultReducer{}

	initial := StateSnapshot{
		Evidence: EvidenceState{
			Items: []EvidenceItem{
				{ID: "e1", SourceRef: "url-1", Content: "same"},
			},
			OpenQuestions: []string{"what changed"},
		},
	}

	next, err := reducer.Apply(initial, StateDelta{
		Evidence: &EvidenceDelta{
			AddItems: []EvidenceItem{
				{ID: "e1", SourceRef: "url-1", Content: "same"},
				{ID: "e2", SourceRef: "url-2", Content: "new"},
			},
			OpenQuestions: []string{"what changed", "what evidence is missing"},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if len(next.Evidence.Items) != 2 {
		t.Fatalf("expected deduplicated evidence items, got %+v", next.Evidence.Items)
	}
	if len(next.Evidence.OpenQuestions) != 2 {
		t.Fatalf("expected deduplicated open questions, got %+v", next.Evidence.OpenQuestions)
	}
}

func TestDefaultReducerApply_ExecutionAccumulatesAndAnswerOverwrites(t *testing.T) {
	reducer := DefaultReducer{}

	initial := StateSnapshot{
		Execution: ExecutionState{
			Iteration:        1,
			ScheduledActions: []string{"search"},
		},
		Answer: AnswerState{
			Draft: "draft-1",
		},
	}

	currentNode := "answer"
	interrupted := true
	interruptReason := "approval_required"
	finalAnswer := "final-answer"
	continueCountIncrement := 1
	lastBranchTarget := "continue"
	lastBranchReason := "need_more_sources"
	lastProgressKind := "progress_new_sources_found"
	lastNewURLCount := 2
	lastNewEvidenceCount := 0
	noProgressRounds := 0
	next, err := reducer.Apply(initial, StateDelta{
		Execution: &ExecutionDelta{
			CurrentNode:                 &currentNode,
			IterationIncrement:          1,
			ContinueCountIncrement:      continueCountIncrement,
			LastBranchTarget:            &lastBranchTarget,
			LastBranchReason:            &lastBranchReason,
			LastProgressKind:            &lastProgressKind,
			LastNewURLCount:             &lastNewURLCount,
			LastNewEvidenceCount:        &lastNewEvidenceCount,
			ConsecutiveNoProgressRounds: &noProgressRounds,
			CompletedActions:            []string{"search"},
			Interrupted:                 &interrupted,
			InterruptReason:             &interruptReason,
		},
		Answer: &AnswerDelta{
			Final: &finalAnswer,
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if next.Execution.CurrentNode != "answer" {
		t.Fatalf("expected current node overwrite, got %q", next.Execution.CurrentNode)
	}
	if next.Execution.Iteration != 2 {
		t.Fatalf("expected iteration increment to 2, got %d", next.Execution.Iteration)
	}
	if next.Execution.ContinueCount != 1 {
		t.Fatalf("expected continue count increment, got %d", next.Execution.ContinueCount)
	}
	if next.Execution.LastBranchTarget != "continue" || next.Execution.LastBranchReason != "need_more_sources" {
		t.Fatalf("expected branch tracking update, got %+v", next.Execution)
	}
	if next.Execution.LastProgressKind != "progress_new_sources_found" {
		t.Fatalf("expected progress kind update, got %+v", next.Execution)
	}
	if next.Execution.LastNewURLCount != 2 || next.Execution.LastNewEvidenceCount != 0 {
		t.Fatalf("expected progress counters update, got %+v", next.Execution)
	}
	if next.Execution.ConsecutiveNoProgressRounds != 0 {
		t.Fatalf("expected no-progress rounds update, got %+v", next.Execution)
	}
	if len(next.Execution.CompletedActions) != 1 {
		t.Fatalf("expected completed action append, got %d", len(next.Execution.CompletedActions))
	}
	if !next.Execution.Interrupted {
		t.Fatal("expected interrupted to be true")
	}
	if next.Execution.Status != ExecutionStatusInterrupted {
		t.Fatalf("expected interrupted execution status, got %q", next.Execution.Status)
	}
	if next.Execution.InterruptReason != "approval_required" {
		t.Fatalf("expected interrupt reason update, got %q", next.Execution.InterruptReason)
	}
	if next.Answer.Final != "final-answer" {
		t.Fatalf("expected final answer overwrite, got %q", next.Answer.Final)
	}
	if next.Answer.Draft != "" {
		t.Fatalf("expected final answer to clear draft, got %q", next.Answer.Draft)
	}
	if next.Execution.Status != ExecutionStatusInterrupted {
		t.Fatalf("expected final answer to keep interrupted execution status until interruption clears, got %q", next.Execution.Status)
	}
}

func TestDefaultReducerApply_ExecutionActionListsRemainUnique(t *testing.T) {
	reducer := DefaultReducer{}

	initial := StateSnapshot{
		Execution: ExecutionState{
			ScheduledActions: []string{"search"},
			CompletedActions: []string{"prepare"},
			FailedActions:    []string{"fetch"},
		},
	}

	next, err := reducer.Apply(initial, StateDelta{
		Execution: &ExecutionDelta{
			ScheduledActions: []string{"search", "fetch"},
			CompletedActions: []string{"prepare", "search"},
			FailedActions:    []string{"fetch", "answer"},
		},
	})
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}

	if len(next.Execution.ScheduledActions) != 2 {
		t.Fatalf("expected unique scheduled actions, got %+v", next.Execution.ScheduledActions)
	}
	if len(next.Execution.CompletedActions) != 2 {
		t.Fatalf("expected unique completed actions, got %+v", next.Execution.CompletedActions)
	}
	if len(next.Execution.FailedActions) != 2 {
		t.Fatalf("expected unique failed actions, got %+v", next.Execution.FailedActions)
	}
}

func TestDefaultReducerApply_RejectsPendingApprovalThatAlreadyHasReviewTimestamp(t *testing.T) {
	reducer := DefaultReducer{}
	reviewedAt := testTime()
	status := ApprovalStatusPending

	_, err := reducer.Apply(StateSnapshot{}, StateDelta{
		Approval: &ApprovalDelta{
			Status:     &status,
			ReviewedAt: &reviewedAt,
		},
	})
	if err == nil {
		t.Fatal("expected pending approval with review timestamp to fail")
	}
}

func TestDefaultReducerApply_RejectsClearedInterruptWithReasonStillSet(t *testing.T) {
	reducer := DefaultReducer{}
	interrupted := false
	reason := "awaiting approval"

	_, err := reducer.Apply(StateSnapshot{}, StateDelta{
		Execution: &ExecutionDelta{
			Interrupted:     &interrupted,
			InterruptReason: &reason,
		},
	})
	if err == nil {
		t.Fatal("expected cleared interrupt with reason to fail")
	}
}

func TestDefaultReducerApply_RejectsFinalAnswerThatStillCarriesDraft(t *testing.T) {
	reducer := DefaultReducer{}

	_, err := reducer.Apply(StateSnapshot{
		Answer: AnswerState{
			Draft: "draft",
			Final: "answer",
		},
	}, StateDelta{})
	if err == nil {
		t.Fatal("expected final answer with draft to fail")
	}
}

func TestDefaultReducerApply_RejectsUnsupportedFutureSnapshotVersion(t *testing.T) {
	reducer := DefaultReducer{}

	_, err := reducer.Apply(StateSnapshot{
		SchemaVersion: CurrentSnapshotVersion + 1,
	}, StateDelta{})
	if err == nil {
		t.Fatal("expected unsupported future snapshot version to fail")
	}
}

func testTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
