package rewrite

import (
	"strings"

	"local/rag-project/internal/framework/convention"
)

type Result struct {
	RewrittenQuestion string
	SubQuestions      []string
	NeedRetrieval     bool
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
		RewrittenQuestion: rewritten,
		SubQuestions:      defaultSubQuestions(rewritten),
		NeedRetrieval:     InferNeedRetrieval(rewritten),
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

func InferNeedRetrieval(question string) bool {
	question = strings.TrimSpace(strings.ToLower(question))
	if question == "" {
		return false
	}

	noRetrievePhrases := []string{
		"你好", "您好", "hi", "hello", "hey",
		"谢谢", "感谢", "thank you", "thanks",
		"再见", "拜拜", "bye", "goodbye",
		"你是谁", "你是干什么的", "你能做什么", "介绍一下你自己",
	}
	for _, phrase := range noRetrievePhrases {
		if question == phrase || strings.HasPrefix(question, phrase+" ") || strings.HasPrefix(question, phrase+"，") || strings.HasPrefix(question, phrase+"。") {
			return false
		}
	}

	if len([]rune(question)) <= 12 {
		shortChatPhrases := []string{"在吗", "忙吗", "收到吗", "好的", "嗯", "哦", "行"}
		for _, phrase := range shortChatPhrases {
			if question == phrase {
				return false
			}
		}
	}

	return true
}
