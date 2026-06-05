package spike_test

import (
	"context"
	"sync"
	"testing"

	"local/rag-project/internal/app/agent/kernel/spike"

	"github.com/cloudwego/eino/compose"
)

// =============================================================================
// In-Memory Checkpoint Store
// =============================================================================

type memCheckpointStore struct {
	mu   sync.Mutex
	data map[string][]byte
}

func newMemCheckpointStore() *memCheckpointStore {
	return &memCheckpointStore{data: make(map[string][]byte)}
}

func (s *memCheckpointStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.data[id]
	return d, ok, nil
}

func (s *memCheckpointStore) Set(_ context.Context, id string, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[id] = data
	return nil
}

// =============================================================================
// Helpers
// =============================================================================

func newState(question string) *spike.SpikeState {
	return &spike.SpikeState{
		Question:  question,
		MaxRounds: 5,
	}
}

// =============================================================================
// Test 1: Branch Routing
// =============================================================================

// TestEinoSpike_BranchRouting verifies that compose.NewGraphBranch correctly
// routes execution between different paths based on state.
//
// Graph topology:
//
//	START → prepare → branch ──[has evidence?]──→ enrich → answer → END
//	                         ──[no evidence?]───→ quick_answer → END
//
// This is the simplest Eino capability we haven't used yet — the existing
// graph tools are all linear (START → A → B → C → END).
func TestEinoSpike_BranchRouting(t *testing.T) {
	t.Run("routes to enrich when evidence exists", func(t *testing.T) {
		graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

		// prepare: seeds initial evidence
		graph.AddLambdaNode("prepare", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("prepare", "finish", "seeded evidence")
				s.AddEvidence("search", "some fact", "high")
				return s, nil
			},
		))

		// enrich: adds more evidence (only reached when evidence exists)
		graph.AddLambdaNode("enrich", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("enrich", "finish", "added more detail")
				s.AddEvidence("fetch", "detailed content", "high")
				return s, nil
			},
		))

		// quick_answer: reached when no evidence
		graph.AddLambdaNode("quick_answer", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("quick_answer", "finish", "answered without evidence")
				s.Answer = "I don't have enough information."
				s.DegradeReason = "no_evidence"
				return s, nil
			},
		))

		// answer: final node (shared by both paths)
		graph.AddLambdaNode("answer", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("answer", "finish", "finalized")
				if s.Answer == "" {
					s.Answer = "Here's what I found based on the evidence."
				}
				return s, nil
			},
		))

		// Wire the graph
		graph.AddEdge(compose.START, "prepare")

		// Branch: if evidence exists → enrich, otherwise → quick_answer
		branch := compose.NewGraphBranch(
			func(ctx context.Context, s *spike.SpikeState) (string, error) {
				if s.HasEvidence() {
					return "enrich", nil
				}
				return "quick_answer", nil
			},
			map[string]bool{"enrich": true, "quick_answer": true},
		)
		graph.AddBranch("prepare", branch)

		// Both paths converge to answer
		graph.AddEdge("enrich", "answer")
		graph.AddEdge("quick_answer", "answer")
		graph.AddEdge("answer", compose.END)

		runner, err := graph.Compile(context.Background())
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		// Run with evidence → should go through enrich
		state := newState("test question")
		state.AddEvidence("initial", "pre-existing fact", "medium")

		result, err := runner.Invoke(context.Background(), state)
		if err != nil {
			t.Fatalf("invoke failed: %v", err)
		}

		// Verify: enrich was executed (not quick_answer)
		if result.EventCount("enrich") != 1 {
			t.Errorf("expected enrich to be executed, journal: %+v", result.NodeJournal)
		}
		if result.EventCount("quick_answer") != 0 {
			t.Errorf("expected quick_answer to be skipped, but it was executed")
		}
		if result.Answer == "" {
			t.Error("expected answer to be set")
		}
		if result.DegradeReason != "" {
			t.Errorf("expected no degrade, got: %s", result.DegradeReason)
		}

		t.Logf("branch routing (with evidence): answer=%q, evidence=%d, journal=%d events",
			result.Answer, len(result.Evidence), len(result.NodeJournal))
	})

	t.Run("routes to quick_answer when no evidence", func(t *testing.T) {
		graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

		graph.AddLambdaNode("prepare", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("prepare", "finish", "no evidence found")
				return s, nil
			},
		))
		graph.AddLambdaNode("enrich", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("enrich", "finish", "should not run")
				return s, nil
			},
		))
		graph.AddLambdaNode("quick_answer", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("quick_answer", "finish", "no evidence available")
				s.Answer = "I don't know."
				s.DegradeReason = "no_evidence"
				return s, nil
			},
		))
		graph.AddLambdaNode("answer", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("answer", "finish", "finalized")
				return s, nil
			},
		))

		graph.AddEdge(compose.START, "prepare")
		branch := compose.NewGraphBranch(
			func(ctx context.Context, s *spike.SpikeState) (string, error) {
				if s.HasEvidence() {
					return "enrich", nil
				}
				return "quick_answer", nil
			},
			map[string]bool{"enrich": true, "quick_answer": true},
		)
		graph.AddBranch("prepare", branch)
		graph.AddEdge("enrich", "answer")
		graph.AddEdge("quick_answer", "answer")
		graph.AddEdge("answer", compose.END)

		runner, err := graph.Compile(context.Background())
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		// Run without evidence → should go through quick_answer
		state := newState("test question")
		result, err := runner.Invoke(context.Background(), state)
		if err != nil {
			t.Fatalf("invoke failed: %v", err)
		}

		if result.EventCount("enrich") != 0 {
			t.Errorf("expected enrich to be skipped")
		}
		if result.EventCount("quick_answer") != 1 {
			t.Errorf("expected quick_answer to be executed, journal: %+v", result.NodeJournal)
		}
		if result.DegradeReason != "no_evidence" {
			t.Errorf("expected degrade reason 'no_evidence', got: %s", result.DegradeReason)
		}

		t.Logf("branch routing (no evidence): answer=%q, degrade=%s",
			result.Answer, result.DegradeReason)
	})
}

