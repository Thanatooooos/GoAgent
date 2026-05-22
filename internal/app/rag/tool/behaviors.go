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
	return webmod.WebSearchBehavior()
}

func WebFetchBehavior() ToolBehavior {
	return webmod.WebFetchBehavior()
}

func ExternalEvidenceWorkflowBehavior() ToolBehavior {
	return webmod.ExternalEvidenceWorkflowBehavior()
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
	return systemmod.DocumentIngestionDiagnoseBehavior()
}

func TaskIngestionDiagnoseBehavior() ToolBehavior {
	return systemmod.TaskIngestionDiagnoseBehavior()
}

func TraceNodeQueryBehavior() ToolBehavior {
	return tracemod.TraceNodeQueryBehavior()
}

func TraceRetrievalDiagnoseBehavior() ToolBehavior {
	return tracemod.TraceRetrievalDiagnoseBehavior()
}

func DocumentRootCauseDiagnosisBehavior() ToolBehavior {
	return graphmod.DocumentRootCauseDiagnosisBehavior()
}

func DocumentDiagnoseWithSearchBehavior() ToolBehavior {
	return graphmod.DocumentDiagnoseWithSearchBehavior()
}
