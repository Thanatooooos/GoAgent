package rewrite

import (
	"strings"

	ragretrieve "local/rag-project/internal/app/rag/core/retrieve"
	"local/rag-project/internal/framework/convention"
)

type Result struct {
	RewrittenQuestion   string
	SubQuestions        []string
	PreferredSearchMode string
}

type Service interface {
	Rewrite(question string) string
	RewriteWithSplit(question string) Result
	RewriteWithHistory(question string, history []convention.ChatMessage) Result
}

type DefaultService struct{}

func NewDefaultService() *DefaultService {
	return &DefaultService{}
}

func (s *DefaultService) Rewrite(question string) string {
	return normalize(question)
}

func (s *DefaultService) RewriteWithSplit(question string) Result {
	rewritten := s.Rewrite(question)
	return Result{
		RewrittenQuestion:   rewritten,
		SubQuestions:        defaultSubQuestions(rewritten),
		PreferredSearchMode: InferSearchModePreference(rewritten),
	}
}

func (s *DefaultService) RewriteWithHistory(question string, _ []convention.ChatMessage) Result {
	return s.RewriteWithSplit(question)
}

func normalize(question string) string {
	return strings.TrimSpace(question)
}

func defaultSubQuestions(question string) []string {
	if question == "" {
		return []string{}
	}
	return []string{question}
}

// InferSearchModePreference 根据问题形态推断更合适的检索模式。
func InferSearchModePreference(question string) string {
	question = strings.TrimSpace(strings.ToLower(question))
	if question == "" {
		return ragretrieve.SearchModeSemantic
	}

	// 代码、路径、报错、版本号、配置项等更适合 lexical + semantic 的混合检索。
	hybridHints := []string{
		"`", "/", "\\", ".go", ".java", ".py", ".sql", ".yaml", ".yml", ".json",
		"报错", "异常", "错误", "error", "stack trace", "panic", "nil pointer",
		"配置", "参数", "字段", "函数", "接口", "类", "命令", "sql", "http", "api",
		"nginx", "docker", "k8s", "kubectl", "redis", "mysql", "postgres",
		"v1", "v2", "404", "500",
	}
	for _, hint := range hybridHints {
		if strings.Contains(question, hint) {
			return ragretrieve.SearchModeHybrid
		}
	}

	// 明显的概念解释类问题先走语义检索。
	semanticHints := []string{
		"什么是", "含义", "定义", "原理", "作用", "为什么", "区别", "优点", "缺点", "场景",
		"how", "why", "what is", "difference", "principle", "overview",
	}
	for _, hint := range semanticHints {
		if strings.Contains(question, hint) {
			return ragretrieve.SearchModeSemantic
		}
	}

	// 明显的精确查找倾向问题先走关键词检索。
	keywordHints := []string{
		"包含", "出现", "叫做", "名称", "标题", "匹配", "搜索词", "关键字",
		"contains", "match", "keyword", "named",
	}
	for _, hint := range keywordHints {
		if strings.Contains(question, hint) {
			return ragretrieve.SearchModeKeyword
		}
	}

	return ragretrieve.SearchModeHybrid
}
