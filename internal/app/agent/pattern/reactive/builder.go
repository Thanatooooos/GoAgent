package reactive

import (
	"context"
	"fmt"
	"strings"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentkernel "local/rag-project/internal/app/agent/kernel"
	agentpattern "local/rag-project/internal/app/agent/pattern"
	agentstate "local/rag-project/internal/app/agent/state"

	"github.com/cloudwego/eino/compose"
)

// Config assembles the first runtime-native reactive pattern.
type Config struct {
	Assembly agentpattern.AssemblyContext
	Runtime  agentpattern.RuntimeConfig
}

// Compile builds the reactive runtime graph and returns a runnable kernel runner.
func Compile(ctx context.Context, cfg Config) (*agentkernel.Runner, error) {
	if cfg.Assembly.Registry == nil {
		return nil, fmt.Errorf("reactive pattern requires capability registry")
	}
	outputMode := strings.TrimSpace(cfg.Runtime.OutputMode)
	if outputMode == "" {
		outputMode = agentstate.OutputModeFinalAnswer
	}

	var (
		bindings           agentcapability.RoleBindings
		searchCapability   agentcapability.Handle
		fetchCapability    agentcapability.Handle
		workflowCapability agentcapability.Handle
		err                error
	)
	if cfg.Runtime.PreferExternalEvidenceWorkflow {
		workflowCapability, err = resolveWorkflowCapability(cfg.Assembly.Registry, cfg.Assembly.Bindings)
		if err != nil {
			return nil, err
		}
	} else {
		bindings, err = agentpattern.ResolveNamedBindings(
			cfg.Assembly.Registry,
			cfg.Assembly.Bindings,
			"reactive",
			agentcapability.RoleSearch,
			agentcapability.RoleFetch,
		)
		if err != nil {
			return nil, err
		}
		searchCapability, err = cfg.Assembly.Registry.Handle(bindings.Resolve(agentcapability.RoleSearch))
		if err != nil {
			return nil, err
		}
		fetchCapability, err = cfg.Assembly.Registry.Handle(bindings.Resolve(agentcapability.RoleFetch))
		if err != nil {
			return nil, err
		}
	}

	kernelCfg := cfg.Runtime.Kernel
	if strings.TrimSpace(kernelCfg.GraphName) == "" {
		kernelCfg.GraphName = "agent_pattern_reactive"
	}
	if kernelCfg.Reducer == nil {
		kernelCfg.Reducer = agentstate.DefaultReducer{}
	}
	kernelCfg.InterruptBeforeNodes = agentpattern.MergeInterruptBeforeNodes(
		kernelCfg.InterruptBeforeNodes,
		requiredInterruptNodes(searchCapability, fetchCapability, workflowCapability, kernelCfg.CheckpointStore != nil),
	)
	capabilityPolicy := buildCapabilityRuntimePolicy(searchCapability, fetchCapability, workflowCapability, cfg.Runtime.PreferExternalEvidenceWorkflow)
	approvalResumeEnabled := kernelCfg.CheckpointStore != nil

	builder := agentkernel.NewBuilder(kernelCfg)
	prepare, err := newPrepareNode()
	if err != nil {
		return nil, err
	}
	observe, err := newObserveNode(cfg.Runtime.Planner, outputMode, capabilityPolicy)
	if err != nil {
		return nil, err
	}
	continueNode, err := newContinueNode()
	if err != nil {
		return nil, err
	}
	handoff, err := newHandoffNode()
	if err != nil {
		return nil, err
	}
	answer, err := newAnswerNode()
	if err != nil {
		return nil, err
	}
	approval, err := newApprovalNode(approvalResumeEnabled, cfg.Runtime.ApprovalSessionStore)
	if err != nil {
		return nil, err
	}
	degrade, err := newDegradeNode()
	if err != nil {
		return nil, err
	}

	nodes := []agentkernel.Node{prepare, observe, continueNode, handoff, answer, approval, degrade}
	var executionNodeName string
	if cfg.Runtime.PreferExternalEvidenceWorkflow {
		workflowNode, workflowErr := newExternalEvidenceNode(workflowCapability)
		if workflowErr != nil {
			return nil, workflowErr
		}
		nodes = append(nodes, workflowNode)
		executionNodeName = workflowNode.Name()
	} else {
		search, searchErr := newSearchNode(searchCapability)
		if searchErr != nil {
			return nil, searchErr
		}
		fetch, fetchErr := newFetchNode(fetchCapability)
		if fetchErr != nil {
			return nil, fetchErr
		}
		nodes = append(nodes, search, fetch)
		executionNodeName = fetch.Name()
	}
	for _, node := range nodes {
		if err := builder.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := builder.AddEdge(compose.START, prepare.Name()); err != nil {
		return nil, err
	}
	if err := builder.AddEdge(prepare.Name(), executionStartNodeName(cfg.Runtime.PreferExternalEvidenceWorkflow)); err != nil {
		return nil, err
	}
	if !cfg.Runtime.PreferExternalEvidenceWorkflow {
		if err := builder.AddEdge("search", "fetch"); err != nil {
			return nil, err
		}
	}
	if err := builder.AddEdge(executionNodeName, observe.Name()); err != nil {
		return nil, err
	}
	if err := builder.AddBranch(observe.Name(), branchOnEvidence, []string{handoff.Name(), answer.Name(), continueNode.Name(), approval.Name(), degrade.Name()}); err != nil {
		return nil, err
	}
	if err := builder.AddEdge(continueNode.Name(), executionStartNodeName(cfg.Runtime.PreferExternalEvidenceWorkflow)); err != nil {
		return nil, err
	}
	if err := builder.AddEdge(handoff.Name(), compose.END); err != nil {
		return nil, err
	}
	if err := builder.AddEdge(answer.Name(), compose.END); err != nil {
		return nil, err
	}
	if approvalResumeEnabled {
		approvalTargets := approvalBranchTargets(cfg.Runtime.PreferExternalEvidenceWorkflow)
		if err := builder.AddBranch(approval.Name(), branchAfterApproval, approvalTargets); err != nil {
			return nil, err
		}
	} else {
		if err := builder.AddEdge(approval.Name(), compose.END); err != nil {
			return nil, err
		}
	}
	if err := builder.AddEdge(degrade.Name(), compose.END); err != nil {
		return nil, err
	}

	return builder.Compile(ctx)
}

func requiredInterruptNodes(searchCapability agentcapability.Handle, fetchCapability agentcapability.Handle, workflowCapability agentcapability.Handle, enableApprovalResume bool) []string {
	nodes := make([]string, 0, 3)
	if enableApprovalResume {
		nodes = append(nodes, "approval")
	}
	if workflowCapability != nil {
		if requiresApproval(workflowCapability.Spec()) {
			nodes = append(nodes, "external_evidence")
		}
		return nodes
	}
	if searchCapability != nil && requiresApproval(searchCapability.Spec()) {
		nodes = append(nodes, "search")
	}
	if fetchCapability != nil && requiresApproval(fetchCapability.Spec()) {
		nodes = append(nodes, "fetch")
	}
	return nodes
}

func approvalBranchTargets(preferWorkflow bool) []string {
	if preferWorkflow {
		return []string{"external_evidence", "degrade"}
	}
	return []string{"search", "fetch", "degrade"}
}

func requiresApproval(spec agentcapability.Spec) bool {
	return spec.RequiresApproval
}

func resolveWorkflowCapability(registry *agentcapability.Registry, bindings agentcapability.RoleBindings) (agentcapability.Handle, error) {
	name, err := agentpattern.ResolveNamedBinding(registry, bindings, "reactive", agentcapability.RoleCollectExternalEvidence)
	if err != nil {
		return nil, err
	}
	handle, err := registry.Handle(name)
	if err != nil {
		return nil, err
	}
	if !handle.Spec().ProducesEvidence {
		return nil, fmt.Errorf("reactive external evidence capability %q must declare produces_evidence=true", name)
	}
	return handle, nil
}

func executionStartNodeName(preferWorkflow bool) string {
	if preferWorkflow {
		return "external_evidence"
	}
	return "search"
}
