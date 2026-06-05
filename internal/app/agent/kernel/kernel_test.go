package kernel

import (
	"context"
	"fmt"
	"testing"
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

func TestKernelRunner_RunReactiveBranchingPath(t *testing.T) {
	builder := NewBuilder(BuilderConfig{
		GraphName: "agent_kernel_branch_test",
		Reducer:   agentstate.DefaultReducer{},
	})

	prepare, err := NewNodeFunc("prepare", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		query := session.Request.Question + " rewritten"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					RewrittenQuery: &query,
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(prepare) error = %v", err)
	}
	evaluate, err := NewNodeFunc("evaluate", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		sufficient := true
		reason := "sufficient evidence"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Evidence: &agentstate.EvidenceDelta{
					Sufficient:        &sufficient,
					SufficiencyReason: &reason,
				},
			},
			Decision: &agentruntime.DecisionArtifact{
				Kind:       "branch",
				Target:     "answer",
				Confidence: 0.9,
				Reasoning:  "evidence is sufficient",
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(evaluate) error = %v", err)
	}
	answer, err := NewNodeFunc("answer", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		final := "final answer"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Answer: &agentstate.AnswerDelta{
					Final: &final,
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(answer) error = %v", err)
	}
	degrade, err := NewNodeFunc("degrade", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		final := "degraded answer"
		reason := "insufficient_evidence"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Answer: &agentstate.AnswerDelta{
					Final:         &final,
					DegradeReason: &reason,
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(degrade) error = %v", err)
	}

	for _, node := range []Node{prepare, evaluate, answer, degrade} {
		if err := builder.AddNode(node); err != nil {
			t.Fatalf("AddNode(%s) error = %v", node.Name(), err)
		}
	}
	if err := builder.AddEdge(compose.START, "prepare"); err != nil {
		t.Fatalf("AddEdge(start, prepare) error = %v", err)
	}
	if err := builder.AddEdge("prepare", "evaluate"); err != nil {
		t.Fatalf("AddEdge(prepare, evaluate) error = %v", err)
	}
	if err := builder.AddBranch("evaluate", func(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
		if session.Snapshot.Evidence.Sufficient {
			return "answer", nil
		}
		return "degrade", nil
	}, []string{"answer", "degrade"}); err != nil {
		t.Fatalf("AddBranch(evaluate) error = %v", err)
	}
	if err := builder.AddEdge("answer", compose.END); err != nil {
		t.Fatalf("AddEdge(answer, end) error = %v", err)
	}
	if err := builder.AddEdge("degrade", compose.END); err != nil {
		t.Fatalf("AddEdge(degrade, end) error = %v", err)
	}

	runner, err := builder.Compile(context.Background())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	session := &agentruntime.RuntimeSession{
		SessionID: "sess-1",
		Request: agentruntime.RequestEnvelope{
			Question: "what happened",
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "what happened",
			},
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt: time.Now(),
		},
	}

	result, err := runner.Run(context.Background(), session)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if result.Snapshot.Context.RewrittenQuery != "what happened rewritten" {
		t.Fatalf("expected rewritten query to be updated, got %q", result.Snapshot.Context.RewrittenQuery)
	}
	if !result.Snapshot.Evidence.Sufficient {
		t.Fatal("expected evidence sufficiency to be true")
	}
	if result.Snapshot.Answer.Final != "final answer" {
		t.Fatalf("expected final answer branch, got %q", result.Snapshot.Answer.Final)
	}
	if result.Snapshot.Answer.DegradeReason != "" {
		t.Fatalf("expected degrade branch to be skipped, got degrade reason %q", result.Snapshot.Answer.DegradeReason)
	}
	if len(result.Journal) == 0 {
		t.Fatal("expected journal events to be appended")
	}
	if !journalHasEventType(result.Journal, agentstate.EventTypeBranchSelected) {
		t.Fatalf("expected branch_selected event in journal, got %+v", result.Journal)
	}
}

func TestKernelRunner_RunWithCheckpointResume(t *testing.T) {
	store := NewMemoryCheckpointStore()
	builder := NewBuilder(BuilderConfig{
		GraphName:            "agent_kernel_checkpoint_test",
		Reducer:              agentstate.DefaultReducer{},
		CheckpointStore:      store,
		InterruptBeforeNodes: []string{"answer"},
	})

	prepare, err := NewNodeFunc("prepare", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		query := "prepared query"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Context: &agentstate.ContextDelta{
					SearchQuery: &query,
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(prepare) error = %v", err)
	}
	answer, err := NewNodeFunc("answer", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		final := "answer after resume"
		return agentruntime.NodeResult{
			Delta: agentstate.StateDelta{
				Answer: &agentstate.AnswerDelta{
					Final: &final,
				},
			},
		}, nil
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(answer) error = %v", err)
	}

	for _, node := range []Node{prepare, answer} {
		if err := builder.AddNode(node); err != nil {
			t.Fatalf("AddNode(%s) error = %v", node.Name(), err)
		}
	}
	if err := builder.AddEdge(compose.START, "prepare"); err != nil {
		t.Fatalf("AddEdge(start, prepare) error = %v", err)
	}
	if err := builder.AddEdge("prepare", "answer"); err != nil {
		t.Fatalf("AddEdge(prepare, answer) error = %v", err)
	}
	if err := builder.AddEdge("answer", compose.END); err != nil {
		t.Fatalf("AddEdge(answer, end) error = %v", err)
	}

	runner, err := builder.Compile(context.Background())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	session := &agentruntime.RuntimeSession{
		SessionID: "sess-checkpoint",
		Request: agentruntime.RequestEnvelope{
			Question: "checkpoint question",
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "checkpoint question",
			},
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt: time.Now(),
		},
	}

	interrupted, err := runner.RunWithCheckpoint(context.Background(), session, "cp-1")
	if err == nil {
		t.Fatal("expected interrupt error on first run")
	}
	if _, ok := compose.ExtractInterruptInfo(err); !ok {
		t.Fatalf("expected interrupt info, got %v", err)
	}
	if interrupted == nil {
		t.Fatal("expected interrupted session result")
	}
	if interrupted.Checkpoint == nil {
		t.Fatal("expected checkpoint metadata to be captured")
	}
	if interrupted.Checkpoint.ID != "cp-1" {
		t.Fatalf("expected checkpoint id cp-1, got %+v", interrupted.Checkpoint)
	}
	if interrupted.Checkpoint.Node != "answer" {
		t.Fatalf("expected checkpoint node answer, got %+v", interrupted.Checkpoint)
	}
	if !interrupted.Snapshot.Execution.Interrupted {
		t.Fatalf("expected execution interrupted state, got %+v", interrupted.Snapshot.Execution)
	}
	if interrupted.Snapshot.Execution.CurrentNode != "answer" {
		t.Fatalf("expected execution current node answer on interrupt, got %+v", interrupted.Snapshot.Execution)
	}
	if len(interrupted.Journal) == 0 || interrupted.Journal[len(interrupted.Journal)-1].EventType != agentstate.EventTypeInterrupt {
		t.Fatalf("expected interrupt journal event, got %+v", interrupted.Journal)
	}
	if !journalHasEventType(interrupted.Journal, agentstate.EventTypeStateApplied) {
		t.Fatalf("expected interrupt path to apply execution delta through reducer, got %+v", interrupted.Journal)
	}

	resumed, err := runner.Resume(context.Background(), session, "cp-1")
	if err != nil {
		t.Fatalf("resume Run() error = %v", err)
	}

	if resumed.Snapshot.Context.SearchQuery != "prepared query" {
		t.Fatalf("expected checkpoint to preserve prepared query, got %q", resumed.Snapshot.Context.SearchQuery)
	}
	if resumed.Snapshot.Answer.Final != "answer after resume" {
		t.Fatalf("expected answer after resume, got %q", resumed.Snapshot.Answer.Final)
	}
	if resumed.Metadata.ResumedFrom != "cp-1" {
		t.Fatalf("expected resumed_from cp-1, got %+v", resumed.Metadata)
	}
	if resumed.Metadata.ResumeCount != 1 {
		t.Fatalf("expected resume count 1, got %+v", resumed.Metadata)
	}
	if resumed.Snapshot.Execution.Interrupted {
		t.Fatalf("expected execution interrupt flag cleared on resume, got %+v", resumed.Snapshot.Execution)
	}
	if resumed.Snapshot.Execution.InterruptReason != "" {
		t.Fatalf("expected interrupt reason cleared on resume, got %+v", resumed.Snapshot.Execution)
	}
	if len(resumed.Journal) == 0 || resumed.Journal[len(resumed.Journal)-1].EventType != agentstate.EventTypeResumeCompleted {
		t.Fatalf("expected resume_completed journal event, got %+v", resumed.Journal)
	}
	if !journalHasEventType(resumed.Journal, agentstate.EventTypeStateApplied) {
		t.Fatalf("expected resume path to apply execution delta through reducer, got %+v", resumed.Journal)
	}
}

func TestKernelRunner_RunNodeErrorAppliesExecutionDeltaThroughReducer(t *testing.T) {
	builder := NewBuilder(BuilderConfig{
		GraphName: "agent_kernel_error_path_test",
		Reducer:   agentstate.DefaultReducer{},
	})

	failing, err := NewNodeFunc("failing", func(ctx context.Context, session *agentruntime.RuntimeSession) (agentruntime.NodeResult, error) {
		return agentruntime.NodeResult{}, fmt.Errorf("boom")
	})
	if err != nil {
		t.Fatalf("NewNodeFunc(failing) error = %v", err)
	}

	if err := builder.AddNode(failing); err != nil {
		t.Fatalf("AddNode(failing) error = %v", err)
	}
	if err := builder.AddEdge(compose.START, "failing"); err != nil {
		t.Fatalf("AddEdge(start, failing) error = %v", err)
	}
	if err := builder.AddEdge("failing", compose.END); err != nil {
		t.Fatalf("AddEdge(failing, end) error = %v", err)
	}

	runner, err := builder.Compile(context.Background())
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	session := &agentruntime.RuntimeSession{
		SessionID: "sess-error",
		Request: agentruntime.RequestEnvelope{
			Question: "why did it fail",
		},
		Snapshot: agentstate.StateSnapshot{
			Request: agentstate.RequestState{
				Question: "why did it fail",
			},
		},
		Metadata: agentruntime.SessionMetadata{
			CreatedAt: time.Now(),
		},
	}

	result, err := runner.Run(context.Background(), session)
	if err == nil {
		t.Fatal("expected node error")
	}
	if result == nil {
		t.Fatal("expected runtime session result on node error")
	}
	if result.Snapshot.Execution.CurrentNode != "failing" {
		t.Fatalf("expected current node to be updated through reducer, got %+v", result.Snapshot.Execution)
	}
	if len(result.Snapshot.Execution.FailedActions) != 1 || result.Snapshot.Execution.FailedActions[0] != "failing" {
		t.Fatalf("expected failed action to be recorded through reducer, got %+v", result.Snapshot.Execution)
	}
	if !journalHasEventType(result.Journal, agentstate.EventTypeNodeError) {
		t.Fatalf("expected node_error event in journal, got %+v", result.Journal)
	}
	if !journalHasEventType(result.Journal, agentstate.EventTypeStateApplied) {
		t.Fatalf("expected state_applied event for error delta in journal, got %+v", result.Journal)
	}
}

func journalHasEventType(events []agentstate.RuntimeEvent, eventType string) bool {
	for _, event := range events {
		if event.EventType == eventType {
			return true
		}
	}
	return false
}
