package runtime

import (
	"testing"
	"time"

	agentstate "local/rag-project/internal/app/agent/state"
)

func TestBuildReplayView(t *testing.T) {
	now := time.Now()
	searchQuery := "what happened rewritten"
	sufficient := true
	reason := "fetched_readable_evidence"
	finalAnswer := "final answer"
	lastBranchTarget := "answer"
	lastBranchReason := "fetched_readable_evidence"
	lastProgressKind := "progress_evidence_gained"
	lastNewURLCount := 1
	lastNewEvidenceCount := 1
	newItemsThisRound := 1
	noProgressRounds := 0

	session := &RuntimeSession{
		SessionID: "sess-replay",
		Request: RequestEnvelope{
			Question: "what happened",
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "what happened",
			},
			Evidence: agentstate.EvidenceState{
				Sufficient:        true,
				SufficiencyReason: "fetched_readable_evidence",
				NewItemsThisRound: 1,
			},
			Execution: agentstate.ExecutionState{
				ContinueCount:               1,
				LastBranchTarget:            "answer",
				LastBranchReason:            "fetched_readable_evidence",
				LastProgressKind:            "progress_evidence_gained",
				LastNewURLCount:             1,
				LastNewEvidenceCount:        1,
				ConsecutiveNoProgressRounds: 0,
			},
			Answer: agentstate.AnswerState{
				Final: "final answer",
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID: "sess-replay",
				Sequence:  1,
				Node:      "prepare",
				EventType: agentstate.EventTypeNodeStart,
				Timestamp: now,
			},
			{
				SessionID: "sess-replay",
				Sequence:  2,
				Node:      "prepare",
				EventType: agentstate.EventTypeStateApplied,
				Timestamp: now.Add(1 * time.Second),
				Delta: &agentstate.StateDelta{
					Context: &agentstate.ContextDelta{
						SearchQuery: &searchQuery,
					},
				},
			},
			{
				SessionID: "sess-replay",
				Sequence:  3,
				Node:      "prepare",
				EventType: agentstate.EventTypeNodeFinish,
				Timestamp: now.Add(2 * time.Second),
			},
			{
				SessionID:   "sess-replay",
				Sequence:    4,
				Node:        "search",
				EventType:   agentstate.EventTypeCapabilityStart,
				Timestamp:   now.Add(3 * time.Second),
				PayloadText: "what happened rewritten",
			},
			{
				SessionID:   "sess-replay",
				Sequence:    5,
				Node:        "search",
				EventType:   agentstate.EventTypeCapabilityResult,
				Timestamp:   now.Add(4 * time.Second),
				PayloadText: "found 1 result",
			},
			{
				SessionID:   "sess-replay",
				Sequence:    6,
				Node:        "observe",
				EventType:   agentstate.EventTypeDecisionEmitted,
				Timestamp:   now.Add(5 * time.Second),
				PayloadText: "kind=branch target=answer confidence=0.90 reasoning=fetched_readable_evidence",
				Decision: &agentstate.DecisionRef{
					Kind:       "branch",
					Target:     "answer",
					Confidence: 0.90,
					Reasoning:  "fetched_readable_evidence",
				},
			},
			{
				SessionID:   "sess-replay",
				Sequence:    7,
				Node:        "observe",
				EventType:   agentstate.EventTypeBranchSelected,
				Timestamp:   now.Add(6 * time.Second),
				PayloadText: "answer",
			},
			{
				SessionID: "sess-replay",
				Sequence:  8,
				Node:      "observe",
				EventType: agentstate.EventTypeStateApplied,
				Timestamp: now.Add(7 * time.Second),
				Delta: &agentstate.StateDelta{
					Evidence: &agentstate.EvidenceDelta{
						Sufficient:        &sufficient,
						SufficiencyReason: &reason,
						NewItemsThisRound: &newItemsThisRound,
					},
					Execution: &agentstate.ExecutionDelta{
						ContinueCountIncrement:      1,
						LastBranchTarget:            &lastBranchTarget,
						LastBranchReason:            &lastBranchReason,
						LastProgressKind:            &lastProgressKind,
						LastNewURLCount:             &lastNewURLCount,
						LastNewEvidenceCount:        &lastNewEvidenceCount,
						ConsecutiveNoProgressRounds: &noProgressRounds,
					},
				},
			},
			{
				SessionID: "sess-replay",
				Sequence:  9,
				Node:      "observe",
				EventType: agentstate.EventTypeNodeFinish,
				Timestamp: now.Add(8 * time.Second),
			},
			{
				SessionID:   "sess-replay",
				Sequence:    10,
				Node:        "answer",
				EventType:   agentstate.EventTypeInterrupt,
				Timestamp:   now.Add(9 * time.Second),
				PayloadText: "before_nodes=answer",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-1",
					Node: "answer",
				},
			},
			{
				SessionID:   "sess-replay",
				Sequence:    11,
				Node:        "answer",
				EventType:   agentstate.EventTypeResumeCompleted,
				Timestamp:   now.Add(10 * time.Second),
				PayloadText: "checkpoint_id=cp-1",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-1",
					Node: "answer",
				},
			},
			{
				SessionID: "sess-replay",
				Sequence:  12,
				Node:      "answer",
				EventType: agentstate.EventTypeNodeStart,
				Timestamp: now.Add(11 * time.Second),
			},
			{
				SessionID:   "sess-replay",
				Sequence:    13,
				Node:        "answer",
				EventType:   agentstate.EventTypeAnswerFinalized,
				Timestamp:   now.Add(12 * time.Second),
				PayloadText: "final answer",
			},
			{
				SessionID: "sess-replay",
				Sequence:  14,
				Node:      "answer",
				EventType: agentstate.EventTypeStateApplied,
				Timestamp: now.Add(13 * time.Second),
				Delta: &agentstate.StateDelta{
					Answer: &agentstate.AnswerDelta{
						Final: &finalAnswer,
					},
				},
			},
			{
				SessionID: "sess-replay",
				Sequence:  15,
				Node:      "answer",
				EventType: agentstate.EventTypeNodeFinish,
				Timestamp: now.Add(14 * time.Second),
			},
		},
		Checkpoint: &CheckpointRef{
			ID:          "cp-1",
			Node:        "answer",
			EventOffset: 10,
			CreatedAt:   now.Add(9 * time.Second),
		},
		Metadata: SessionMetadata{
			UpdatedAt:      now.Add(15 * time.Second),
			ResumedFrom:    "cp-1",
			ResumeCount:    1,
			RuntimeName:    "agent_runtime_m1",
			RuntimeVersion: "v1",
		},
	}

	view := BuildReplayView(session)

	if view.SessionID != "sess-replay" {
		t.Fatalf("expected session id, got %q", view.SessionID)
	}
	if view.Question != "what happened" {
		t.Fatalf("expected question, got %q", view.Question)
	}
	if view.EventCount != 15 || view.LastSequence != 15 {
		t.Fatalf("expected 15 events and last sequence 15, got count=%d last=%d", view.EventCount, view.LastSequence)
	}
	if len(view.Timeline) != 15 {
		t.Fatalf("expected 15 timeline events, got %d", len(view.Timeline))
	}
	if view.Timeline[1].Summary != "applied context.search_query" {
		t.Fatalf("expected state_applied summary, got %+v", view.Timeline[1])
	}
	if view.Timeline[6].Summary != "selected answer" {
		t.Fatalf("expected branch summary, got %+v", view.Timeline[6])
	}
	if view.EventTypeCounts[agentstate.EventTypeStateApplied] != 3 {
		t.Fatalf("expected 3 state_applied events, got %+v", view.EventTypeCounts)
	}
	if view.EventTypeCounts[agentstate.EventTypeCapabilityStart] != 1 {
		t.Fatalf("expected capability_start count, got %+v", view.EventTypeCounts)
	}
	if len(view.Nodes) != 4 {
		t.Fatalf("expected 4 node views, got %d", len(view.Nodes))
	}
	if view.Nodes[0].Node != "prepare" || view.Nodes[0].Status != "completed" {
		t.Fatalf("expected completed prepare node, got %+v", view.Nodes[0])
	}
	if view.Nodes[2].Node != "answer" || view.Nodes[2].Status != "interrupted" {
		t.Fatalf("expected interrupted answer node view, got %+v", view.Nodes[2])
	}
	if view.Nodes[3].Node != "answer" || view.Nodes[3].Status != "completed" {
		t.Fatalf("expected completed resumed answer node view, got %+v", view.Nodes[3])
	}
	if len(view.Capabilities) != 1 {
		t.Fatalf("expected 1 capability summary, got %d", len(view.Capabilities))
	}
	if view.Capabilities[0].Node != "search" || view.Capabilities[0].Status != "completed" {
		t.Fatalf("expected completed search capability, got %+v", view.Capabilities[0])
	}
	if view.Capabilities[0].InputSummary != "what happened rewritten" || view.Capabilities[0].OutputSummary != "found 1 result" {
		t.Fatalf("expected capability summaries, got %+v", view.Capabilities[0])
	}
	if len(view.Branches) != 1 || view.Branches[0].Target != "answer" {
		t.Fatalf("expected branch summary, got %+v", view.Branches)
	}
	if len(view.Decisions) != 1 || view.LastDecision == nil {
		t.Fatalf("expected decision summaries, got decisions=%d last=%+v", len(view.Decisions), view.LastDecision)
	}
	if view.LastDecision.Kind != "branch" || view.LastDecision.Target != "answer" {
		t.Fatalf("expected branch last decision, got %+v", view.LastDecision)
	}
	if view.Checkpoint == nil || view.Checkpoint.ID != "cp-1" {
		t.Fatalf("expected checkpoint ref, got %+v", view.Checkpoint)
	}
	if view.CheckpointState.Status != "resumed" || view.CheckpointState.Active {
		t.Fatalf("expected resumed inactive checkpoint state, got %+v", view.CheckpointState)
	}
	if view.CheckpointState.LastInterruptAt.IsZero() || view.CheckpointState.LastResumeAt.IsZero() {
		t.Fatalf("expected checkpoint lifecycle timestamps, got %+v", view.CheckpointState)
	}
	if view.CheckpointState.ResumeCount != 1 || view.CheckpointState.ResumedFrom != "cp-1" {
		t.Fatalf("expected resume metadata in checkpoint state, got %+v", view.CheckpointState)
	}
	if view.FinalAnswer != "final answer" {
		t.Fatalf("expected final answer, got %q", view.FinalAnswer)
	}
	if view.ContinueCount != 1 || view.LastBranchTarget != "answer" || view.LastBranchReason != "fetched_readable_evidence" {
		t.Fatalf("expected branch/continue summary, got %+v", view)
	}
	if view.LastProgressKind != "progress_evidence_gained" {
		t.Fatalf("expected progress-kind summary, got %+v", view)
	}
	if view.LastNewURLCount != 1 || view.LastNewEvidenceCount != 1 || view.ConsecutiveNoProgressRounds != 0 {
		t.Fatalf("expected progress summary, got %+v", view)
	}
	if !view.EvidenceSufficient || view.EvidenceReason != "fetched_readable_evidence" {
		t.Fatalf("expected evidence summary, got sufficient=%v reason=%q", view.EvidenceSufficient, view.EvidenceReason)
	}
}