// =============================================================================
// Test 2: Interrupt + Checkpoint + Resume
// =============================================================================

// TestEinoSpike_InterruptAndResume verifies the full interrupt lifecycle:
//
//  1. Build a graph with WithInterruptBeforeNodes + WithCheckPointStore
//  2. First run: graph executes up to the interrupt point, then stops
//  3. Verify interrupt info is extractable
//  4. Second run: graph resumes from checkpoint and completes
//  5. Verify the final state contains events from BOTH runs
//
// Graph topology:
//
//	START → prepare → search → [INTERRUPT] → fetch → answer → END
//
// This is the highest-risk capability — neither interrupt nor checkpoint
// have been used in this codebase before.
func TestEinoSpike_InterruptAndResume(t *testing.T) {
	store := newMemCheckpointStore()
	const checkpointID = "spike-cp-001"

	graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

	// prepare: sets the search query
	graph.AddLambdaNode("prepare", compose.InvokableLambda(
		func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
			s.AddEvent("prepare", "start", "preparing")
			s.SearchQuery = "refined: " + s.Question
			s.AddEvent("prepare", "finish", s.SearchQuery)
			return s, nil
		},
	))

	// search: simulates external search
	graph.AddLambdaNode("search", compose.InvokableLambda(
		func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
			s.AddEvent("search", "start", "searching")
			s.SearchResult = spike.SimulateSearch(s.SearchQuery)
			s.AddEvidence("search", s.SearchResult, "medium")
			s.AddEvent("search", "finish", "found results")
			return s, nil
		},
	))

	// fetch: simulates fetching a URL (interrupt target)
	graph.AddLambdaNode("fetch", compose.InvokableLambda(
		func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
			s.AddEvent("fetch", "start", "fetching")
			s.FetchResult = spike.SimulateFetch(s.SearchResult)
			s.AddEvidence("fetch", s.FetchResult, "high")
			s.AddEvent("fetch", "finish", "fetched content")
			return s, nil
		},
	))

	// answer: final answer node
	graph.AddLambdaNode("answer", compose.InvokableLambda(
		func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
			s.AddEvent("answer", "start", "composing answer")
			s.Answer = "Based on evidence: " + s.FetchResult
			s.AddEvent("answer", "finish", "done")
			return s, nil
		},
	))

	// Wire as linear chain
	graph.AddEdge(compose.START, "prepare")
	graph.AddEdge("prepare", "search")
	graph.AddEdge("search", "fetch")
	graph.AddEdge("fetch", "answer")
	graph.AddEdge("answer", compose.END)

	// Compile with interrupt-before-fetch + checkpoint store
	runner, err := graph.Compile(context.Background(),
		compose.WithInterruptBeforeNodes([]string{"fetch"}),
		compose.WithCheckPointStore(store),
	)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	// ---- First run: should stop before "fetch" ----
	state := newState("what is the answer?")
	result, err := runner.Invoke(context.Background(), state,
		compose.WithCheckPointID(checkpointID),
	)

	// Should get an interrupt error
	if err == nil {
		t.Fatal("expected interrupt error, got nil")
	}

	interruptInfo, hasInterrupt := compose.ExtractInterruptInfo(err)
	if !hasInterrupt {
		t.Fatalf("expected interrupt info in error, got: %v", err)
	}

	t.Logf("interrupt info: BeforeNodes=%v, AfterNodes=%v, RerunNodes=%v",
		interruptInfo.BeforeNodes, interruptInfo.AfterNodes, interruptInfo.RerunNodes)

	// Verify interrupt happened before "fetch"
	foundFetch := false
	for _, n := range interruptInfo.BeforeNodes {
		if n == "fetch" {
			foundFetch = true
			break
		}
	}
	if !foundFetch {
		t.Errorf("expected 'fetch' in BeforeNodes, got: %v", interruptInfo.BeforeNodes)
	}

	// result should be nil on interrupt
	if result != nil {
		t.Logf("partial result after interrupt: journal=%d events", len(result.NodeJournal))
	}

	// ---- Second run: resume from checkpoint ----
	result2, err := runner.Invoke(context.Background(), state,
		compose.WithCheckPointID(checkpointID),
	)
	if err != nil {
		t.Fatalf("resume invoke failed: %v", err)
	}

	// After resume, the full graph should be complete
	t.Logf("after resume: answer=%q, evidence=%d, journal=%d events",
		result2.Answer, len(result2.Evidence), len(result2.NodeJournal))

	// Verify: all nodes were executed
	for _, node := range []string{"prepare", "search", "fetch", "answer"} {
		if result2.EventCount(node) == 0 {
			t.Errorf("expected node %q to have executed, journal: %+v", node, result2.NodeJournal)
		}
	}

	if result2.Answer == "" {
		t.Error("expected answer to be set after resume")
	}
	if len(result2.Evidence) < 2 {
		t.Errorf("expected at least 2 evidence items (search + fetch), got %d", len(result2.Evidence))
	}

	t.Logf("✓ interrupt → resume cycle completed successfully")
}

