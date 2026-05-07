package rewrite

import (
	"encoding/json"
	"fmt"
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
	aichat "local/rag-project/internal/infra-ai/chat"
)

const (
	defaultRewriteModel = "default"
	rewriteSystemPrompt = `你是一个查询改写助手。你的任务是将用户问题优化为更适合知识库检索的查询。

你必须遵守以下规则：
1. 将指代词（如"它"、"这个"、"那个"、"上面提到的"）替换为具体实体名，使问题可独立理解。
2. 去除口语冗余，保留核心语义。
3. 将复杂问题拆解为 2-3 个独立的检索子问题。
4. 根据问题特点推断更合适的检索模式：
   - semantic：概念解释、原理、区别、总结类问题
   - keyword：名称、标题、精确短语匹配类问题
   - hybrid：代码、报错、参数、接口、配置、术语定位类问题

示例：
用户问："RAG是什么"
输出：{"rewritten": "什么是RAG检索增强生成技术", "sub_questions": ["RAG技术定义", "检索增强生成原理"], "preferred_search_mode": "semantic"}

用户问：上一条消息是"什么是向量数据库"，当前问："它有哪些应用场景"
输出：{"rewritten": "向量数据库有哪些应用场景", "sub_questions": ["向量数据库应用场景", "向量数据库典型用途"], "preferred_search_mode": "semantic"}

用户问：上一条消息是"K8s是什么"，当前问："怎么部署它"
输出：{"rewritten": "如何部署Kubernetes集群", "sub_questions": ["Kubernetes部署方法", "K8s安装步骤"], "preferred_search_mode": "hybrid"}

你必须严格按 JSON 格式输出，不要输出其他内容：
{"rewritten": "改写后的主问题（独立的、去指代后的完整问题）", "sub_questions": ["检索子问题1", "检索子问题2"], "preferred_search_mode": "semantic|keyword|hybrid"}`
)

// LLMService 依赖 LLM 服务实现查询改写与扩展。
type LLMService struct {
	chatService aichat.LLMService
}

// NewLLMService 创建基于 LLM 的 rewrite 服务。
func NewLLMService(chatService aichat.LLMService) *LLMService {
	return &LLMService{chatService: chatService}
}

// Rewrite 对单个问题进行改写优化。
func (s *LLMService) Rewrite(question string) string {
	result := s.RewriteWithSplit(question)
	return result.RewrittenQuestion
}

// RewriteWithSplit 将问题改写并拆解为多个子问题。
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
	return parsed
}

// RewriteWithHistory 结合对话历史进行指代消解后再改写。
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
	return parsed
}

// callRewriteLLM 调用 LLM 执行一次改写请求。
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

	// 尝试提取 JSON 块（LLM 有时会包在 markdown 代码块里）
	if extracted := extractJSONBlock(raw); extracted != "" {
		raw = extracted
	}

	var parsed rewriteLLMResponse
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		// JSON 解析失败时退化为单问题改写
		return fallbackResult(raw)
	}

	rewritten := strings.TrimSpace(parsed.Rewritten)
	subs := normalizeSubQuestions(parsed.SubQuestions, rewritten)
	return Result{
		RewrittenQuestion:   rewritten,
		SubQuestions:        subs,
		PreferredSearchMode: normalizePreferredSearchMode(parsed.PreferredSearchMode, rewritten),
	}
}

type rewriteLLMResponse struct {
	Rewritten           string   `json:"rewritten"`
	SubQuestions        []string `json:"sub_questions"`
	PreferredSearchMode string   `json:"preferred_search_mode"`
}

func fallbackResult(question string) Result {
	question = strings.TrimSpace(question)
	if question == "" {
		return Result{}
	}
	return Result{
		RewrittenQuestion:   question,
		SubQuestions:        []string{question},
		PreferredSearchMode: InferSearchModePreference(question),
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
	// 找到代码块标记后的第一个换行，内容从换行后开始。
	contentStart := strings.IndexByte(raw[markerStart:], '\n')
	if contentStart == -1 {
		return ""
	}
	contentStart += markerStart + 1

	// 找闭合的 ``` 标记，内容在闭合标记前结束。
	end := strings.Index(raw[contentStart:], "```")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(raw[contentStart : contentStart+end])
}

func normalizePreferredSearchMode(raw string, rewritten string) string {
	mode := strings.TrimSpace(strings.ToLower(raw))
	switch mode {
	case ragretrieve.SearchModeSemantic, ragretrieve.SearchModeKeyword, ragretrieve.SearchModeHybrid:
		return mode
	default:
		return InferSearchModePreference(rewritten)
	}
}
