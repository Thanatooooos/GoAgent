package rewrite

import (
	"encoding/json"
	"fmt"
	"strings"

	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	defaultRewriteModel = "default"
	rewriteSystemPrompt = `你是一个查询改写助手。你的任务是将用户问题改写为更适合知识库检索的查询，并判断这次对话是否真的需要检索知识库。

你必须遵守以下规则：
1. 将指代词替换为具体实体，让问题可以独立理解。
2. 去掉口语赘述，保留核心语义。
3. 如果问题复杂，可以拆成 1-2 个检索子问题；不要过度拆分。
4. 多意图拆分时必须保留原文中的英文术语和关键标识（如 GMP、netpoller、AOF、RDB、SIGURG）。
5. 保留原文中的技术术语、命令名、英文缩写和专有名词（如 SIGURG、unlink、bgsave、GTID、pprof），不要替换或省略。
6. 输出 need_retrieval：
   - true：用户在询问知识、文档、配置、错误、规则、事实，需要知识库支持
   - false：用户只是寒暄、感谢、结束对话、自我介绍类闲聊，不需要检索知识库

示例：
用户问："RAG是什么"
输出：{"rewritten":"什么是RAG检索增强生成","sub_questions":["RAG定义","RAG原理"],"need_retrieval":true}

用户问："它有哪些应用场景"
输出：{"rewritten":"向量数据库有哪些应用场景","sub_questions":["向量数据库应用场景"],"need_retrieval":true}

用户问："你好"
输出：{"rewritten":"你好","sub_questions":["你好"],"need_retrieval":false}

你必须严格输出 JSON，不要输出其他内容：
{"rewritten":"改写后的主问题","sub_questions":["子问题1","子问题2"],"need_retrieval":true|false}`
	historyFollowUpPromptSuffix = `

## 指代消解补充
- 当前是带对话历史的 follow-up：只做指代消解，把最新问题改写成可独立检索的单句。
- sub_questions 必须输出 []，不要拆成多个检索子问题。
- 不要把同一 follow-up 改写成多个角度或步骤。
- 若追问的是同一主题的子方面（如持久化、扩容规则），改写中必须保留可检索的主题词（如 Redis、AOF/RDB、slice、channel）。`
)

type LLMService struct {
	chatService aichat.LLMService
}

func NewLLMService(chatService aichat.LLMService) *LLMService {
	return &LLMService{chatService: chatService}
}

func (s *LLMService) Rewrite(question string) string {
	result := s.RewriteWithSplit(question)
	return result.RewrittenQuestion
}

func (s *LLMService) RewriteWithSplit(question string) Result {
	if s == nil || s.chatService == nil {
		return fallbackResult(question)
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return Result{}
	}

	parsed, err := s.callRewriteLLM(rewriteSystemPrompt, question)
	if err != nil {
		return fallbackResult(question)
	}
	guarded, _ := GuardRewriteResult(question, parsed)
	return collapseSingleIntentSubQuestions(question, guarded)
}

func (s *LLMService) RewriteWithHistory(question string, history []convention.ChatMessage) Result {
	if s == nil || s.chatService == nil {
		return fallbackResult(question)
	}
	question = strings.TrimSpace(question)
	if question == "" {
		return Result{}
	}

	historyPrompt := buildRewriteHistoryPrompt(rewriteSystemPrompt, history, question)
	parsed, err := s.callRewriteLLM(historyPrompt, question)
	if err != nil {
		return fallbackResult(question)
	}
	guarded, _ := GuardRewriteResult(question, parsed)
	return finalizeHistoryFollowUpResult(question, guarded)
}

func finalizeHistoryFollowUpResult(question string, result Result) Result {
	result = collapseHistoryFollowUpSubQuestions(result)
	return enrichPersistenceFollowUpRewrite(question, result)
}

