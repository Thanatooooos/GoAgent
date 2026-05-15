package system

import (
	"strings"

	ragcore "local/rag-project/internal/app/rag/tool/core"
)

type DiagnosisResultView struct {
	Conclusion      string
	Confidence      string
	Facts           []string
	Inferences      []string
	RiskHints       []string
	NextActions     []string
	TaskID          string
	LatestTaskID    string
	LatestNodeID    string
	LatestNodeError string
}

func ViewDiagnosisResult(result ragcore.Result) (DiagnosisResultView, bool) {
	switch strings.TrimSpace(result.Name) {
	case "document_ingestion_diagnose", "task_ingestion_diagnose", "trace_retrieval_diagnose":
	default:
		return DiagnosisResultView{}, false
	}

	return DiagnosisResultView{
		Conclusion:      result.GetString("conclusion"),
		Confidence:      result.GetString("confidence"),
		Facts:           result.PreferStringSlice("facts", "evidence"),
		Inferences:      result.GetStringSlice("inferences"),
		RiskHints:       result.GetStringSlice("riskHints"),
		NextActions:     result.PreferStringSlice("nextActions", "suggestions"),
		TaskID:          result.GetString("taskId"),
		LatestTaskID:    result.GetString("latestTaskId"),
		LatestNodeID:    result.GetString("latestNodeId"),
		LatestNodeError: result.GetString("latestNodeError"),
	}, true
}