func TestBuildReplayView_WithSkippedCapabilityAndActiveCheckpoint(t *testing.T) {
	now := time.Now()
	session := &RuntimeSession{
		SessionID: "sess-skip",
		Request: RequestEnvelope{
			Question: "no results",
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID:   "sess-skip",
				Sequence:    1,
				Node:        "fetch",
				EventType:   agentstate.EventTypeCapabilitySkipped,
				Timestamp:   now,
				PayloadText: "web fetch skipped: no fetchable urls",
			},
			{
				SessionID:   "sess-skip",
				Sequence:    2,
				Node:        "answer",
				EventType:   agentstate.EventTypeInterrupt,
				Timestamp:   now.Add(1 * time.Second),
				PayloadText: "before_nodes=answer",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-skip",
					Node: "answer",
				},
			},
		},
		Checkpoint: &CheckpointRef{
			ID:          "cp-skip",
			Node:        "answer",
			EventOffset: 2,
			CreatedAt:   now.Add(1 * time.Second),
		},
		Metadata: SessionMetadata{
			UpdatedAt: now.Add(2 * time.Second),
		},
	}

	view := BuildReplayView(session)

	if len(view.Capabilities) != 1 || view.Capabilities[0].Status != "skipped" {
		t.Fatalf("expected skipped capability summary, got %+v", view.Capabilities)
	}
	if view.CheckpointState.Status != "interrupted" || !view.CheckpointState.Active {
		t.Fatalf("expected active interrupted checkpoint state, got %+v", view.CheckpointState)
	}
}

