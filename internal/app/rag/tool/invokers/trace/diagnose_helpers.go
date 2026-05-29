package builtin

import (
	"encoding/json"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	ragdomain "local/rag-project/internal/app/rag/domain"
)

const (
	diagnosisConfidenceHigh   = "high"
	diagnosisConfidenceMedium = "medium"
	diagnosisConfidenceLow    = "low"
)

type ingestionNodeStats struct {
	Total         int
	SuccessCount  int
	FailedCount   int
	RunningCount  int
	PendingCount  int
	LastNodeID    string
	LastStatus    string
	FailedNodeID  string
	FailedError   string
	RunningNodeID string
}

func summarizeIngestionNodes(nodes []ingestiondomain.TaskNode) ingestionNodeStats {
	stats := ingestionNodeStats{Total: len(nodes)}
	for _, node := range nodes {
		nodeID := strings.TrimSpace(node.NodeID)
		status := strings.TrimSpace(node.Status)
		if nodeID != "" {
			stats.LastNodeID = nodeID
		}
		if status != "" {
			stats.LastStatus = status
		}
		switch status {
		case ingestiondomain.TaskStatusSuccess:
			stats.SuccessCount++
		case ingestiondomain.TaskStatusFailed:
			stats.FailedCount++
			if stats.FailedNodeID == "" {
				stats.FailedNodeID = nodeID
				stats.FailedError = strings.TrimSpace(node.ErrorMessage)
			}
		case ingestiondomain.TaskStatusRunning:
			stats.RunningCount++
			if stats.RunningNodeID == "" {
				stats.RunningNodeID = nodeID
			}
		case ingestiondomain.TaskStatusPending:
			stats.PendingCount++
		}
	}
	return stats
}

func appendNodeStatsEvidence(evidence []string, stats ingestionNodeStats, prefix string) []string {
	evidence = append(evidence, fmt.Sprintf("%s.total=%d", prefix, stats.Total))
	evidence = append(evidence, fmt.Sprintf("%s.success=%d", prefix, stats.SuccessCount))
	evidence = append(evidence, fmt.Sprintf("%s.failed=%d", prefix, stats.FailedCount))
	evidence = append(evidence, fmt.Sprintf("%s.running=%d", prefix, stats.RunningCount))
	if stats.PendingCount > 0 {
		evidence = append(evidence, fmt.Sprintf("%s.pending=%d", prefix, stats.PendingCount))
	}
	if stats.LastNodeID != "" {
		evidence = append(evidence, fmt.Sprintf("%s.lastNode=%s", prefix, stats.LastNodeID))
	}
	if stats.LastStatus != "" {
		evidence = append(evidence, fmt.Sprintf("%s.lastStatus=%s", prefix, stats.LastStatus))
	}
	return evidence
}

func hasInconsistentIngestionState(taskStatus string, logStatus string, documentStatus string) bool {
	taskStatus = strings.TrimSpace(taskStatus)
	logStatus = strings.TrimSpace(logStatus)
	documentStatus = strings.TrimSpace(documentStatus)

	if taskStatus == ingestiondomain.TaskStatusFailed && (logStatus == "success" || documentStatus == "success") {
		return true
	}
	if taskStatus == ingestiondomain.TaskStatusSuccess && (logStatus == "failed" || documentStatus == "failed") {
		return true
	}
	if logStatus == "failed" && documentStatus == "success" {
		return true
	}
	if logStatus == "success" && documentStatus == "failed" {
		return true
	}
	if taskStatus == ingestiondomain.TaskStatusRunning && (logStatus == "failed" || documentStatus == "failed") {
		return true
	}
	return false
}

func readTraceExtra(raw string) map[string]any {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return nil
	}
	return payload
}

func readTraceExtraString(raw string, key string) string {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return ""
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", typed))
	}
}

func readTraceExtraInt(raw string, key string) int {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return -1
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return -1
	}
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	default:
		return -1
	}
}

func readTraceExtraFloat(raw string, key string) float64 {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return -1
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return -1
	}
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	default:
		return -1
	}
}

func readTraceExtraBool(raw string, key string) bool {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return false
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return false
	}
	typed, ok := value.(bool)
	return ok && typed
}

