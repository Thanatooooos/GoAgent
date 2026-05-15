package tool

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

// Type aliases — the canonical definitions live in core.
type ToolInvoker = ragcore.ToolInvoker
type ToolSpec = ragcore.ToolSpec
type GuidanceInput = ragcore.GuidanceInput
type GuidanceNote = ragcore.GuidanceNote
type NextDecision = ragcore.NextDecision
type ToolBehavior = ragcore.ToolBehavior
type ToolModule = ragcore.ToolModule

// LegacyToolAdapter bridges a legacy Tool into a ToolModule.
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
	if isEmptyToolBehavior(behavior) {
		behavior = inferLegacyBehavior(firstNonEmpty(strings.TrimSpace(spec.Definition.Name), legacyToolName(tool)))
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

// inferLegacyBehavior returns empty ToolBehavior — production uses Registry.GetBehavior instead.
func inferLegacyBehavior(name string) ToolBehavior {
	return ToolBehavior{}
}

// inferLegacyToolSpec returns empty ToolSpec — production uses Registry.GetSpec instead.
func inferLegacyToolSpec(name string) (ToolSpec, bool) {
	return ToolSpec{}, false
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
