package rewrite

import (
	"strings"

	"local/rag-project/internal/framework/convention"
)

type Result struct {
	RewrittenQuestion string
	SubQuestions      []string
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
