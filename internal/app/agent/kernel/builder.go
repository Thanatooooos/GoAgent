package kernel

import (
	"context"
	"fmt"
	"strings"
	"time"

	agentruntime "local/rag-project/internal/app/agent/runtime"
	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

// BuilderConfig configures the minimal M1 kernel graph builder.
type BuilderConfig struct {
	GraphName            string
	Reducer              agentstate.Reducer
	CheckpointStore      CheckpointStore
	InterruptBeforeNodes []string
	CompileOptions       []compose.GraphCompileOption
}

// Builder assembles a typed runtime graph that runs on RuntimeSession.
type Builder struct {
	graph                *compose.Graph[*agentruntime.RuntimeSession, *agentruntime.RuntimeSession]
	reducer              agentstate.Reducer
	graphName            string
	checkpointStore      CheckpointStore
	interruptBeforeNodes []string
	compileOptions       []compose.GraphCompileOption
}

// NewBuilder creates a builder for the M1 runtime kernel skeleton.
func NewBuilder(cfg BuilderConfig) *Builder {
	reducer := cfg.Reducer
	if reducer == nil {
		reducer = agentstate.DefaultReducer{}
	}
	graphName := strings.TrimSpace(cfg.GraphName)
	if graphName == "" {
		graphName = "agent_runtime_m1"
	}
	return &Builder{
		graph:                compose.NewGraph[*agentruntime.RuntimeSession, *agentruntime.RuntimeSession](),
		reducer:              reducer,
		graphName:            graphName,
		checkpointStore:      cfg.CheckpointStore,
		interruptBeforeNodes: append([]string(nil), cfg.InterruptBeforeNodes...),
		compileOptions:       append([]compose.GraphCompileOption(nil), cfg.CompileOptions...),
	}
}

// AddNode adds a runtime-native node to the graph.
func (b *Builder) AddNode(node Node) error {
	if b == nil || b.graph == nil {
		return fmt.Errorf("kernel builder is not initialized")
	}
	if node == nil {
		return fmt.Errorf("node is required")
	}
	name := strings.TrimSpace(node.Name())
	if name == "" {
		return fmt.Errorf("node name is required")
	}

	b.graph.AddLambdaNode(name, compose.InvokableLambda(
		func(ctx context.Context, session *agentruntime.RuntimeSession) (*agentruntime.RuntimeSession, error) {
			return b.invokeNode(ctx, node, session)
		},
	))
	return nil
}

// AddEdge wires a linear edge between two nodes.
func (b *Builder) AddEdge(from, to string) error {
	if b == nil || b.graph == nil {
		return fmt.Errorf("kernel builder is not initialized")
	}
	return b.graph.AddEdge(from, to)
}

// AddBranch wires a branch after the given node.
func (b *Builder) AddBranch(from string, branch BranchFunc, targets []string) error {
	if b == nil || b.graph == nil {
		return fmt.Errorf("kernel builder is not initialized")
	}
	if branch == nil {
		return fmt.Errorf("branch func is required")
	}
	if len(targets) == 0 {
		return fmt.Errorf("branch targets are required")
	}
	allowed := make(map[string]bool, len(targets))
	for _, target := range targets {
		trimmed := strings.TrimSpace(target)
		if trimmed == "" {
			return fmt.Errorf("branch target is required")
		}
		allowed[trimmed] = true
	}
	return b.graph.AddBranch(from, compose.NewGraphBranch(
		func(ctx context.Context, session *agentruntime.RuntimeSession) (string, error) {
			target, err := branch(ctx, session)
			if err != nil {
				return "", err
			}
			sessionID := ""
			if session != nil {
				sessionID = session.SessionID
			}
			appendSessionEvent(session, agentstate.NewRuntimeEvent(
				sessionID,
				from,
				agentstate.EventTypeBranchSelected,
				target,
			))
			return target, nil
		},
		allowed,
	))
}

// Compile compiles the runtime graph into an executable runner.
func (b *Builder) Compile(ctx context.Context) (*Runner, error) {
	if b == nil || b.graph == nil {
		return nil, fmt.Errorf("kernel builder is not initialized")
	}

	opts := make([]compose.GraphCompileOption, 0, 3+len(b.compileOptions))
	opts = append(opts, compose.WithGraphName(b.graphName))
	if b.checkpointStore != nil {
		opts = append(opts, compose.WithCheckPointStore(b.checkpointStore))
	}
	if len(b.interruptBeforeNodes) > 0 {
		opts = append(opts, compose.WithInterruptBeforeNodes(append([]string(nil), b.interruptBeforeNodes...)))
	}
	opts = append(opts, b.compileOptions...)

	runnable, err := b.graph.Compile(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("compile kernel graph: %w", err)
	}
	return &Runner{
		runnable: runnable,
		reducer:  b.reducer,
	}, nil
}

func (b *Builder) invokeNode(ctx context.Context, node Node, session *agentruntime.RuntimeSession) (*agentruntime.RuntimeSession, error) {
	if session == nil {
		return nil, fmt.Errorf("runtime session is required")
	}
	if b.reducer == nil {
		return nil, fmt.Errorf("state reducer is required")
	}

	nodeName := node.Name()
	startedAt := time.Now()
	appendSessionEvent(session, agentstate.NewRuntimeEventAt(
		startedAt,
		session.SessionID,
		nodeName,
		agentstate.EventTypeNodeStart,
		"",
	))

	result, err := node.Run(ctx, session)
	if err != nil {
		errorDelta := nodeErrorDelta(nodeName)
		nextSnapshot, reduceErr := b.reducer.Apply(session.Snapshot, errorDelta)
		session.Metadata.UpdatedAt = time.Now()
		appendSessionEvent(session, agentstate.NewRuntimeEventAt(
			session.Metadata.UpdatedAt,
			session.SessionID,
			nodeName,
			agentstate.EventTypeNodeError,
			err.Error(),
		))
		if reduceErr != nil {
			appendSessionEvent(session, agentstate.NewRuntimeEventAt(
				session.Metadata.UpdatedAt,
				session.SessionID,
				nodeName,
				agentstate.EventTypeReducerError,
				reduceErr.Error(),
			))
			return session, fmt.Errorf("node run failed: %v; apply error delta: %w", err, reduceErr)
		}
		session.Snapshot = nextSnapshot
		stateApplied := agentstate.NewRuntimeEventAt(
			session.Metadata.UpdatedAt,
			session.SessionID,
			nodeName,
			agentstate.EventTypeStateApplied,
			"",
		)
		stateApplied.Delta = cloneDeltaPtr(errorDelta)
		appendSessionEvent(session, stateApplied)
		return session, err
	}

	for _, event := range result.Events {
		appendSessionEvent(session, event)
	}
	if result.Decision != nil {
		event := agentstate.NewRuntimeEventAt(
			time.Now(),
			session.SessionID,
			nodeName,
			agentstate.EventTypeDecisionEmitted,
			formatDecision(result.Decision),
		)
		event.Decision = agentstate.NewDecisionRef(
			result.Decision.Kind,
			result.Decision.Target,
			result.Decision.Confidence,
			result.Decision.Reasoning,
		)
		appendSessionEvent(session, event)
	}

	nextSnapshot, err := b.reducer.Apply(session.Snapshot, result.Delta)
	if err != nil {
		session.Metadata.UpdatedAt = time.Now()
		appendSessionEvent(session, agentstate.NewRuntimeEventAt(
			session.Metadata.UpdatedAt,
			session.SessionID,
			nodeName,
			agentstate.EventTypeReducerError,
			err.Error(),
		))
		return session, err
	}

	session.Snapshot = nextSnapshot
	session.Metadata.UpdatedAt = time.Now()
	stateApplied := agentstate.NewRuntimeEventAt(
		session.Metadata.UpdatedAt,
		session.SessionID,
		nodeName,
		agentstate.EventTypeStateApplied,
		"",
	)
	stateApplied.Delta = cloneDeltaPtr(result.Delta)
	appendSessionEvent(session, stateApplied)
	appendSessionEvent(session, agentstate.NewRuntimeEventAt(
		session.Metadata.UpdatedAt,
		session.SessionID,
		nodeName,
		agentstate.EventTypeNodeFinish,
		"",
	))

	return session, nil
}

func formatDecision(decision *agentruntime.DecisionArtifact) string {
	if decision == nil {
		return ""
	}
	return fmt.Sprintf("kind=%s target=%s confidence=%.2f reasoning=%s",
		decision.Kind,
		decision.Target,
		decision.Confidence,
		decision.Reasoning,
	)
}

func cloneDeltaPtr(delta agentstate.StateDelta) *agentstate.StateDelta {
	cloned := agentstate.CloneDelta(delta)
	return &cloned
}

func nodeErrorDelta(nodeName string) agentstate.StateDelta {
	return agentstate.StateDelta{
		Execution: &agentstate.ExecutionDelta{
			CurrentNode:   stringPtr(nodeName),
			FailedActions: []string{nodeName},
		},
	}
}

func stringPtr(value string) *string {
	return &value
}
