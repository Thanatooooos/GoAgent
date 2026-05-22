package tool

import ragcore "local/rag-project/internal/app/rag/tool/core"

// Type aliases - the canonical definitions live in core.
type ToolInvoker = ragcore.ToolInvoker
type ToolSpec = ragcore.ToolSpec
type GuidanceInput = ragcore.GuidanceInput
type GuidanceNote = ragcore.GuidanceNote
type NextDecision = ragcore.NextDecision
type ToolBehavior = ragcore.ToolBehavior
type ToolModule = ragcore.ToolModule
type LegacyToolAdapter = ragcore.LegacyToolAdapter

func NewLegacyToolAdapter(tool Tool) LegacyToolAdapter {
	return ragcore.NewLegacyToolAdapter(tool)
}

func NewLegacyToolAdapterWithSpec(tool Tool, spec ToolSpec) LegacyToolAdapter {
	return ragcore.NewLegacyToolAdapterWithSpec(tool, spec)
}

func NewLegacyToolAdapterWithBehavior(tool Tool, spec ToolSpec, behavior ToolBehavior) LegacyToolAdapter {
	return ragcore.NewLegacyToolAdapterWithBehavior(tool, spec, behavior)
}