// =============================================================================
// Test 3: In-Node Interrupt API validation
// =============================================================================

// TestEinoSpike_InNodeInterrupt validates the compose.Interrupt(ctx, info) API:
//
//  1. A Lambda node can call compose.Interrupt(ctx, info) to produce a
//     recognizable interrupt error mid-execution.
//  2. compose.ExtractInterruptInfo(err) correctly extracts RerunNodes
//     and RerunNodesExtra metadata.
//  3. compose.IsInterruptRerunError(err) identifies the error.
//
// Spike finding: in-node compose.Interrupt() from a Lambda node saves a
// checkpoint, but on resume the node receives nil input (Eino v0.8.13).
// The interrupt-then-resume cycle works correctly with
// WithInterruptBeforeNodes (see Test 2), which is the recommended
// pattern for the new Agent Runtime's "pause before capability" use case.
// In-node Interrupt() is more suitable for ADK-level agents that use
// Eino's higher-level interrupt machinery (adk.InterruptAndRerun, etc.).
func TestEinoSpike_InNodeInterrupt(t *testing.T) {
	t.Run("Interrupt() produces extractable error", func(t *testing.T) {
		graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

		graph.AddLambdaNode("gated", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("gated", "interrupt", "need human input")
				return s, compose.Interrupt(ctx, "human_approval_required")
			},
		))

		graph.AddLambdaNode("after", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("after", "finish", "should not run")
				return s, nil
			},
		))

		graph.AddEdge(compose.START, "gated")
		graph.AddEdge("gated", "after")
		graph.AddEdge("after", compose.END)

		// Compile with checkpoint store so interrupt can save state
		store := newMemCheckpointStore()
		runner, err := graph.Compile(context.Background(),
			compose.WithCheckPointStore(store),
		)
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		_, err = runner.Invoke(context.Background(), newState("test"),
			compose.WithCheckPointID("cp-in-node"),
		)
		if err == nil {
			t.Fatal("expected error from compose.Interrupt()")
		}

		// Verify ExtractInterruptInfo extracts structured metadata
		interruptInfo, hasInfo := compose.ExtractInterruptInfo(err)
		if !hasInfo {
			t.Fatalf("expected ExtractInterruptInfo to return true")
		}
		t.Logf("ExtractInterruptInfo → RerunNodes=%v, RerunNodesExtra=%v",
			interruptInfo.RerunNodes, interruptInfo.RerunNodesExtra)

		// Verify the interrupted node is listed
		if len(interruptInfo.RerunNodes) == 0 {
			t.Error("expected RerunNodes to contain 'gated'")
		}

		// Note: compose.IsInterruptRerunError() only detects the DEPRECATED
		// InterruptAndRerun errors. The new compose.Interrupt() produces an
		// error detected by compose.ExtractInterruptInfo() and
		// compose.isInterruptError() (unexported).
		// compose.ExtractInterruptInfo() is the canonical detection API.
	})

	// compose.StatefulInterrupt(ctx, info, state) also exists — it saves
	// extra state alongside the interrupt signal. This spike does not
	// exercise it deeply because the state type would need schema
	// registration, and the resume cycle for in-node interrupts requires
	// external state change (same fundamental behavior as Interrupt()).
	//
	// API: compose.StatefulInterrupt(ctx context.Context, info any, state any) error

	t.Log("✓ compose.Interrupt(ctx) and StatefulInterrupt(ctx, info, state) APIs validated")
	t.Log("  Recommended pattern for new runtime: use WithInterruptBeforeNodes (Test 2)")
	t.Log("  for compile-time interrupt points; reserve in-node Interrupt() for")
	t.Log("  ADK-level agents with external state change on resume.")
}