func TestBuildReplayView_ProjectsPendingApprovalFromSharedStateNotPatternState(t *testing.T) {
	now := time.Now()
	session := &RuntimeSession{
		SessionID: "sess-approval-replay",
		Request: RequestEnvelope{
			Question: "needs approval",
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "needs approval",
			},
			Approval: agentstate.ApprovalState{
				Status:       agentstate.ApprovalStatusPending,
				Reason:       "web_fetch_approval_required",
				Capability:   "web_fetch",
				CheckpointID: "cp-approval",
				RerunNode:    "fetch",
				RequestedAt:  now,
			},
			Execution: agentstate.ExecutionState{
				Interrupted:     true,
				InterruptReason: "awaiting approval",
			},
			Pattern: agentstate.PatternState{
				Name: "reactive",
				Data: map[string]any{
					"approval_status": "approved",
					"checkpoint_id":   "wrong-pattern-checkpoint",
				},
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID:   "sess-approval-replay",
				Sequence:    1,
				Node:        "fetch",
				EventType:   agentstate.EventTypeInterrupt,
				Timestamp:   now,
				PayloadText: "awaiting approval",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-approval",
					Node: "fetch",
				},
			},
		},
		Checkpoint: &CheckpointRef{
			ID:          "cp-approval",
			Node:        "fetch",
			EventOffset: 1,
			CreatedAt:   now,
		},
	}

	view := BuildReplayView(session)

	if view.ApprovalStatus != agentstate.ApprovalStatusPending {
		t.Fatalf("expected pending approval status from shared state, got %+v", view)
	}
	if view.ApprovalReason != "web_fetch_approval_required" {
		t.Fatalf("expected approval reason from shared state, got %+v", view)
	}
	if view.ApprovalCapability != "web_fetch" {
		t.Fatalf("expected approval capability from shared state, got %+v", view)
	}
	if view.ApprovalCheckpointID != "cp-approval" {
		t.Fatalf("expected approval checkpoint id from shared state, got %+v", view)
	}
	if view.ApprovalRerunNode != "fetch" {
		t.Fatalf("expected approval rerun node from shared state, got %+v", view)
	}
	if view.CheckpointState.Status != "interrupted" || !view.CheckpointState.Active {
		t.Fatalf("expected interrupted active checkpoint state, got %+v", view.CheckpointState)
	}
}

