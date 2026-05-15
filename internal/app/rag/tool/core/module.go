package core

import (
	"context"
	"fmt"
	"strings"
)

type ToolInvoker interface {
	Invoke(ctx context.Context, call Call) (Result, error)
}

type ToolSpec struct {
	Definition          Definition
	Capability          string
	EvidenceSources     []string
	ExecutionMode       string
	RiskLevel           string
	ApprovalRequirement string
	ReadOnly            bool
	Family              string
}

func (s ToolSpec) Normalize() ToolSpec {
	s.Definition.Name = strings.TrimSpace(s.Definition.Name)
	s.Definition.Description = strings.TrimSpace(s.Definition.Description)
	s.Capability = strings.TrimSpace(strings.ToLower(s.Capability))
	s.EvidenceSources = UniqueTrimmedStrings(s.EvidenceSources)
	s.ExecutionMode = strings.TrimSpace(strings.ToLower(s.ExecutionMode))
	s.RiskLevel = strings.TrimSpace(strings.ToLower(s.RiskLevel))
	s.ApprovalRequirement = strings.TrimSpace(strings.ToLower(s.ApprovalRequirement))
	s.Family = strings.TrimSpace(strings.ToLower(s.Family))
	if s.ReadOnly {
		s.Definition.ReadOnly = true
	} else if s.Definition.ReadOnly {
		s.ReadOnly = true
	}
	if s.ExecutionMode == "" {
		if s.ReadOnly {
			s.ExecutionMode = ExecutionModeReadOnly
		} else {
			s.ExecutionMode = ExecutionModeProposalOnly
		}
	}
	return s
}

func (s ToolSpec) ResultMeta() ResultMeta {
	normalized := s.Normalize()
	return ResultMeta{
		Capability:          normalized.Capability,
		EvidenceSources:     append([]string(nil), normalized.EvidenceSources...),
		ExecutionMode:       normalized.ExecutionMode,
		RiskLevel:           normalized.RiskLevel,
		ApprovalRequirement: normalized.ApprovalRequirement,
		ReadOnly:            normalized.ReadOnly,
		Family:              normalized.Family,
	}
}

type GuidanceInput struct {
	AllResults []Result
}

type GuidanceNote struct {
	Section string
	Text    string
}

type NextDecision struct {
	HintCalls []HintCall
	Done      bool
	Reason    string
	Terminal  bool
	Retryable bool
}

type ToolBehavior struct {
	Decode        func(result Result) (any, error)
	Next          func(result Result, input WorkflowInput) NextDecision
	Observe       func(result Result, input ObserveInput) (ObserveResult, bool)
	RenderContext func(result Result) string
	BuildGuidance func(result Result, input GuidanceInput) []GuidanceNote
}

type ToolModule struct {
	Name     string
	Invoker  ToolInvoker
	Spec     ToolSpec
	Behavior ToolBehavior
}

func (m ToolModule) Normalize() ToolModule {
	m.Name = strings.TrimSpace(m.Name)
	m.Spec = m.Spec.Normalize()
	if m.Name == "" {
		m.Name = strings.TrimSpace(m.Spec.Definition.Name)
	}
	if strings.TrimSpace(m.Spec.Definition.Name) == "" {
		m.Spec.Definition.Name = m.Name
	}
	return m
}

func (m ToolModule) Validate() error {
	m = m.Normalize()
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("tool module name is required")
	}
	if m.Invoker == nil {
		return fmt.Errorf("tool module %q invoker is required", m.Name)
	}
	return m.Spec.Definition.Validate()
}

type LegacyToolAdapter struct {
	tool     Tool
	spec     ToolSpec
	behavior ToolBehavior
}

func NewLegacyToolAdapter(tool Tool) LegacyToolAdapter {
	return NewLegacyToolAdapterWithSpec(tool, ToolSpec{})
}

func NewLegacyToolAdapterWithSpec(tool Tool, spec ToolSpec) LegacyToolAdapter {
	return NewLegacyToolAdapterWithBehavior(tool, spec, ToolBehavior{})
}

// InferBehavior is an optional hook set by downstream packages to provide
// behavior inference for legacy tools. When set, NewLegacyToolAdapter calls
// this function when the supplied behavior is empty.
var InferBehavior func(name string) ToolBehavior

// SetInferBehavior wires InferBehavior to use the given Registry for behavior lookup.
func SetInferBehavior(r *Registry) {
	InferBehavior = func(name string) ToolBehavior {
		if behavior, ok := r.GetBehavior(name); ok {
			return behavior
		}
		return ToolBehavior{}
	}
}

func NewLegacyToolAdapterWithBehavior(tool Tool, spec ToolSpec, behavior ToolBehavior) LegacyToolAdapter {
	if tool != nil {
		definition := tool.Definition()
		if strings.TrimSpace(spec.Definition.Name) == "" {
			spec.Definition = definition
		}
		if spec.ReadOnly || definition.ReadOnly {
			spec.ReadOnly = true
			spec.Definition.ReadOnly = true
		}
	}
	if isEmptyToolBehavior(behavior) && InferBehavior != nil {
		name := strings.TrimSpace(spec.Definition.Name)
		if name == "" && tool != nil {
			name = strings.TrimSpace(tool.Definition().Name)
		}
		if name != "" {
			behavior = InferBehavior(name)
		}
	}
	return LegacyToolAdapter{
		tool:     tool,
		spec:     spec,
		behavior: behavior,
	}
}

func isEmptyToolBehavior(behavior ToolBehavior) bool {
	return behavior.Decode == nil &&
		behavior.Next == nil &&
		behavior.Observe == nil &&
		behavior.RenderContext == nil &&
		behavior.BuildGuidance == nil
}

func legacyToolName(tool Tool) string {
	if tool == nil {
		return ""
	}
	return strings.TrimSpace(tool.Definition().Name)
}

func (a LegacyToolAdapter) Module() ToolModule {
	if a.tool == nil {
		return ToolModule{}
	}
	spec := a.spec.Normalize()
	if strings.TrimSpace(spec.Definition.Name) == "" {
		spec.Definition = a.tool.Definition()
		spec = spec.Normalize()
	}
	return ToolModule{
		Name:     strings.TrimSpace(spec.Definition.Name),
		Invoker:  a.tool,
		Spec:     spec,
		Behavior: a.behavior,
	}.Normalize()
}
