package prompt

import (
	"fmt"
	"strings"

	"local/rag-project/internal/framework/convention"
)

type Context struct {
	Question         string
	KnowledgeContext string
	ToolContext      string
	AnswerGuidance   string
	History          []convention.ChatMessage
	SystemPromptKey  string
	SystemPrompt     string
}

type Service struct {
	loader *TemplateLoader
}

func NewService(loader *TemplateLoader) *Service {
	if loader == nil {
		loader = NewTemplateLoader()
	}
	return &Service{loader: loader}
}

func (s *Service) BuildSystemPrompt(ctx Context) (string, error) {
	if strings.TrimSpace(ctx.SystemPrompt) != "" {
		return cleanup(ctx.SystemPrompt), nil
	}

	key := ctx.SystemPromptKey
	if strings.TrimSpace(key) == "" {
		key = DefaultKBTemplate
	}
	return s.loader.Load(key)
}

func (s *Service) BuildMessages(ctx Context) ([]convention.ChatMessage, error) {
	systemPrompt, err := s.BuildSystemPrompt(ctx)
	if err != nil {
		return nil, fmt.Errorf("build system prompt: %w", err)
	}

	messages := make([]convention.ChatMessage, 0, len(ctx.History)+5)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, convention.SystemMessage(systemPrompt))
	}
	if strings.TrimSpace(ctx.KnowledgeContext) != "" {
		messages = append(messages, convention.SystemMessage(formatKnowledgeContext(ctx.KnowledgeContext)))
	}
	if strings.TrimSpace(ctx.ToolContext) != "" {
		messages = append(messages, convention.SystemMessage(formatToolContext(ctx.ToolContext)))
	}
	if strings.TrimSpace(ctx.AnswerGuidance) != "" {
		messages = append(messages, convention.SystemMessage(formatAnswerGuidance(ctx.AnswerGuidance)))
	}
	if len(ctx.History) > 0 {
		messages = append(messages, ctx.History...)
	}
	if strings.TrimSpace(ctx.Question) != "" {
		messages = append(messages, convention.UserMessage(strings.TrimSpace(ctx.Question)))
	}
	return messages, nil
}

func formatKnowledgeContext(context string) string {
	return "## 知识上下文\n" + strings.TrimSpace(context)
}

func formatToolContext(context string) string {
	return "## 工具上下文\n" + strings.TrimSpace(context)
}

func formatAnswerGuidance(guidance string) string {
	return "## 回答要求\n" + strings.TrimSpace(guidance)
}
