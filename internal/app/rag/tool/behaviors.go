package tool

import (
	graphmod "local/rag-project/internal/app/rag/tool/modules/graph"
	metamod "local/rag-project/internal/app/rag/tool/modules/meta"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
	tracemod "local/rag-project/internal/app/rag/tool/modules/trace"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

func ThinkBehavior() ToolBehavior {
	return metamod.ThinkBehavior()
}

func WebSearchBehavior() ToolBehavior {
	behavior := webmod.WebSearchBehavior()
	behavior.RenderContext = renderWebSearchContextCompat
	return behavior
}

func WebFetchBehavior() ToolBehavior {
	behavior := webmod.WebFetchBehavior()
	behavior.RenderContext = renderWebFetchContextCompat
	behavior.BuildGuidance = func(_ Result, input GuidanceInput) []GuidanceNote {
		return runtimeGuidanceNotes(input.AllResults)
	}
	return behavior
}

func ExternalEvidenceWorkflowBehavior() ToolBehavior {
	behavior := webmod.ExternalEvidenceWorkflowBehavior()
	behavior.RenderContext = renderExternalEvidenceContextCompat
	behavior.BuildGuidance = func(_ Result, input GuidanceInput) []GuidanceNote {
		return runtimeGuidanceNotes(input.AllResults)
	}
	return behavior
}

func DocumentQueryBehavior() ToolBehavior {
	return systemmod.DocumentQueryBehavior()
}

func DocumentChunkLogQueryBehavior() ToolBehavior {
	return systemmod.DocumentChunkLogQueryBehavior()
}

func DocumentListBehavior() ToolBehavior {
	return systemmod.DocumentListBehavior()
}

func TaskListBehavior() ToolBehavior {
	return systemmod.TaskListBehavior()
}

func IngestionTaskQueryBehavior() ToolBehavior {
	return systemmod.IngestionTaskQueryBehavior()
}

func IngestionTaskNodeQueryBehavior() ToolBehavior {
	return systemmod.IngestionTaskNodeQueryBehavior()
}

func DocumentIngestionDiagnoseBehavior() ToolBehavior {
	behavior := systemmod.DocumentIngestionDiagnoseBehavior()
	behavior.BuildGuidance = func(_ Result, input GuidanceInput) []GuidanceNote {
		return runtimeGuidanceNotes(input.AllResults)
	}
	return behavior
}

func TaskIngestionDiagnoseBehavior() ToolBehavior {
	behavior := systemmod.TaskIngestionDiagnoseBehavior()
	behavior.BuildGuidance = func(_ Result, input GuidanceInput) []GuidanceNote {
		return runtimeGuidanceNotes(input.AllResults)
	}
	return behavior
}

func TraceNodeQueryBehavior() ToolBehavior {
	behavior := tracemod.TraceNodeQueryBehavior()
	behavior.RenderContext = renderTraceNodeQueryContextCompat
	return behavior
}

func TraceRetrievalDiagnoseBehavior() ToolBehavior {
	return tracemod.TraceRetrievalDiagnoseBehavior()
}

func DocumentRootCauseDiagnosisBehavior() ToolBehavior {
	behavior := graphmod.DocumentRootCauseDiagnosisBehavior()
	behavior.RenderContext = renderDocumentRootCauseDiagnosisContextCompat
	behavior.BuildGuidance = func(result Result, _ GuidanceInput) []GuidanceNote {
		return buildDocumentRootCauseDiagnosisGuidanceNotes(result)
	}
	return behavior
}

func DocumentDiagnoseWithSearchBehavior() ToolBehavior {
	behavior := graphmod.DocumentDiagnoseWithSearchBehavior()
	behavior.RenderContext = renderDocumentDiagnoseWithSearchContextCompat
	behavior.BuildGuidance = func(result Result, input GuidanceInput) []GuidanceNote {
		return buildDocumentDiagnoseWithSearchGuidanceNotes(result, input.AllResults)
	}
	return behavior
}