func readTraceExtraStringSlice(raw string, key string) []string {
	payload := readTraceExtra(raw)
	if len(payload) == 0 {
		return nil
	}
	value, ok := payload[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			text := strings.TrimSpace(fmt.Sprintf("%v", item))
			if text != "" {
				items = append(items, text)
			}
		}
		return items
	case []string:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(item); text != "" {
				items = append(items, text)
			}
		}
		return items
	default:
		return nil
	}
}

func findToolWorkflowNode(nodes []ragdomain.RagTraceNode) *ragdomain.RagTraceNode {
	return findTraceNode(nodes, "tool_workflow")
}

func normalizeDiagnosisConfidence(confidence string) string {
	switch strings.TrimSpace(strings.ToLower(confidence)) {
	case diagnosisConfidenceHigh:
		return diagnosisConfidenceHigh
	case diagnosisConfidenceMedium:
		return diagnosisConfidenceMedium
	default:
		return diagnosisConfidenceLow
	}
}

func buildDiagnosisPayload(scope string, conclusion string, confidence string, evidence []string, suggestions []string, extra map[string]any) map[string]any {
	rawEvidence := normalizeStringList(evidence)
	facts := humanizeDiagnosisFacts(scope, rawEvidence)
	nextActions := normalizeStringList(suggestions)
	inferences := deriveDiagnosisInferences(conclusion, confidence)
	riskHints := deriveDiagnosisRiskHints(conclusion, confidence)

	data := map[string]any{
		"diagnosisScope": scope,
		"conclusion":     strings.TrimSpace(conclusion),
		"confidence":     normalizeDiagnosisConfidence(confidence),
		"evidence":       rawEvidence,
		"rawEvidence":    rawEvidence,
		"facts":          facts,
		"inferences":     inferences,
		"riskHints":      riskHints,
		"suggestions":    nextActions,
		"nextActions":    nextActions,
	}
	for key, value := range extra {
		data[key] = value
	}
	return data
}

func humanizeDiagnosisFacts(scope string, evidence []string) []string {
	scope = strings.TrimSpace(scope)
	evidence = normalizeStringList(evidence)
	if len(evidence) == 0 {
		return []string{}
	}

	values := map[string]string{}
	for _, item := range evidence {
		key, value, ok := strings.Cut(item, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		values[key] = value
	}

	facts := make([]string, 0, 6)
	appendFact := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		for _, existing := range facts {
			if existing == text {
				return
			}
		}
		facts = append(facts, text)
	}

	switch scope {
	case "task_ingestion":
		appendFact(humanizeTaskStatus(values))
		appendFact(humanizeTaskFailureNode(values))
		appendFact(humanizeTaskLastNode(values))
		appendFact(humanizeTaskNodeCounts(values))
	case "document_ingestion":
		appendFact(humanizeDocumentStatus(values))
		appendFact(humanizeChunkLogStatus(values))
		appendFact(humanizeTaskFailureNode(values))
		appendFact(humanizeTaskNodeCountsWithPrefix(values, "ingestionNodes"))
	case "trace_retrieval":
		appendFact(humanizeTraceStatus(values))
		appendFact(humanizeRetrieveStats(values))
		appendFact(humanizeLongTermMemory(values))
		appendFact(humanizeSessionRecall(values))
		appendFact(humanizeToolWorkflow(values))
		appendFact(humanizeFailedTraceNode(values))
	}

	if len(facts) > 0 {
		return facts
	}

	fallback := make([]string, 0, len(evidence))
	for _, item := range evidence {
		fallback = append(fallback, humanizeGenericEvidence(item))
	}
	return fallback
}

func humanizeTaskStatus(values map[string]string) string {
	status := values["task.status"]
	if status == "" {
		return ""
	}
	return fmt.Sprintf("任务当前状态为%s。", translateStatus(status))
}

func humanizeDocumentStatus(values map[string]string) string {
	status := values["document.status"]
	processMode := values["document.processMode"]
	if status == "" && processMode == "" {
		return ""
	}
	if status != "" && processMode != "" {
		return fmt.Sprintf("文档当前状态为%s，处理模式是 %s。", translateStatus(status), processMode)
	}
	if status != "" {
		return fmt.Sprintf("文档当前状态为%s。", translateStatus(status))
	}
	return fmt.Sprintf("文档处理模式是 %s。", processMode)
}

