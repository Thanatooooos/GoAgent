package builtin

import (
	"context"
	"fmt"
	"strings"

	ragdomain "local/rag-project/internal/app/rag/domain"
	ragport "local/rag-project/internal/app/rag/port"
	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/app/rag/traceinsight"
)

type TraceRetrievalDiagnoseTool struct {
	runRepo  ragport.RagTraceRunRepository
	nodeRepo ragport.RagTraceNodeRepository
}

func NewTraceRetrievalDiagnoseTool(runRepo ragport.RagTraceRunRepository, nodeRepo ragport.RagTraceNodeRepository) *TraceRetrievalDiagnoseTool {
	return &TraceRetrievalDiagnoseTool{
		runRepo:  runRepo,
		nodeRepo: nodeRepo,
	}
}

func (t *TraceRetrievalDiagnoseTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "trace_retrieval_diagnose",
		Description: "Diagnose a RAG trace and explain whether the issue is rewrite, retrieval, prompt, or execution quality.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "traceId",
				Type:        ragtool.ParamTypeString,
				Description: "Trace id.",
				Required:    true,
			},
		},
	}
}

func (t *TraceRetrievalDiagnoseTool) Invoke(ctx context.Context, call ragtool.Call) (ragtool.Result, error) {
	if t == nil || t.runRepo == nil || t.nodeRepo == nil {
		return ragtool.Result{Name: "trace_retrieval_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: "trace repositories are required"}, fmt.Errorf("trace repositories are required")
	}
	traceID := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "traceId"))
	if traceID == "" {
		return ragtool.Result{Name: "trace_retrieval_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: "traceId is required"}, fmt.Errorf("traceId is required")
	}

	run, err := t.runRepo.GetByTraceID(ctx, traceID)
	if err != nil {
		return ragtool.Result{Name: "trace_retrieval_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}
	nodes, err := t.nodeRepo.ListByTraceID(ctx, traceID)
	if err != nil {
		return ragtool.Result{Name: "trace_retrieval_diagnose", Status: ragtool.CallStatusFailed, ErrorMessage: err.Error()}, err
	}

	conclusion, confidence, evidence, suggestions, focusNode, focusReason := diagnoseTraceRetrieval(run, nodes)
	confidence = normalizeDiagnosisConfidence(confidence)
	summary := fmt.Sprintf("trace=%s confidence=%s conclusion=%s", traceID, confidence, conclusion)
	if focusNode != "" {
		summary = fmt.Sprintf("%s node=%s", summary, focusNode)
	}
	if focusReason != "" {
		summary = fmt.Sprintf("%s reason=%s", summary, focusReason)
	}

	return ragtool.Result{
		Name:    "trace_retrieval_diagnose",
		Status:  ragtool.CallStatusSuccess,
		Summary: summary,
		Data: buildDiagnosisPayload("trace_retrieval", conclusion, confidence, evidence, suggestions, map[string]any{
			"traceId":        run.TraceID,
			"conversationId": run.ConversationID,
			"taskId":         run.TaskID,
			"traceStatus":    run.Status,
			"focusNode":      focusNode,
			"focusReason":    focusReason,
			"nodeCount":      len(nodes),
		}),
	}, nil
}

func diagnoseTraceRetrieval(
	run ragdomain.RagTraceRun,
	nodes []ragdomain.RagTraceNode,
) (conclusion string, confidence string, evidence []string, suggestions []string, focusNode string, focusReason string) {
	evidence = append(evidence, fmt.Sprintf("trace.status=%s", strings.TrimSpace(run.Status)))
	evidence = append(evidence, fmt.Sprintf("trace.nodeCount=%d", len(nodes)))
	if strings.TrimSpace(run.ConversationID) != "" {
		evidence = append(evidence, fmt.Sprintf("trace.conversationId=%s", strings.TrimSpace(run.ConversationID)))
	}
	if strings.TrimSpace(run.ErrorMessage) != "" {
		evidence = append(evidence, fmt.Sprintf("trace.error=%s", strings.TrimSpace(run.ErrorMessage)))
	}

	for _, node := range nodes {
		if strings.TrimSpace(node.Status) == "failed" {
			evidence = append(evidence, fmt.Sprintf("failedNode=%s", strings.TrimSpace(node.NodeID)))
			if strings.TrimSpace(node.ErrorMessage) != "" {
				evidence = append(evidence, fmt.Sprintf("failedNode.error=%s", strings.TrimSpace(node.ErrorMessage)))
			}
			return fmt.Sprintf("trace execution failed at node %s", strings.TrimSpace(node.NodeID)), "high", evidence,
				traceSuggestionsForFailedNode(node), strings.TrimSpace(node.NodeID), ragcore.FirstNonEmpty(strings.TrimSpace(node.ErrorMessage), "node execution failed")
		}
	}

	rewriteNode := findTraceNode(nodes, "rewrite")
	retrieveNode := findTraceNode(nodes, "retrieve")
	promptNode := findTraceNode(nodes, "prompt")
	toolWorkflowNode := findToolWorkflowNode(nodes)
	longTermMemoryNode := findTraceNode(nodes, "long_term_memory")
	sessionRecallNode := findTraceNode(nodes, "session_recall")
	longTermMemory := traceinsight.ParseLongTermMemoryNode(longTermMemoryNode)
	sessionRecall := traceinsight.ParseSessionRecallNode(sessionRecallNode)
	evidence = appendTraceMemoryEvidence(evidence, longTermMemory, sessionRecall)

	if rewriteNode == nil && retrieveNode == nil && promptNode == nil {
		return "trace does not contain key retrieval-phase nodes yet", "low", evidence,
			[]string{"check whether this trace comes from the RAG chat main path", "inspect trace node persistence and stage recording"}, "", ""
	}

	if retrieveNode != nil {
		chunkCount := readTraceExtraInt(retrieveNode.ExtraData, "chunkCount")
		searchMode := readTraceExtraString(retrieveNode.ExtraData, "searchMode")
		if searchMode != "" {
			evidence = append(evidence, fmt.Sprintf("retrieve.searchMode=%s", searchMode))
		}
		topScore := readTraceExtraFloat(retrieveNode.ExtraData, "topScore")
		if topScore >= 0 {
			evidence = append(evidence, fmt.Sprintf("retrieve.topScore=%.4f", topScore))
			if topScore > 0 && topScore < 0.35 {
				return "trace retrieval returned chunks, but the top retrieval score is weak, so grounding quality is likely poor", diagnosisConfidenceMedium, evidence,
					[]string{"inspect the highest-ranked chunks for relevance drift", "compare semantic, keyword, and hybrid retrieval outputs for the same query"}, "retrieve", fmt.Sprintf("topScore=%.4f", topScore)
			}
		}
		if chunkCount >= 0 {
			evidence = append(evidence, fmt.Sprintf("retrieve.chunkCount=%d", chunkCount))
			if chunkCount == 0 {
				if sessionRecall != nil && sessionRecall.CandidateCount > 0 && sessionRecall.ExcerptCount == 0 && sessionRecall.TruncatedBy != "" {
					return "trace retrieval returned no chunks, and session recall also dropped all candidate excerpts because of recall selection limits", diagnosisConfidenceHigh, evidence,
						[]string{"inspect session recall token budget and per-message limits", "check whether the follow-up question should be narrowed so recall can keep a usable excerpt"}, "session_recall", "truncatedBy=" + sessionRecall.TruncatedBy
				}
				if traceMemoryRecallSelectedCount(longTermMemory, sessionRecall) > 0 {
					return "trace retrieval returned no chunks, but memory recall still contributed prompt context, so answer quality now depends on recalled memory rather than knowledge-base grounding", diagnosisConfidenceMedium, evidence,
						[]string{"inspect whether the recalled long-term memory or session excerpts were actually relevant to the question", "if the answer should have been grounded in the knowledge base, inspect rewrite quality and knowledge coverage"}, "retrieve", "chunkCount=0 with memory recall context"
				}
				if longTermMemory != nil || sessionRecall != nil {
					return "trace retrieval returned no chunks and no memory recall context was selected either, so the answer likely lacked grounding evidence", diagnosisConfidenceHigh, evidence,
						[]string{"check query rewrite quality and knowledge base coverage first", "inspect why long-term memory and session recall both produced no usable context"}, "retrieve", "chunkCount=0 and memory/session recall empty"
				}
				return "trace retrieval returned no chunks", "high", evidence,
					[]string{"check query rewrite quality and knowledge base coverage", "inspect retrieval filters, embeddings, and search mode selection"}, "retrieve", "chunkCount=0"
			}
			if chunkCount <= 2 {
				return "trace retrieval returned only a few chunks, so answer quality may be weak", "medium", evidence,
					[]string{"inspect retrieval recall quality and chunk coverage", "compare semantic, keyword, and hybrid search modes for this query"}, "retrieve", fmt.Sprintf("chunkCount=%d", chunkCount)
			}
		}
	}

	if sessionRecall != nil && sessionRecall.CandidateCount > 0 && sessionRecall.ExcerptCount == 0 && sessionRecall.TruncatedBy != "" {
		return "session recall found candidates, but no excerpt survived the current prompt-budget or per-message selection limits", diagnosisConfidenceMedium, evidence,
			[]string{"inspect session recall prompt budget and excerpt limits", "check whether the prior long message should be summarized or chunked differently for recall"}, "session_recall", "truncatedBy=" + sessionRecall.TruncatedBy
	}

	if hasDegradedMemoryTrace(longTermMemory, sessionRecall) {
		return "trace completed through a degraded memory recall path, so repeated runs may show unstable cache or fingerprint behavior even if execution succeeded", diagnosisConfidenceMedium, evidence,
			[]string{"inspect Redis recall cache and scope-version lookup health", "inspect session recall fingerprint generation if the degraded path came from fingerprint_unavailable"}, degradedMemoryTraceNodeID(longTermMemory, sessionRecall), "memory recall degraded"
	}

	if rewriteNode != nil {
		evidence = append(evidence, fmt.Sprintf("rewrite.status=%s", strings.TrimSpace(rewriteNode.Status)))
	}
	if promptNode != nil {
		evidence = append(evidence, fmt.Sprintf("prompt.status=%s", strings.TrimSpace(promptNode.Status)))
	}
	if toolWorkflowNode != nil {
		evidence = append(evidence, fmt.Sprintf("toolWorkflow.status=%s", strings.TrimSpace(toolWorkflowNode.Status)))
		toolCallCount := readTraceExtraInt(toolWorkflowNode.ExtraData, "toolCallCount")
		if toolCallCount >= 0 {
			evidence = append(evidence, fmt.Sprintf("toolWorkflow.callCount=%d", toolCallCount))
		}
		if readTraceExtraBool(toolWorkflowNode.ExtraData, "degraded") {
			evidence = append(evidence, "toolWorkflow.degraded=true")
		}
		degradeReason := readTraceExtraString(toolWorkflowNode.ExtraData, "degradeReason")
		if degradeReason != "" {
			evidence = append(evidence, fmt.Sprintf("toolWorkflow.degradeReason=%s", degradeReason))
		}
		toolNames := readTraceExtraStringSlice(toolWorkflowNode.ExtraData, "toolNames")
		if len(toolNames) > 0 {
			evidence = append(evidence, fmt.Sprintf("toolWorkflow.tools=%s", strings.Join(toolNames, ",")))
		}
		if readTraceExtraBool(toolWorkflowNode.ExtraData, "degraded") {
			return "trace completed tool workflow with degraded tool calls; diagnosis evidence may be incomplete", "medium", evidence,
				[]string{"inspect tool call trace nodes and the degrade reason before trusting the final answer", "re-run the query after fixing the degraded tool or backing service"}, "tool_workflow", ragcore.FirstNonEmpty(readTraceExtraString(toolWorkflowNode.ExtraData, "degradeReason"), "tool workflow degraded")
		}
	}

	if strings.TrimSpace(run.Status) == "success" {
		return "trace execution completed successfully; if the answer was poor, the issue is more likely retrieval relevance or answer synthesis quality", "medium", evidence,
			[]string{"inspect retrieved chunk relevance and prompt grounding", "compare the final answer against retrieved evidence to see whether synthesis drift occurred"}, "retrieve", "execution succeeded"
	}

	if strings.TrimSpace(run.Status) == "running" {
		return "trace is still running", "high", evidence,
			[]string{"wait for trace completion and re-run diagnosis", "inspect the latest active node progress"}, ragcore.FirstNonEmpty(activeTraceNodeID(nodes), "trace"), "trace is running"
	}

	return "trace state is partially inconsistent and needs manual review", "low", evidence,
		[]string{"compare trace run status with individual node statuses", "inspect trace node extraData and persistence ordering"}, "", ""
}

func findTraceNode(nodes []ragdomain.RagTraceNode, nodeID string) *ragdomain.RagTraceNode {
	for i := range nodes {
		if strings.TrimSpace(nodes[i].NodeID) == strings.TrimSpace(nodeID) {
			return &nodes[i]
		}
	}
	return nil
}

func activeTraceNodeID(nodes []ragdomain.RagTraceNode) string {
	for _, node := range nodes {
		if strings.TrimSpace(node.Status) == "running" {
			return strings.TrimSpace(node.NodeID)
		}
	}
	return ""
}

func traceSuggestionsForFailedNode(node ragdomain.RagTraceNode) []string {
	switch strings.TrimSpace(node.NodeID) {
	case "rewrite":
		return []string{"inspect rewrite prompt and rewritten sub-questions", "check whether the model produced malformed or misleading rewrite results"}
	case "retrieve":
		return []string{"inspect retrieval backend availability and search mode selection", "check embeddings, filters, and vector/keyword retrieval paths"}
	case "prompt":
		return []string{"inspect prompt assembly and injected knowledge/tool context", "check whether the prompt stage received the expected retrieval context"}
	case "tool_workflow":
		return []string{"inspect tool planning and tool execution summaries", "check whether diagnosis/query tools degraded and polluted the final answer context"}
	default:
		return []string{"inspect the failed trace node details", "check trace node extraData and related service logs"}
	}
}
