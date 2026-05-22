package tool

import (
	graphmod "local/rag-project/internal/app/rag/tool/modules/graph"
	metamod "local/rag-project/internal/app/rag/tool/modules/meta"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
	tracemod "local/rag-project/internal/app/rag/tool/modules/trace"
	webmod "local/rag-project/internal/app/rag/tool/modules/web"
)

// Type aliases for module view types.

type DiagnosisResultView = systemmod.DiagnosisResultView
type DocumentQueryResultView = systemmod.DocumentQueryResultView
type DocumentChunkLogItemView = systemmod.DocumentChunkLogItemView
type DocumentChunkLogQueryResultView = systemmod.DocumentChunkLogQueryResultView
type DocumentListItemView = systemmod.DocumentListItemView
type DocumentListResultView = systemmod.DocumentListResultView
type IngestionTaskNodeSummaryView = systemmod.IngestionTaskNodeSummaryView
type IngestionTaskQueryResultView = systemmod.IngestionTaskQueryResultView
type IngestionTaskNodeItemView = systemmod.IngestionTaskNodeItemView
type IngestionTaskNodeQueryResultView = systemmod.IngestionTaskNodeQueryResultView
type TaskListItemView = systemmod.TaskListItemView
type TaskListResultView = systemmod.TaskListResultView

type WebSearchItemView = webmod.WebSearchItemView
type WebSearchResultView = webmod.WebSearchResultView
type WebFetchPageView = webmod.WebFetchPageView
type WebFetchResultView = webmod.WebFetchResultView
type ExternalEvidenceSourceItemView = webmod.ExternalEvidenceSourceItemView
type ExternalEvidenceSourceReviewView = webmod.ExternalEvidenceSourceReviewView
type ExternalEvidenceQualityView = webmod.ExternalEvidenceQualityView
type ExternalEvidenceWorkflowView = webmod.ExternalEvidenceWorkflowView

type ThinkResultView = metamod.ThinkResultView

type TraceNodeItemView = tracemod.TraceNodeItemView
type TraceNodeMemoryRecallSummaryView = tracemod.TraceNodeMemoryRecallSummaryView
type TraceNodeQueryResultView = tracemod.TraceNodeQueryResultView
type TraceRetrievalDiagnoseResultView = tracemod.TraceRetrievalDiagnoseResultView

type DocumentRootCauseDiagnosisView = graphmod.DocumentRootCauseDiagnosisView
type DocumentDiagnoseWithSearchView = graphmod.DocumentDiagnoseWithSearchView

// View helpers.

func ViewDiagnosisResult(result Result) (DiagnosisResultView, bool) {
	return systemmod.ViewDiagnosisResult(result)
}

func ViewDocumentQueryResult(result Result) (DocumentQueryResultView, bool) {
	return systemmod.ViewDocumentQueryResult(result)
}

func ViewDocumentChunkLogQueryResult(result Result) (DocumentChunkLogQueryResultView, bool) {
	return systemmod.ViewDocumentChunkLogQueryResult(result)
}

func ViewDocumentListResult(result Result) (DocumentListResultView, bool) {
	return systemmod.ViewDocumentListResult(result)
}

func ViewIngestionTaskQueryResult(result Result) (IngestionTaskQueryResultView, bool) {
	return systemmod.ViewIngestionTaskQueryResult(result)
}

func ViewIngestionTaskNodeQueryResult(result Result) (IngestionTaskNodeQueryResultView, bool) {
	return systemmod.ViewIngestionTaskNodeQueryResult(result)
}

func ViewTaskListResult(result Result) (TaskListResultView, bool) {
	return systemmod.ViewTaskListResult(result)
}

func ViewWebSearchResult(result Result) (WebSearchResultView, bool) {
	return webmod.ViewWebSearchResult(result)
}

func ViewWebFetchResult(result Result) (WebFetchResultView, bool) {
	return webmod.ViewWebFetchResult(result)
}

func ViewExternalEvidenceWorkflowResult(result Result) (ExternalEvidenceWorkflowView, bool) {
	return webmod.ViewExternalEvidenceWorkflowResult(result)
}

func ViewTraceNodeQueryResult(result Result) (TraceNodeQueryResultView, bool) {
	return tracemod.ViewTraceNodeQueryResult(result)
}

func ViewTraceRetrievalDiagnoseResult(result Result) (TraceRetrievalDiagnoseResultView, bool) {
	return tracemod.ViewTraceRetrievalDiagnoseResult(result)
}

func ViewDocumentRootCauseDiagnosisResult(result Result) (DocumentRootCauseDiagnosisView, bool) {
	return graphmod.ViewDocumentRootCauseDiagnosisResult(result)
}

func ViewDocumentDiagnoseWithSearchResult(result Result) (DocumentDiagnoseWithSearchView, bool) {
	return graphmod.ViewDocumentDiagnoseWithSearchResult(result)
}
