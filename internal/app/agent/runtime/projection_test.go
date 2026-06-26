package runtime

import (
	"testing"

	agentstate "local/rag-project/internal/app/agent/state"
)

func TestProjectSnapshotAt(t *testing.T) {
	searchQuery := "golang generics"
	sufficient := true
	reason := "fetched_readable_evidence"
	final := "final answer"

	session := &RuntimeSession{
		SessionID: "sess-project",
		InitialSnapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "golang generics",
			},
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "golang generics",
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{Sequence: 1, Node: "prepare", EventType: agentstate.EventTypeNodeStart},
			{
				Sequence:  2,
				Node:      "prepare",
				EventType: agentstate.EventTypeStateApplied,
				Delta: &agentstate.StateDelta{
					Context: &agentstate.ContextDelta{
						SearchQuery: &searchQuery,
					},
				},
			},
			{Sequence: 3, Node: "prepare", EventType: agentstate.EventTypeNodeFinish},
			{
				Sequence:  4,
				Node:      "observe",
				EventType: agentstate.EventTypeStateApplied,
				Delta: &agentstate.StateDelta{
					Evidence: &agentstate.EvidenceDelta{
						Sufficient:        &sufficient,
						SufficiencyReason: &reason,
					},
				},
			},
			{
				Sequence:  5,
				Node:      "answer",
				EventType: agentstate.EventTypeStateApplied,
				Delta: &agentstate.StateDelta{
					Answer: &agentstate.AnswerDelta{
						Final: &final,
					},
				},
			},
		},
	}

	projected, err := ProjectSnapshotAt(session, 3, nil)
	if err != nil {
		t.Fatalf("ProjectSnapshotAt(sequence=3) error = %v", err)
	}
	if projected.Context.SearchQuery != "golang generics" {
		t.Fatalf("expected search query after sequence 3, got %+v", projected.Context)
	}
	if projected.Evidence.Sufficient {
		t.Fatalf("expected evidence to remain unset before sequence 4, got %+v", projected.Evidence)
	}

	projected, err = ProjectSnapshotAt(session, 4, nil)
	if err != nil {
		t.Fatalf("ProjectSnapshotAt(sequence=4) error = %v", err)
	}
	if !projected.Evidence.Sufficient || projected.Evidence.SufficiencyReason != "fetched_readable_evidence" {
		t.Fatalf("expected evidence state at sequence 4, got %+v", projected.Evidence)
	}
	if projected.Answer.Final != "" {
		t.Fatalf("expected answer to remain empty before sequence 5, got %+v", projected.Answer)
	}

	projected, err = ProjectSnapshotAt(session, 99, nil)
	if err != nil {
		t.Fatalf("ProjectSnapshotAt(sequence=99) error = %v", err)
	}
	if projected.Answer.Final != "final answer" {
		t.Fatalf("expected final answer after full projection, got %+v", projected.Answer)
	}
}

func TestBuildProjectionTimeline(t *testing.T) {
	searchQuery := "runtime projection"
	session := &RuntimeSession{
		SessionID: "sess-timeline",
		InitialSnapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "runtime projection",
			},
		},
		Journal: []agentstate.RuntimeEvent{
			{Sequence: 1, Node: "prepare", EventType: agentstate.EventTypeNodeStart},
			{
				Sequence:  2,
				Node:      "prepare",
				EventType: agentstate.EventTypeStateApplied,
				Delta: &agentstate.StateDelta{
					Context: &agentstate.ContextDelta{
						SearchQuery: &searchQuery,
					},
				},
			},
			{Sequence: 3, Node: "prepare", EventType: agentstate.EventTypeNodeFinish},
		},
	}

	timeline, err := BuildProjectionTimeline(session, nil)
	if err != nil {
		t.Fatalf("BuildProjectionTimeline() error = %v", err)
	}
	if len(timeline) != 3 {
		t.Fatalf("expected 3 projection points, got %d", len(timeline))
	}
	if timeline[0].Snapshot.Context.SearchQuery != "" {
		t.Fatalf("expected no query at sequence 1, got %+v", timeline[0].Snapshot.Context)
	}
	if timeline[1].Snapshot.Context.SearchQuery != "runtime projection" {
		t.Fatalf("expected query at sequence 2, got %+v", timeline[1].Snapshot.Context)
	}
	if timeline[2].Snapshot.Context.SearchQuery != "runtime projection" {
		t.Fatalf("expected query to persist at sequence 3, got %+v", timeline[2].Snapshot.Context)
	}
}

func TestProjectSnapshotAt_NormalizesLegacySnapshotVersion(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-legacy-version",
		InitialSnapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "legacy snapshot",
			},
		},
	}

	projected, err := ProjectSnapshotAt(session, 0, nil)
	if err != nil {
		t.Fatalf("ProjectSnapshotAt(sequence=0) error = %v", err)
	}

	if projected.SchemaVersion != agentstate.CurrentSnapshotVersion {
		t.Fatalf("expected normalized schema version %d, got %d", agentstate.CurrentSnapshotVersion, projected.SchemaVersion)
	}
}

func TestProjectSnapshotAt_RejectsUnsupportedFutureSnapshotVersion(t *testing.T) {
	session := &RuntimeSession{
		SessionID: "sess-future-version",
		InitialSnapshot: agentstate.StateSnapshot{
			SchemaVersion: agentstate.CurrentSnapshotVersion + 1,
			Request: agentstate.RequestState{
				Question: "future snapshot",
			},
		},
	}

	_, err := ProjectSnapshotAt(session, 0, nil)
	if err == nil {
		t.Fatal("expected unsupported future snapshot version to fail")
	}
}