func TestBuildReplayView_ReconstructsPendingApprovalFromJournalWhenSnapshotMissing(t *testing.T) {
	now := time.Now()
	session := &RuntimeSession{
		SessionID: "sess-approval-journal",
		Request: RequestEnvelope{
			Question: "journal approval fallback",
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID:   "sess-approval-journal",
				Sequence:    1,
				Node:        "approval",
				EventType:   agentstate.EventTypeApprovalPending,
				Timestamp:   now,
				PayloadText: "web_fetch_approval_required",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-journal-approval",
					Node: "approval",
				},
			},
			{
				SessionID:   "sess-approval-journal",
				Sequence:    2,
				Node:        "fetch",
				EventType:   agentstate.EventTypeInterrupt,
				Timestamp:   now.Add(1 * time.Second),
				PayloadText: "awaiting approval",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-journal-approval",
					Node: "fetch",
				},
			},
		},
	}

	view := BuildReplayView(session)

	if view.ApprovalStatus != agentstate.ApprovalStatusPending {
		t.Fatalf("expected pending approval reconstructed from journal, got %+v", view)
	}
	if view.ApprovalReason != "web_fetch_approval_required" {
		t.Fatalf("expected approval reason from journal, got %+v", view)
	}
	if view.ApprovalCheckpointID != "cp-journal-approval" {
		t.Fatalf("expected approval checkpoint from journal, got %+v", view)
	}
	if view.CheckpointState.Status != "interrupted" || !view.CheckpointState.Active {
		t.Fatalf("expected checkpoint state from journal, got %+v", view.CheckpointState)
	}
}

func TestBuildReplayView_ReconstructsResolvedApprovalFromJournalWhenSnapshotMissing(t *testing.T) {
	now := time.Now()
	session := &RuntimeSession{
		SessionID: "sess-approval-resolved-journal",
		Request: RequestEnvelope{
			Question: "journal approval resolved fallback",
		},
		Journal: []agentstate.RuntimeEvent{
			{
				SessionID:   "sess-approval-resolved-journal",
				Sequence:    1,
				Node:        "approval",
				EventType:   agentstate.EventTypeApprovalPending,
				Timestamp:   now,
				PayloadText: "fetch_approval_required",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-resolved-approval",
					Node: "approval",
				},
			},
			{
				SessionID:   "sess-approval-resolved-journal",
				Sequence:    2,
				Node:        "approval",
				EventType:   agentstate.EventTypeApprovalResolved,
				Timestamp:   now.Add(1 * time.Second),
				PayloadText: agentstate.ApprovalStatusApproved,
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-resolved-approval",
					Node: "approval",
				},
			},
			{
				SessionID:   "sess-approval-resolved-journal",
				Sequence:    3,
				Node:        "approval",
				EventType:   agentstate.EventTypeResumeCompleted,
				Timestamp:   now.Add(2 * time.Second),
				PayloadText: "checkpoint_id=cp-resolved-approval",
				Checkpoint: &agentstate.CheckpointRef{
					ID:   "cp-resolved-approval",
					Node: "approval",
				},
			},
		},
	}

	view := BuildReplayView(session)

	if view.ApprovalStatus != agentstate.ApprovalStatusApproved {
		t.Fatalf("expected approved status reconstructed from journal, got %+v", view)
	}
	if view.ApprovalCheckpointID != "cp-resolved-approval" {
		t.Fatalf("expected approval checkpoint from journal, got %+v", view)
	}
	if view.CheckpointState.Status != "resumed" || view.CheckpointState.Active {
		t.Fatalf("expected resumed checkpoint state from journal, got %+v", view.CheckpointState)
	}
}
