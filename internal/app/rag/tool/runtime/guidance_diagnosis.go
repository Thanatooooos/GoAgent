package runtime

import (
	"fmt"
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	systemmod "local/rag-project/internal/app/rag/tool/modules/system"
)

func buildDiagnosisGuidance(result Result, allResults []Result) string {
	view, _ := systemmod.ViewDiagnosisResult(result)
	conclusion := strings.TrimSpace(view.Conclusion)
	confidence := strings.TrimSpace(view.Confidence)
	facts := append([]string(nil), view.Facts...)
	inferences := append([]string(nil), view.Inferences...)
	riskHints := append([]string(nil), view.RiskHints...)
	nextActions := append([]string(nil), view.NextActions...)

	conclusion, confidence, facts, inferences, riskHints = enrichDiagnosisWithDeeperEvidence(
		result, allResults, conclusion, confidence, facts, inferences, riskHints,
	)

	var builder strings.Builder
	builder.WriteString("这是一类诊断回答。请优先使用中文，并按「结论 / 证据 / 建议」的顺序组织回答。")
	builder.WriteString("\n结论部分只陈述当前最可能的问题，并明确写出置信度。")
	if conclusion != "" {
		builder.WriteString("\n当前建议结论：")
		builder.WriteString(conclusion)
		builder.WriteString("。")
	}
	if confidence != "" {
		builder.WriteString("\n当前置信度：")
		builder.WriteString(confidence)
		builder.WriteString("。")
	}
	if len(facts) > 0 {
		builder.WriteString("\n证据部分优先引用 3-5 条最关键事实，不要把推断写成事实：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(facts, "\n- "))
	}
	if len(inferences) > 0 {
		builder.WriteString("\n如果需要补充推断，请明确标注「推断」或「可能原因」，不要伪装成已确认事实：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(inferences, "\n- "))
	}
	if len(riskHints) > 0 {
		builder.WriteString("\n如果证据仍有边界或风险，请显式提醒用户：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(riskHints, "\n- "))
	}
	if len(nextActions) > 0 {
		builder.WriteString("\n建议部分给出可执行的下一步检查项：")
		builder.WriteString("\n- ")
		builder.WriteString(strings.Join(nextActions, "\n- "))
	}
	builder.WriteString("\n不要只复述工具调用过程。若后续 task/node 查询的结果与前面的文档级诊断结论不一致（例如诊断说 failed 但 task/node 实际 running），必须以 task/node 的实际状态为准，并在回答中显式说明状态不一致。没有更深的节点级证据时，不要臆造错误原因。")
	return builder.String()
}

func enrichDiagnosisWithDeeperEvidence(
	base Result, allResults []Result,
	conclusion string, confidence string, facts []string, inferences []string, riskHints []string,
) (string, string, []string, []string, []string) {
	if strings.TrimSpace(base.Name) == "trace_retrieval_diagnose" {
		return conclusion, confidence, facts, inferences, riskHints
	}

	taskID := strings.TrimSpace(base.GetString("taskId"))
	if taskID == "" {
		taskID = strings.TrimSpace(base.GetString("latestTaskId"))
	}
	if taskID == "" {
		return conclusion, confidence, facts, inferences, riskHints
	}

	taskResult, hasTask := findLatestTaskResult(allResults, taskID)
	nodeResult, hasNode := findLatestTaskNodeDetail(allResults, taskID)

	conclusionLower := strings.ToLower(conclusion)
	diagnoseSaysFailed := strings.Contains(conclusionLower, "failed") || strings.Contains(conclusionLower, "失败")

	taskStatus := strings.ToLower(strings.TrimSpace(taskResult.GetString("status")))
	nodeStatus := strings.ToLower(strings.TrimSpace(nodeResult.GetString("status")))

	if diagnoseSaysFailed && (taskStatus == "running" || nodeStatus == "running") {
		conclusion, confidence, facts, inferences, riskHints = resolveStatusConflict(
			taskResult, nodeResult, hasTask, hasNode, taskID, conclusion, confidence, facts, inferences, riskHints,
		)
		return conclusion, confidence, facts, inferences, riskHints
	}

	if !hasNode {
		return conclusion, confidence, facts, inferences, riskHints
	}

	nodeID := strings.TrimSpace(nodeResult.GetString("nodeId"))
	nodeError := strings.TrimSpace(nodeResult.GetString("errorMessage"))
	nodeOrder := nodeResult.GetInt("nodeOrder")
	durationMs := nodeResult.GetInt("durationMs")
	if nodeID == "" {
		return conclusion, confidence, facts, inferences, riskHints
	}

	if nodeStatus == "failed" {
		conclusion = fmt.Sprintf("文档导入失败，失败发生在 %s 节点", nodeID)
		if nodeError != "" {
			conclusion = fmt.Sprintf("%s，节点报错为 %q", conclusion, nodeError)
		}
		confidence = "high"
	}

	extraFacts := make([]string, 0, 3)
	if nodeOrder > 0 {
		extraFacts = append(extraFacts, fmt.Sprintf("对应的 ingestion 节点是第 %d 个节点 %s，状态为%s。", nodeOrder, nodeID, renderStatusLabel(nodeStatus)))
	} else {
		extraFacts = append(extraFacts, fmt.Sprintf("对应的 ingestion 节点是 %s，状态为%s。", nodeID, renderStatusLabel(nodeStatus)))
	}
	if nodeError != "" {
		extraFacts = append(extraFacts, fmt.Sprintf("该节点的具体错误是 %q。", nodeError))
	}
	if durationMs > 0 {
		extraFacts = append(extraFacts, fmt.Sprintf("该节点持续时间约为 %dms。", durationMs))
	}
	facts = prependUniqueFacts(facts, extraFacts...)

	if nodeStatus == "failed" {
		inferences = []string{
			fmt.Sprintf("推断：前序阶段大概率已经完成，失败更集中发生在 %s 节点对应的索引或下游持久化环节。", nodeID),
		}
	}

	return conclusion, confidence, facts, inferences, riskHints
}

func resolveStatusConflict(
	taskResult Result, nodeResult Result, hasTask bool, hasNode bool, taskID string,
	conclusion string, confidence string, facts []string, inferences []string, riskHints []string,
) (string, string, []string, []string, []string) {
	diagnoseConclusion := conclusion

	if hasNode {
		nodeID := strings.TrimSpace(nodeResult.GetString("nodeId"))
		conclusion = fmt.Sprintf("文档仍在处理中，当前运行到 %s 节点", nodeID)
	} else {
		conclusion = fmt.Sprintf("文档仍在处理中，关联任务 %s 仍在运行", taskID)
	}
	confidence = "high"

	extraFacts := make([]string, 0, 3)
	if hasNode {
		nodeID := strings.TrimSpace(nodeResult.GetString("nodeId"))
		extraFacts = append(extraFacts, fmt.Sprintf("更接近实时执行链路的节点查询显示 %s 仍在运行中。", nodeID))
	} else {
		extraFacts = append(extraFacts, fmt.Sprintf("更接近实时执行链路的任务查询显示 %s 仍在运行中。", taskID))
	}
	extraFacts = append(extraFacts, fmt.Sprintf("文档级诊断结论（%s）与任务/节点实际状态不一致，可能因状态异步更新滞后。", strings.TrimSpace(diagnoseConclusion)))
	facts = prependUniqueFacts(facts, extraFacts...)

	inferences = []string{
		"推断：文档级状态可能因异步更新延迟而滞后，应以更接近执行链路的任务/节点状态为准。",
	}

	riskHints = prependUniqueFacts(riskHints,
		"状态不一致风险：文档级状态与任务/节点实际状态存在冲突，已优先采用任务/节点状态。建议检查文档状态同步机制。",
	)

	return conclusion, confidence, facts, inferences, riskHints
}

func findLatestTaskNodeDetail(results []Result, taskID string) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		result := results[idx]
		if strings.TrimSpace(result.Name) != "ingestion_task_node_query" {
			continue
		}
		if strings.TrimSpace(result.GetString("taskId")) != taskID {
			continue
		}
		if strings.TrimSpace(result.GetString("nodeId")) == "" {
			continue
		}
		return result, true
	}
	return Result{}, false
}

func findLatestTaskResult(results []Result, taskID string) (Result, bool) {
	for idx := len(results) - 1; idx >= 0; idx-- {
		result := results[idx]
		if strings.TrimSpace(result.Name) != "ingestion_task_query" {
			continue
		}
		if strings.TrimSpace(result.GetString("taskId")) != taskID {
			continue
		}
		return result, true
	}
	return Result{}, false
}

func prependUniqueFacts(existing []string, incoming ...string) []string {
	seen := map[string]struct{}{}
	items := make([]string, 0, len(existing)+len(incoming))
	for _, fact := range incoming {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		items = append(items, fact)
	}
	for _, fact := range existing {
		fact = strings.TrimSpace(fact)
		if fact == "" {
			continue
		}
		if _, ok := seen[fact]; ok {
			continue
		}
		seen[fact] = struct{}{}
		items = append(items, fact)
	}
	return items
}

func renderStatusLabel(status string) string {
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
