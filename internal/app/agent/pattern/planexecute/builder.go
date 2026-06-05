package planexecute

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

// Compile builds the first runtime-native plan-execute graph.
func Compile(ctx context.Context, cfg Config) (*agentkernel.Runner, error) {
	if cfg.Assembly.Registry == nil {
		return nil, fmt.Errorf("plan-execute pattern requires capability registry")
	}
	bindings, err := agentpattern.ResolveNamedBindings(
		cfg.Assembly.Registry,
		cfg.Assembly.Bindings,
		"plan-execute",
		agentcapability.RoleSearch,
		agentcapability.RoleFetch,
	)
	if err != nil {
		return nil, err
	}
	searchName := bindings.Resolve(agentcapability.RoleSearch)
	fetchName := bindings.Resolve(agentcapability.RoleFetch)
	searchSpec, _ := cfg.Assembly.Registry.Spec(searchName)
	fetchSpec, _ := cfg.Assembly.Registry.Spec(fetchName)

	kernelCfg := cfg.Runtime.Kernel
	if strings.TrimSpace(kernelCfg.GraphName) == "" {
		kernelCfg.GraphName = "agent_pattern_plan_execute"
	}
	if kernelCfg.Reducer == nil {
		kernelCfg.Reducer = agentstate.DefaultReducer{}
	}
	approvalResumeEnabled := kernelCfg.CheckpointStore != nil
	kernelCfg.InterruptBeforeNodes = agentpattern.MergeInterruptBeforeNodes(kernelCfg.InterruptBeforeNodes, requiredInterruptBeforeNodes(approvalResumeEnabled))

	builder := agentkernel.NewBuilder(kernelCfg)
	buildPlan, err := newBuildPlanNode(
		cfg.Assembly.Registry,
		searchSpec,
		fetchSpec,
		cfg.Runtime.CapabilityCatalogBuilder,
		cfg.Runtime.CapabilitySelector,
		cfg.Runtime.CapabilityResolver,
	)
	if err != nil {
		return nil, err
	}
	selectStep, err := newSelectStepNode()
	if err != nil {
		return nil, err
	}
	executeStep, err := newExecuteStepNode(cfg.Assembly.Registry, cfg.Runtime.CapabilityResolver)
	if err != nil {
		return nil, err
	}
	assessStep, err := newAssessStepNode()
	if err != nil {
		return nil, err
	}
	approval, err := newApprovalNode(approvalResumeEnabled, cfg.Runtime.ApprovalSessionStore)
	if err != nil {
		return nil, err
	}
	finalize, err := newFinalizeNode(cfg.Runtime.OutputMode)
	if err != nil {
		return nil, err
	}
	nodes := []agentkernel.Node{buildPlan, selectStep, executeStep, assessStep, approval, finalize}
	for _, node := range nodes {
		if err := builder.AddNode(node); err != nil {
			return nil, err
		}
	}
	if err := builder.AddEdge(compose.START, "build_plan"); err != nil {
		return nil, err
	}
	if err := builder.AddEdge("build_plan", "select_step"); err != nil {
		return nil, err
	}
	if err := builder.AddBranch("select_step", branchAfterSelection, []string{"execute_step", "approval", "finalize"}); err != nil {
		return nil, err
	}
	if err := builder.AddEdge("execute_step", "assess_step"); err != nil {
		return nil, err
	}
	if err := builder.AddBranch("assess_step", branchAfterAssessment, []string{"select_step", "build_plan", "approval", "finalize"}); err != nil {
		return nil, err
	}
	if approvalResumeEnabled {
		if err := builder.AddBranch("approval", branchAfterApproval, []string{"execute_step", "finalize"}); err != nil {
			return nil, err
		}
	} else {
		if err := builder.AddEdge("approval", compose.END); err != nil {
			return nil, err
		}
	}
	if err := builder.AddEdge("finalize", compose.END); err != nil {
		return nil, err
	}
	return builder.Compile(ctx)
}

func requiredInterruptBeforeNodes(enableApprovalResume bool) []string {
	if !enableApprovalResume {
		return nil
	}
	return []string{"approval"}
}
