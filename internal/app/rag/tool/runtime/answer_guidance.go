package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
)

func BuildAnswerGuidance(results []Result) string {
	if diagnosis, ok := selectDiagnosisResult(results); ok {
		return buildDiagnosisGuidance(diagnosis, results)
	}
	if externalEvidence, ok := selectExternalEvidenceResult(results); ok {
		return buildExternalEvidenceGuidance(results, externalEvidence)
	}
	if webResults := selectWebResults(results); len(webResults) > 0 {
		return buildWebSearchGuidance(results, webResults)
	}
	return ""
}

func selectDiagnosisResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		switch strings.TrimSpace(results[idx].Name) {
		case "document_ingestion_diagnose", "task_ingestion_diagnose", "trace_retrieval_diagnose":
			return results[idx], true
		}
	}
	return Result{}, false
}

func selectExternalEvidenceResult(results []Result) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		if strings.TrimSpace(results[idx].Name) == "external_evidence_workflow" {
			return results[idx], true
		}
	}
	return Result{}, false
}

func selectWebResults(results []Result) []Result {
	webResults := make([]Result, 0)
	for idx := len(results) - 1; idx >= 0; idx-- {
		name := strings.TrimSpace(results[idx].Name)
		if name == "web_search" || name == "web_fetch" {
			webResults = append(webResults, results[idx])
		}
	}
	return webResults
}
