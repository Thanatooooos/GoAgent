package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
	tracemod "local/rag-project/internal/app/rag/tool/modules/trace"
)

func mergeCheckedTools(groups ...[]string) []string {
	merged := make([]string, 0)
	for _, group := range groups {
		merged = append(merged, group...)
	}
	return UniqueTrimmedStrings(merged)
}

func toolNames(results []Result) []string {
	if len(results) == 0 {
		return nil
	}
	names := make([]string, 0, len(results))
	for _, result := range results {
		if name := strings.TrimSpace(result.Name); name != "" {
			names = append(names, name)
		}
	}
	return UniqueTrimmedStrings(names)
}

func firstNonEmptyHintCalls(groups ...[]HintCall) []HintCall {
	for _, group := range groups {
		group = NormalizeHintCalls(group)
		if len(group) > 0 {
			return group
		}
	}
	return nil
}

func validateHintAgainstEvidence(nextHintCalls []HintCall, input ObserveInput) bool {
	nextHintCalls = NormalizeHintCalls(nextHintCalls)
	if len(nextHintCalls) == 0 {
		return true
	}
	results := append([]Result(nil), input.Results...)
	results = append(results, input.RoundResults...)
	allowed := collectEvidenceIDs(input.Question, input.PreviousState.NextHintCalls, results)
	for _, hintCall := range nextHintCalls {
		if len(hintCall.Arguments) == 0 {
			return false
		}
		for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
			value := strings.TrimSpace(readStringArg(hintCall.Arguments, key))
			if value == "" {
				continue
			}
			if _, ok := allowed[value]; !ok {
				return false
			}
		}
	}
	return true
}

func validateCallAgainstEvidence(call Call, question string, previousHintCalls []HintCall, results []Result) bool {
	if strings.TrimSpace(call.Name) == "" {
		return false
	}
	allowed := collectEvidenceIDs(question, previousHintCalls, results)
	for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
		value := strings.TrimSpace(readStringArg(call.Arguments, key))
		if value == "" {
			continue
		}
		if _, ok := allowed[value]; !ok {
			return false
		}
	}
	return true
}

func collectEvidenceIDs(question string, previousHintCalls []HintCall, results []Result) map[string]struct{} {
	allowed := map[string]struct{}{}
	for _, id := range []string{
		firstMatchedID(documentIDPattern, question),
		firstMatchedID(taskIDPattern, question),
		firstMatchedID(traceIDPattern, question),
	} {
		if id != "" {
			allowed[id] = struct{}{}
		}
	}
	previousHintCalls = NormalizeHintCalls(previousHintCalls)
	for _, hintCall := range previousHintCalls {
		for _, key := range []string{"documentId", "taskId", "nodeId", "traceId"} {
			if value := strings.TrimSpace(readStringArg(hintCall.Arguments, key)); value != "" {
				allowed[value] = struct{}{}
			}
		}
	}
	for _, result := range results {
		collectEvidenceIDsFromResult(allowed, result)
	}
	return allowed
}

func collectEvidenceIDsFromResult(allowed map[string]struct{}, result Result) {
	if collectEvidenceIDsFromTypedView(allowed, result) {
		return
	}
	collectEvidenceIDsFromData(allowed, result.Data)
}

func collectEvidenceIDsFromTypedView(allowed map[string]struct{}, result Result) bool {
	if view, ok := systemmod.ViewDiagnosisResult(result); ok {
		addEvidenceIDs(allowed, view.TraceID, view.TaskID, view.LatestTaskID, view.LatestNodeID)
		return true
	}
	if view, ok := systemmod.ViewDocumentQueryResult(result); ok {
		addEvidenceIDs(allowed, view.DocumentID)
		return true
	}
	if view, ok := systemmod.ViewDocumentChunkLogQueryResult(result); ok {
		addEvidenceIDs(allowed, view.DocumentID, view.LatestTaskID)
		for _, item := range view.ChunkLogs {
			addEvidenceIDs(allowed, item.DocumentID, item.IngestionTaskID)
			addEvidenceIDs(allowed, item.FailedNodeIDs...)
			addEvidenceIDs(allowed, item.RunningNodeIDs...)
		}
		return true
	}
	if view, ok := systemmod.ViewIngestionTaskQueryResult(result); ok {
		addEvidenceIDs(allowed, view.TaskID)
		for _, node := range view.TaskNodeSummary {
			addEvidenceIDs(allowed, node.NodeID)
		}
		return true
	}
	if view, ok := systemmod.ViewIngestionTaskNodeQueryResult(result); ok {
		addEvidenceIDs(allowed, view.TaskID, view.NodeID)
		for _, node := range view.Nodes {
			addEvidenceIDs(allowed, node.NodeID)
		}
		return true
	}
	if view, ok := tracemod.ViewTraceNodeQueryResult(result); ok {
		addEvidenceIDs(allowed, view.TraceID, view.TaskID)
		for _, node := range view.Nodes {
			addEvidenceIDs(allowed, node.NodeID)
		}
		return true
	}
	if view, ok := tracemod.ViewTraceRetrievalDiagnoseResult(result); ok {
		addEvidenceIDs(allowed, view.TaskID, view.LatestTaskID, view.LatestNodeID)
		return true
	}
	return false
}

func collectEvidenceIDsFromData(allowed map[string]struct{}, data map[string]any) {
	if len(data) == 0 {
		return
	}
	for _, key := range []string{"documentId", "taskId", "nodeId", "traceId", "latestTaskId", "latestNodeId"} {
		if value := readDataString(data, key); value != "" {
			allowed[value] = struct{}{}
		}
	}
	for _, item := range readMapItems(data["taskNodeSummary"]) {
		if nodeID := strings.TrimSpace(readStringArg(item, "nodeId")); nodeID != "" {
			allowed[nodeID] = struct{}{}
		}
	}
	for _, item := range readMapItems(data["nodes"]) {
		if nodeID := strings.TrimSpace(readStringArg(item, "nodeId")); nodeID != "" {
			allowed[nodeID] = struct{}{}
		}
	}
}

func addEvidenceIDs(allowed map[string]struct{}, ids ...string) {
	for _, id := range ids {
		if id = strings.TrimSpace(id); id != "" {
			allowed[id] = struct{}{}
		}
	}
}