func enrichPersistenceFollowUpRewrite(question string, result Result) Result {
	question = strings.TrimSpace(question)
	rewritten := strings.TrimSpace(result.RewrittenQuestion)
	if question == "" || rewritten == "" {
		return result
	}
	if !strings.Contains(question, "持久化") {
		return result
	}
	lower := strings.ToLower(rewritten)
	if strings.Contains(rewritten, "AOF") || strings.Contains(rewritten, "RDB") ||
		strings.Contains(lower, "aof") || strings.Contains(lower, "rdb") {
		return result
	}
	if !strings.Contains(rewritten, "Redis") && !strings.Contains(lower, "redis") {
		return result
	}
	rewritten = strings.TrimSpace(rewritten + " AOF RDB")
	result.RewrittenQuestion = rewritten
	if len(result.SubQuestions) > 0 {
		result.SubQuestions = []string{rewritten}
	}
	return result
}

func collapseHistoryFollowUpSubQuestions(result Result) Result {
	rewritten := strings.TrimSpace(result.RewrittenQuestion)
	if rewritten == "" {
		return result
	}
	if len(result.SubQuestions) <= 1 {
		return result
	}
	result.SubQuestions = []string{rewritten}
	return result
}

func (s *LLMService) callRewriteLLM(systemPrompt string, question string) (Result, error) {
	request := convention.ChatRequest{
		Messages: []convention.ChatMessage{
			convention.SystemMessage(systemPrompt),
			convention.UserMessage(question),
		},
	}
	response, err := s.chatService.ChatWithRequest(request)
	if err != nil {
		return Result{}, fmt.Errorf("rewrite llm call: %w", err)
	}
	return parseRewriteResponse(response), nil
}

func parseRewriteResponse(raw string) Result {
	raw = strings.TrimSpace(raw)
	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}

	var parsed rewriteLLMResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return fallbackResult(raw)
	}

	rewritten := strings.TrimSpace(parsed.Rewritten)
	subs := normalizeSubQuestions(parsed.SubQuestions, rewritten)
	return Result{
		RewrittenQuestion: rewritten,
		SubQuestions:      subs,
		NeedRetrieval:     normalizeNeedRetrieval(parsed.NeedRetrieval, rewritten),
	}
}

type rewriteLLMResponse struct {
	Rewritten     string   `json:"rewritten"`
	SubQuestions  []string `json:"sub_questions"`
	NeedRetrieval *bool    `json:"need_retrieval"`
}

func fallbackResult(question string) Result {
	question = strings.TrimSpace(question)
	if question == "" {
		return Result{}
	}
	return Result{
		RewrittenQuestion: question,
		SubQuestions:      []string{question},
		NeedRetrieval:     InferNeedRetrieval(question),
	}
}

func normalizeSubQuestions(raw []string, rewritten string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(raw)+1)

	rewritten = strings.TrimSpace(rewritten)
	if rewritten != "" {
		seen[rewritten] = true
		result = append(result, rewritten)
	}

	for _, q := range raw {
		q = strings.TrimSpace(q)
		if q == "" || seen[q] {
			continue
		}
		seen[q] = true
		result = append(result, q)
		if len(result) >= 4 {
			break
		}
	}
	if len(result) == 0 && rewritten != "" {
		result = append(result, rewritten)
	}
	return result
}

func buildRewriteHistoryPrompt(baseSystemPrompt string, history []convention.ChatMessage, question string) string {
	if len(history) == 0 {
		return baseSystemPrompt
	}
	var builder strings.Builder
	builder.WriteString(baseSystemPrompt)
	builder.WriteString("\n\n## 对话历史\n")
	for _, msg := range history {
		switch msg.Role {
		case convention.UserRole:
			builder.WriteString("用户：")
		case convention.AssistantRole:
			builder.WriteString("助手：")
		default:
			continue
		}
		builder.WriteString(strings.TrimSpace(msg.Content))
		builder.WriteString("\n")
	}
	builder.WriteString("\n请根据以上对话历史，对用户的最新问题进行指代消解和改写。")
	builder.WriteString(historyFollowUpPromptSuffix)
	return builder.String()
}

func extractJSONBlock(raw string) string {
	markerStart := strings.Index(raw, "```json")
	if markerStart == -1 {
		markerStart = strings.Index(raw, "```")
	}
	if markerStart == -1 {
		return ""
	}
	contentStart := strings.IndexByte(raw[markerStart:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += markerStart + 1

	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}

func normalizeNeedRetrieval(raw *bool, rewritten string) bool {
	if raw != nil {
		return *raw
	}
	return InferNeedRetrieval(rewritten)
}
