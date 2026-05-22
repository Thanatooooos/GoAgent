package prompt

import (
	"fmt"
	"strings"

	"local/rag-project/internal/framework/convention"
)

type Context struct {
	Question         string
	MemoryContext    string
	SessionContext   string
	KnowledgeContext string
	ToolContext      string
	WorkflowPolicy   string
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

	messages := make([]convention.ChatMessage, 0, len(ctx.History)+6)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, convention.SystemMessage(systemPrompt))
	}
	if strings.TrimSpace(ctx.MemoryContext) != "" {
		messages = append(messages, convention.SystemMessage(formatMemoryContext(ctx.MemoryContext)))
	}
	if strings.TrimSpace(ctx.SessionContext) != "" {
		messages = append(messages, convention.SystemMessage(formatSessionContext(ctx.SessionContext)))
	}
	if strings.TrimSpace(ctx.KnowledgeContext) != "" {
		messages = append(messages, convention.SystemMessage(formatKnowledgeContext(ctx.KnowledgeContext)))
	}
	if strings.TrimSpace(ctx.ToolContext) != "" {
		messages = append(messages, convention.SystemMessage(formatToolContext(ctx.ToolContext)))
	}
	if strings.TrimSpace(ctx.WorkflowPolicy) != "" {
		messages = append(messages, convention.SystemMessage(formatWorkflowPolicy(ctx.WorkflowPolicy)))
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

func formatMemoryContext(context string) string {
	return "## Long-Term Memory\nUse these persistent user- or knowledge-base-specific memories when they are relevant to the current question.\n" + strings.TrimSpace(context)
}

func formatSessionContext(context string) string {
	return "## 会话上下文片段\n" + strings.TrimSpace(context)
}

func formatKnowledgeContext(context string) string {
	return "## Knowledge Context\n" + strings.TrimSpace(context)
}

func formatToolContext(context string) string {
	return "## Tool Context\n" + strings.TrimSpace(context)
}

func formatWorkflowPolicy(policy string) string {
	return "## Workflow Policy\n" + strings.TrimSpace(policy)
}

func formatAnswerGuidance(guidance string) string {
	return "## Answer Guidance\n" + strings.TrimSpace(guidance)
}