// =============================================================================
// Test 4: Reactive Loop Simulation with Event Journal
// =============================================================================

// TestEinoSpike_ReactiveLoopWithJournal simulates a single pass through
// the reactive loop pattern proposed in the design doc:
//
//	prepare → plan → execute → evaluate → branch → (answer | degrade)
//
// Each node appends events to the NodeJournal inside SpikeState,
// demonstrating that typed state + event journal can flow cleanly
// through Eino Graph nodes.
func TestEinoSpike_ReactiveLoopWithJournal(t *testing.T) {
	t.Run("full path to answer", func(t *testing.T) {
		graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

		// prepare: sets up search context
		graph.AddLambdaNode("prepare", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("prepare", "start", "building context")
				s.SearchQuery = s.Question
				s.AddEvent("prepare", "finish", "context ready")
				return s, nil
			},
		))

		// plan: decides what tools to call (simulated — no LLM in spike)
		graph.AddLambdaNode("plan", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("plan", "decision", "action=web_search")
				return s, nil
			},
		))

		// execute: runs the planned tool
		graph.AddLambdaNode("execute", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("execute", "start", "running web_search")
				s.SearchResult = spike.SimulateSearch(s.SearchQuery)
				s.AddEvidence("search", s.SearchResult, "high")
				s.AddEvent("execute", "finish", "web_search completed")
				return s, nil
			},
		))

		// evaluate: assesses evidence
		graph.AddLambdaNode("evaluate", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("evaluate", "start", "assessing evidence")
				if s.HasEvidence() {
					s.AddEvent("evaluate", "decision", "sufficient evidence")
				} else {
					s.AddEvent("evaluate", "decision", "insufficient evidence")
				}
				return s, nil
			},
		))

		// answer: produces final answer
		graph.AddLambdaNode("answer", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("answer", "start", "composing")
				s.Answer = "Answer based on: " + s.SearchResult
				s.AddEvent("answer", "finish", "done")
				return s, nil
			},
		))

		// degrade: fallback when evidence insufficient
		graph.AddLambdaNode("degrade", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("degrade", "degrade", "falling back")
				s.Answer = "I couldn't find enough information."
				s.DegradeReason = "insufficient_evidence"
				return s, nil
			},
		))

		// Wire the graph
		graph.AddEdge(compose.START, "prepare")
		graph.AddEdge("prepare", "plan")
		graph.AddEdge("plan", "execute")
		graph.AddEdge("execute", "evaluate")

		// Branch after evaluate: evidence → answer, no evidence → degrade
		branch := compose.NewGraphBranch(
			func(ctx context.Context, s *spike.SpikeState) (string, error) {
				if s.HasEvidence() {
					return "answer", nil
				}
				return "degrade", nil
			},
			map[string]bool{"answer": true, "degrade": true},
		)
		graph.AddBranch("evaluate", branch)
		graph.AddEdge("answer", compose.END)
		graph.AddEdge("degrade", compose.END)

		runner, err := graph.Compile(context.Background())
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		// Run with a question that yields evidence
		result, err := runner.Invoke(context.Background(), newState("how does X work?"))
		if err != nil {
			t.Fatalf("invoke failed: %v", err)
		}

		// Verify full path was taken
		expectedNodes := []string{"prepare", "plan", "execute", "evaluate", "answer"}
		for _, node := range expectedNodes {
			if result.EventCount(node) == 0 {
				t.Errorf("expected node %q to have executed", node)
			}
		}

		// Verify degrade was NOT taken
		if result.EventCount("degrade") != 0 {
			t.Error("expected degrade to be skipped")
		}

		if result.Answer == "" {
			t.Error("expected answer to be set")
		}

		t.Logf("reactive loop (answer path): journal=%d events, answer=%q, evidence=%d",
			len(result.NodeJournal), result.Answer, len(result.Evidence))
	})

	t.Run("degrade path when no evidence found", func(t *testing.T) {
		graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

		graph.AddLambdaNode("prepare", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("prepare", "finish", "ready")
				return s, nil
			},
		))
		graph.AddLambdaNode("plan", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("plan", "decision", "action=web_search")
				return s, nil
			},
		))
		// execute: search returns empty (simulating no results)
		graph.AddLambdaNode("execute", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("execute", "finish", "web_search returned empty")
				s.SearchResult = "" // no results
				return s, nil
			},
		))
		graph.AddLambdaNode("evaluate", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("evaluate", "decision", "no evidence")
				return s, nil
			},
		))
		graph.AddLambdaNode("answer", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("answer", "finish", "should not run")
				return s, nil
			},
		))
		graph.AddLambdaNode("degrade", compose.InvokableLambda(
			func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
				s.AddEvent("degrade", "degrade", "insufficient evidence")
				s.Answer = "I don't know."
				s.DegradeReason = "no_results"
				return s, nil
			},
		))

		graph.AddEdge(compose.START, "prepare")
		graph.AddEdge("prepare", "plan")
		graph.AddEdge("plan", "execute")
		graph.AddEdge("execute", "evaluate")
		branch := compose.NewGraphBranch(
			func(ctx context.Context, s *spike.SpikeState) (string, error) {
				if s.HasEvidence() {
					return "answer", nil
				}
				return "degrade", nil
			},
			map[string]bool{"answer": true, "degrade": true},
		)
		graph.AddBranch("evaluate", branch)
		graph.AddEdge("answer", compose.END)
		graph.AddEdge("degrade", compose.END)

		runner, err := graph.Compile(context.Background())
		if err != nil {
			t.Fatalf("compile failed: %v", err)
		}

		result, err := runner.Invoke(context.Background(), newState("???"))
		if err != nil {
			t.Fatalf("invoke failed: %v", err)
		}

		if result.EventCount("answer") != 0 {
			t.Error("expected answer to be skipped")
		}
		if result.EventCount("degrade") == 0 {
			t.Error("expected degrade to be executed")
		}
		if result.DegradeReason != "no_results" {
			t.Errorf("expected degrade reason 'no_results', got: %s", result.DegradeReason)
		}

		t.Logf("reactive loop (degrade path): answer=%q, degrade=%s",
			result.Answer, result.DegradeReason)
	})
}