func humanizeChunkLogStatus(values map[string]string) string {
	status := values["latestChunkLog.status"]
	taskID := values["latestChunkLog.taskId"]
	errMsg := values["latestChunkLog.error"]
	if status == "" && taskID == "" && errMsg == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	if taskID != "" {
		parts = append(parts, fmt.Sprintf("最近一次 chunk log 对应的任务是 %s", taskID))
	}
	if status != "" {
		parts = append(parts, fmt.Sprintf("状态为%s", translateStatus(status)))
	}
	if errMsg != "" {
		parts = append(parts, fmt.Sprintf("记录的错误是 %q", errMsg))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, "，") + "。"
}

func humanizeTaskFailureNode(values map[string]string) string {
	nodeID := firstNonEmptyValue(values["failedNode"], values["runningNode"])
	if nodeID == "" {
		return ""
	}
	statusText := "当前聚焦的节点"
	if values["failedNode"] != "" {
		statusText = "失败节点"
	} else if values["runningNode"] != "" {
		statusText = "当前运行中的节点"
	}
	errMsg := values["failedNode.error"]
	if errMsg != "" {
		return fmt.Sprintf("%s是 %s，节点报错为 %q。", statusText, nodeID, errMsg)
	}
	return fmt.Sprintf("%s是 %s。", statusText, nodeID)
}

func humanizeTaskLastNode(values map[string]string) string {
	nodeID := firstNonEmptyValue(values["taskNodes.lastNode"], values["ingestionNodes.lastNode"])
	status := firstNonEmptyValue(values["taskNodes.lastStatus"], values["ingestionNodes.lastStatus"])
	if nodeID == "" && status == "" {
		return ""
	}
	if nodeID != "" && status != "" {
		return fmt.Sprintf("最后推进到的节点是 %s，该节点状态为%s。", nodeID, translateStatus(status))
	}
	if nodeID != "" {
		return fmt.Sprintf("最后推进到的节点是 %s。", nodeID)
	}
	return fmt.Sprintf("最后一个节点状态为%s。", translateStatus(status))
}

func humanizeTaskNodeCounts(values map[string]string) string {
	return humanizeTaskNodeCountsWithPrefix(values, "taskNodes")
}

func humanizeTaskNodeCountsWithPrefix(values map[string]string, prefix string) string {
	total := values[prefix+".total"]
	success := values[prefix+".success"]
	failed := values[prefix+".failed"]
	running := values[prefix+".running"]
	pending := values[prefix+".pending"]
	if total == "" && success == "" && failed == "" && running == "" && pending == "" {
		return ""
	}
	parts := make([]string, 0, 4)
	if total != "" {
		parts = append(parts, fmt.Sprintf("共有 %s 个节点", total))
	}
	if success != "" {
		parts = append(parts, fmt.Sprintf("%s 个成功", success))
	}
	if failed != "" && failed != "0" {
		parts = append(parts, fmt.Sprintf("%s 个失败", failed))
	}
	if running != "" && running != "0" {
		parts = append(parts, fmt.Sprintf("%s 个仍在运行", running))
	}
	if pending != "" && pending != "0" {
		parts = append(parts, fmt.Sprintf("%s 个未开始", pending))
	}
	if len(parts) == 0 {
		return ""
	}
	sentence := strings.Join(parts, "，")
	lastNode := firstNonEmptyValue(values[prefix+".lastNode"], values["failedNode"], values["runningNode"])
	if lastNode != "" && (lastNode == "indexer" || lastNode == "chunker" || lastNode == "parser" || lastNode == "fetcher") {
		sentence += "，说明流程已经推进到 " + lastNode + " 阶段"
	}
	return sentence + "。"
}

func humanizeTraceStatus(values map[string]string) string {
	status := values["trace.status"]
	nodeCount := values["trace.nodeCount"]
	if status == "" && nodeCount == "" {
		return ""
	}
	if status != "" && nodeCount != "" {
		return fmt.Sprintf("这次 trace 的整体状态为%s，共记录了 %s 个节点。", translateStatus(status), nodeCount)
	}
	if status != "" {
		return fmt.Sprintf("这次 trace 的整体状态为%s。", translateStatus(status))
	}
	return fmt.Sprintf("这次 trace 共记录了 %s 个节点。", nodeCount)
}

