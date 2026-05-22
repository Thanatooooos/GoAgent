package runtime

import (
	"strings"

	. "local/rag-project/internal/app/rag/tool/core"
	"local/rag-project/internal/framework/log"
)

// baseRouteRule defines a keyword-driven routing rule. Rules in a family are
// checked in order; the first match wins.
type baseRouteRule struct {
	requireAll [][]string // at least one keyword from each inner slice must match
	exclude    []string   // none of these must match
	buildCall  func(id string) Call
}

var (
	diagnosisKeywords = []string{
		"diagnose", "failure", "why", "failed", "排查", "诊断", "失败", "原因",
		"running", "processing", "progress", "slow", "stuck", "node", "status",
		"运行", "处理中", "进度", "慢", "卡", "节点", "状态", "还在", "完成",
	}
	solutionKeywords = []string{
		"解决", "怎么办", "修复", "方案", "办法", "如何处理", "怎么修复",
		"solution", "fix", "how to fix", "resolve", "troubleshoot",
	}
	chunkLogKeywords = []string{
		"chunk log", "chunklog", "chunk", "ingestion", "pipeline",
		"diagnose", "failure", "排查", "诊断", "失败",
	}
	docKeywords            = []string{"document", "doc", "文档"}
	taskKeywords           = []string{"ingestion", "task", "任务", "导入任务"}
	traceKeywords          = []string{"trace", "chain", "retrieval", "链路", "检索", "召回"}
	traceDiagnosisKeywords = []string{
		"diagnose", "failure", "why", "bad", "poor", "failed",
		"排查", "诊断", "失败", "原因", "效果差", "召回差",
	}
)

var documentBaseRules = []baseRouteRule{
	{
		requireAll: [][]string{docKeywords, diagnosisKeywords, solutionKeywords},
		buildCall: func(id string) Call {
			return Call{Name: "document_diagnose_with_search", Arguments: map[string]any{"documentId": id}}
		},
	},
	{
		requireAll: [][]string{docKeywords, diagnosisKeywords},
		exclude:    solutionKeywords,
		buildCall: func(id string) Call {
			return Call{Name: "document_root_cause_diagnosis", Arguments: map[string]any{"documentId": id}}
		},
	},
	{
		requireAll: [][]string{docKeywords, chunkLogKeywords},
		exclude:    diagnosisKeywords,
		buildCall: func(id string) Call {
			return Call{Name: "document_chunk_log_query", Arguments: map[string]any{"documentId": id}}
		},
	},
	{
		requireAll: [][]string{docKeywords},
		buildCall:  func(id string) Call { return Call{Name: "document_query", Arguments: map[string]any{"documentId": id}} },
	},
}

var taskBaseRules = []baseRouteRule{
	{
		requireAll: [][]string{taskKeywords, diagnosisKeywords},
		buildCall: func(id string) Call {
			return Call{Name: "task_ingestion_diagnose", Arguments: map[string]any{"taskId": id}}
		},
	},
	{
		requireAll: [][]string{taskKeywords},
		buildCall: func(id string) Call {
			return Call{Name: "ingestion_task_query", Arguments: map[string]any{"taskId": id, "includeNodes": true}}
		},
	},
}

func PlanWithBaseRules(input WorkflowInput, maxCalls int) []Call {
	question := strings.TrimSpace(input.Question)
	if question == "" {
		return nil
	}
	lowered := strings.ToLower(question)
	calls := make([]Call, 0, maxCalls)
	seen := map[string]struct{}{}

	appendCall := func(call Call) {
		if len(calls) >= maxCalls {
			return
		}
		key := callKey(call)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		calls = append(calls, call)
	}

	appendDocumentCalls(lowered, question, input, appendCall)
	appendTaskCalls(lowered, question, appendCall)
	appendTraceCalls(lowered, question, input.TraceID, appendCall)

	if len(calls) == 0 {
		appendOpenEndedCalls(lowered, input, appendCall)
	}

	if len(calls) == 0 && KnowledgeBaseInsufficient(input.RetrieveResult) {
		log.Infof("[agent] kb insufficient (chunks=%d), triggering external_evidence_workflow for %q", len(input.RetrieveResult.Chunks), TruncateForLog(question))
		appendCall(Call{Name: "external_evidence_workflow", Arguments: map[string]any{"question": question}})
	}

	return calls
}

func applyFirstMatchingRule(lowered string, id string, rules []baseRouteRule) *Call {
	for _, rule := range rules {
		if !matchKeywordGroups(lowered, rule.requireAll) {
			continue
		}
		if len(rule.exclude) > 0 && containsAny(lowered, rule.exclude...) {
			continue
		}
		call := rule.buildCall(id)
		return &call
	}
	return nil
}

func matchKeywordGroups(text string, groups [][]string) bool {
	for _, group := range groups {
		if !containsAny(text, group...) {
			return false
		}
	}
	return true
}

func appendDocumentCalls(lowered, question string, input WorkflowInput, appendCall func(Call)) {
	id := firstMatchedID(documentIDPattern, question)
	if id == "" || !containsAny(lowered, docKeywords...) {
		return
	}
	call := applyFirstMatchingRule(lowered, id, documentBaseRules)
	if call != nil {
		appendCall(*call)
	}
}

func appendTaskCalls(lowered, question string, appendCall func(Call)) {
	id := firstMatchedID(taskIDPattern, question)
	if id == "" || !containsAny(lowered, taskKeywords...) {
		return
	}
	call := applyFirstMatchingRule(lowered, id, taskBaseRules)
	if call != nil {
		appendCall(*call)
	}
}

func appendTraceCalls(lowered, question, traceID string, appendCall func(Call)) {
	id := firstMatchedID(traceIDPattern, question)
	if id == "" && containsAny(lowered, "本次", "当前", "this", "current") && containsAny(lowered, traceKeywords...) {
		id = strings.TrimSpace(traceID)
	}
	if id == "" || !containsAny(lowered, traceKeywords...) {
		return
	}
	if containsAny(lowered, traceDiagnosisKeywords...) {
		appendCall(Call{Name: "trace_retrieval_diagnose", Arguments: map[string]any{"traceId": id}})
	}
	appendCall(Call{Name: "trace_node_query", Arguments: map[string]any{"traceId": id}})
}

func appendOpenEndedCalls(lowered string, input WorkflowInput, appendCall func(Call)) {
	isOpenEnded := containsAny(lowered,
		"哪些", "最近", "所有", "列表", "哪个", "哪个文档", "哪些文档",
		"which", "list", "recent", "all", "any",
		"失败", "运行中", "处理中",
	)
	if !isOpenEnded {
		return
	}
	defaultKB := ""
	if len(input.KnowledgeBaseIDs) > 0 {
		defaultKB = strings.TrimSpace(input.KnowledgeBaseIDs[0])
	}
	if containsAny(lowered, docKeywords...) {
		callArgs := map[string]any{}
		if defaultKB != "" {
			callArgs["knowledgeBaseId"] = defaultKB
		}
		if containsAny(lowered, "失败", "failed") {
			callArgs["status"] = "failed"
		} else if containsAny(lowered, "运行", "处理中", "running") {
			callArgs["status"] = "running"
		}
		appendCall(Call{Name: "document_list", Arguments: callArgs})
	}
	if containsAny(lowered, taskKeywords...) {
		callArgs := map[string]any{}
		if containsAny(lowered, "失败", "failed") {
			callArgs["status"] = "failed"
		} else if containsAny(lowered, "运行", "处理中", "running") {
			callArgs["status"] = "running"
		}
		appendCall(Call{Name: "task_list", Arguments: callArgs})
	}
}
