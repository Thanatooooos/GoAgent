package rewrite

import "testing"

func TestLooksLikeMultiIntentQuery(t *testing.T) {
	multi := []string{
		"线上 CPU 和内存同时告警，一般按什么顺序排查",
		"Redis 要落盘备份，AOF rewrite 和 RDB bgsave 一般怎么选",
		"slice 和 map 的扩容规则我老记混，能帮我捋一下吗",
		"SSE 长连接特别多，Go 是怎么靠调度模型和 IO 多路复用撑住的",
		"HTTP 明文传输有啥问题，上 HTTPS 以后怎么保证不被窃听和篡改",
	}
	for _, q := range multi {
		if !looksLikeMultiIntentQuery(q) {
			t.Fatalf("expected multi-intent for %q", q)
		}
	}

	single := []string{
		"defer 的执行顺序是什么",
		"select 多个 case 同时就绪时怎么选",
		"sync.Pool 典型用法是什么",
		"errgroup 相比 WaitGroup 多了什么能力",
		"TCP 四次挥手为什么需要 TIME_WAIT",
		"Raft 集群加节点，为什么推荐一次只改一个成员",
	}
	for _, q := range single {
		if looksLikeMultiIntentQuery(q) {
			t.Fatalf("expected single-intent for %q", q)
		}
	}
}

func TestCollapseSingleIntentSubQuestions(t *testing.T) {
	original := "defer 的执行顺序是什么"
	result := Result{
		RewrittenQuestion: "defer语句的执行顺序是什么",
		SubQuestions: []string{
			"defer语句的执行顺序是什么",
			"defer执行顺序规则",
		},
		NeedRetrieval: true,
	}
	collapsed := collapseSingleIntentSubQuestions(original, result)
	if len(collapsed.SubQuestions) != 1 {
		t.Fatalf("expected 1 sub question, got %v", collapsed.SubQuestions)
	}
	if collapsed.SubQuestions[0] != result.RewrittenQuestion {
		t.Fatalf("unexpected sub question: %q", collapsed.SubQuestions[0])
	}
}

func TestCollapseSingleIntentSubQuestionsKeepsMultiIntentSplit(t *testing.T) {
	original := "slice 和 map 的扩容规则我老记混，能帮我捋一下吗"
	result := Result{
		RewrittenQuestion: "slice和map的扩容规则是什么",
		SubQuestions: []string{
			"slice和map的扩容规则是什么",
			"slice扩容规则",
			"map扩容规则",
		},
		NeedRetrieval: true,
	}
	kept := collapseSingleIntentSubQuestions(original, result)
	if len(kept.SubQuestions) != 3 {
		t.Fatalf("expected multi-intent split preserved, got %v", kept.SubQuestions)
	}
}

func TestRewriteWithSplitCollapsesSingleIntentOverSplit(t *testing.T) {
	llm := &stubLLMService{
		response: `{"rewritten":"defer语句的执行顺序是什么","sub_questions":["defer执行顺序规则"],"need_retrieval":true}`,
	}
	service := NewLLMService(llm)

	result := service.RewriteWithSplit("defer 的执行顺序是什么")
	if len(result.SubQuestions) != 1 {
		t.Fatalf("expected collapsed sub questions, got %v", result.SubQuestions)
	}
	if result.SubQuestions[0] != "defer语句的执行顺序是什么" {
		t.Fatalf("unexpected sub question: %q", result.SubQuestions[0])
	}
}