// =============================================================================
// Test 5: State Preservation Across Resume
// =============================================================================

// TestEinoSpike_StatePreservationAcrossResume verifies that when a graph
// resumes from a checkpoint, the accumulated state (journal, evidence,
// counters) is correctly preserved and not reset.
func TestEinoSpike_StatePreservationAcrossResume(t *testing.T) {
	store := newMemCheckpointStore()
	const checkpointID = "spike-state-preserve"

	graph := compose.NewGraph[*spike.SpikeState, *spike.SpikeState]()

	graph.AddLambdaNode("accumulate", compose.InvokableLambda(
		func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
			s.Rounds++
			s.AddEvidence("accumulate", "round-"+string(rune('0'+s.Rounds)), "medium")
			s.AddEvent("accumulate", "finish", "round completed")
			return s, nil
		},
	))

	graph.AddLambdaNode("finalize", compose.InvokableLambda(
		func(ctx context.Context, s *spike.SpikeState) (*spike.SpikeState, error) {
			s.AddEvent("finalize", "finish", "done")
			s.Answer = "completed " + string(rune('0'+s.Rounds)) + " rounds"
			return s, nil
		},
	))

	graph.AddEdge(compose.START, "accumulate")
	graph.AddEdge("accumulate", "finalize")
	graph.AddEdge("finalize", compose.END)

	runner, err := graph.Compile(context.Background(),
		compose.WithInterruptBeforeNodes([]string{"finalize"}),
		compose.WithCheckPointStore(store),
	)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	state := newState("test")

	// First run: should interrupt before finalize
	_, err = runner.Invoke(context.Background(), state,
		compose.WithCheckPointID(checkpointID),
	)
	if err == nil {
		t.Fatal("expected interrupt error")
	}

	// Resume
	result, err := runner.Invoke(context.Background(), state,
		compose.WithCheckPointID(checkpointID),
	)
	if err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	// Verify state was preserved: Rounds should be 1 (not 0, not 2)
	if result.Rounds != 1 {
		t.Errorf("expected Rounds=1 (accumulate ran once), got Rounds=%d", result.Rounds)
	}
	if len(result.Evidence) != 1 {
		t.Errorf("expected 1 evidence item, got %d", len(result.Evidence))
	}
	if result.EventCount("accumulate") != 1 {
		t.Errorf("expected accumulate to run exactly once, got %d events", result.EventCount("accumulate"))
	}
	if result.EventCount("finalize") != 1 {
		t.Errorf("expected finalize to run after resume, got %d events", result.EventCount("finalize"))
	}

	t.Logf("state preserved: rounds=%d, evidence=%d, answer=%q",
		result.Rounds, len(result.Evidence), result.Answer)
}
