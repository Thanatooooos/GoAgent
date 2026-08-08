package runner

import (
	"context"
	"fmt"
	"strings"
)

type EnrichmentOptions struct {
	QuestionCount     int
	MaxQuestionLength int
	SummaryMaxChars   int
}

type DocumentEnricher interface {
	Summarize(context.Context, string, EnrichmentOptions) (string, error)
	GenerateQuestions(context.Context, string, string, EnrichmentOptions) ([]string, error)
}

type PromptCompleter interface{ Chat(string) (string, error) }

type llmDocumentEnricher struct{ client PromptCompleter }

func NewLLMDocumentEnricher(client PromptCompleter) DocumentEnricher {
	return &llmDocumentEnricher{client: client}
}

func (e *llmDocumentEnricher) Summarize(_ context.Context, content string, _ EnrichmentOptions) (string, error) {
	if e == nil || e.client == nil {
		return "", fmt.Errorf("document enrichment chat client is required")
	}
	return e.client.Chat("请只基于以下文档内容生成简洁摘要，不要添加输入之外的信息：\n\n" + content)
}

func (e *llmDocumentEnricher) GenerateQuestions(_ context.Context, title string, content string, options EnrichmentOptions) ([]string, error) {
	if e == nil || e.client == nil {
		return nil, fmt.Errorf("document enrichment chat client is required")
	}
	prompt := fmt.Sprintf("为下面内容生成最多 %d 个可由该段充分回答的用户问题，每行一个，不要编号。标题：%s\n\n内容：%s", options.QuestionCount, strings.TrimSpace(title), strings.TrimSpace(content))
	response, err := e.client.Chat(prompt)
	if err != nil {
		return nil, err
	}
	return normalizeGeneratedQuestions(strings.Split(response, "\n"), options.QuestionCount, options.MaxQuestionLength), nil
}

func normalizeGeneratedQuestions(values []string, limit int, maxLength int) []string {
	if limit <= 0 || maxLength <= 0 {
		return nil
	}
	result := make([]string, 0, limit)
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len([]rune(value)) > maxLength {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
		if len(result) == limit {
			break
		}
	}
	return result
}

func sampleDocumentForSummary(content string, maxLength int) string {
	content = strings.TrimSpace(content)
	if maxLength <= 0 || len([]rune(content)) <= maxLength {
		return content
	}
	runes := []rune(content)
	partLength := maxLength / 3
	if partLength == 0 {
		return string(runes[:maxLength])
	}
	middleStart := len(runes)/2 - partLength/2
	middleEnd := middleStart + partLength
	return string(runes[:partLength]) + "\n[...内容省略...]\n" + string(runes[middleStart:middleEnd]) + "\n[...内容省略...]\n" + string(runes[len(runes)-partLength:])
}
