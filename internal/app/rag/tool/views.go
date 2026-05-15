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
type TraceNodeQueryResultView = tracemod.TraceNodeQueryResultView

type DocumentRootCauseDiagnosisView = graphmod.DocumentRootCauseDiagnosisView
type DocumentDiagnoseWithSearchView = graphmod.DocumentDiagnoseWithSearchView

// View helpers.

func ViewDiagnosisResult(result Result) (DiagnosisResultView, bool) {
	return systemmod.ViewDiagnosisResult(result)
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

func ViewDocumentRootCauseDiagnosisResult(result Result) (DocumentRootCauseDiagnosisView, bool) {
	return graphmod.ViewDocumentRootCauseDiagnosisResult(result)
}

func ViewDocumentDiagnoseWithSearchResult(result Result) (DocumentDiagnoseWithSearchView, bool) {
	return graphmod.ViewDocumentDiagnoseWithSearchResult(result)
}