func humanizeRetrieveStats(values map[string]string) string {
	chunkCount := values["retrieve.chunkCount"]
	topScore := values["retrieve.topScore"]
	searchMode := values["retrieve.searchMode"]
	if chunkCount == "" && topScore == "" && searchMode == "" {
		return ""
	}
	parts := make([]string, 0, 3)
	if searchMode != "" {
		parts = append(parts, "检索模式为 "+searchMode)
	}
	if chunkCount != "" {
		parts = append(parts, "召回到的 chunk 数量为 "+chunkCount)
	}
	if topScore != "" {
		parts = append(parts, "最高相关分数为 "+topScore)
	}
	return strings.Join(parts, "，") + "。"
}

func humanizeToolWorkflow(values map[string]string) string {
	status := values["toolWorkflow.status"]
	degraded := values["toolWorkflow.degraded"]
	callCount := values["toolWorkflow.callCount"]
	degradeReason := values["toolWorkflow.degradeReason"]
	tools := values["toolWorkflow.tools"]
	if status == "" && degraded == "" && callCount == "" && degradeReason == "" && tools == "" {
		return ""
	}
	parts := make([]string, 0, 4)
	if callCount != "" {
		parts = append(parts, "本次 tool workflow 共调用了 "+callCount+" 次工具")
	}
	if status != "" {
		parts = append(parts, "整体状态为 "+translateStatus(status))
	}
	if degraded == "true" {
		parts = append(parts, "存在降级")
	}
	if degradeReason != "" {
		parts = append(parts, "降级原因是 "+degradeReason)
	}
	if tools != "" {
		parts = append(parts, "涉及工具包括 "+tools)
	}
	return strings.Join(parts, "，") + "。"
}

func humanizeFailedTraceNode(values map[string]string) string {
	nodeID := values["failedNode"]
	errMsg := values["failedNode.error"]
	if nodeID == "" {
		return ""
	}
	if errMsg != "" {
		return fmt.Sprintf("失败节点是 %s，错误信息为 %q。", nodeID, errMsg)
	}
	return fmt.Sprintf("失败节点是 %s。", nodeID)
}

func humanizeGenericEvidence(item string) string {
	item = strings.TrimSpace(item)
	if item == "" {
		return ""
	}
	key, value, ok := strings.Cut(item, "=")
	if !ok {
		return item
	}
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if key == "" || value == "" {
		return item
	}
	return fmt.Sprintf("%s 为 %s。", key, value)
}

func translateStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed":
		return "失败"
	case "running":
		return "运行中"
	case "success":
		return "成功"
	case "pending":
		return "待处理"
	default:
		return strings.TrimSpace(status)
	}
}

func firstNonEmptyValue(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func normalizeStringList(values []string) []string {
	if len(values) == 0 {
		return []string{}
	}
	items := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			items = append(items, trimmed)
		}
	}
	return items
}

func deriveDiagnosisInferences(conclusion string, confidence string) []string {
	conclusion = strings.TrimSpace(conclusion)
	confidence = normalizeDiagnosisConfidence(confidence)
	if conclusion == "" {
		return []string{}
	}
	inferences := []string{conclusion}
	switch confidence {
	case diagnosisConfidenceMedium:
		inferences = append(inferences, "当前结论已经有一定证据支持，但仍建议结合更详细的日志或状态回写顺序继续核实。")
	case diagnosisConfidenceLow:
		inferences = append(inferences, "当前证据仍不完整，或不同状态之间存在冲突，因此这更像是一个待验证的初步判断。")
	}
	return inferences
}

func deriveDiagnosisRiskHints(conclusion string, confidence string) []string {
	conclusion = strings.TrimSpace(strings.ToLower(conclusion))
	confidence = normalizeDiagnosisConfidence(confidence)
	risks := make([]string, 0, 3)
	if confidence != diagnosisConfidenceHigh {
		risks = append(risks, fmt.Sprintf("当前置信度为 %s，后续如果补充到更多证据，结论仍可能调整。", confidence))
	}
	if strings.Contains(conclusion, "inconsistent") {
		risks = append(risks, "不同状态之间可能存在回写顺序问题，或受到重试、补偿流程影响，因此当前快照可能互相冲突。")
	}
	if strings.Contains(conclusion, "degraded") {
		risks = append(risks, "部分诊断依赖降级后的 tool 或 trace 证据，因此最终解释可能还不完整。")
	}
	if strings.Contains(conclusion, "running") {
		risks = append(risks, "目标仍在运行中，因此当前诊断更像是阶段性快照，而不是最终结论。")
	}
	return risks
}
