package runner

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"local/rag-project/internal/framework/llmgen"
)

type EnrichmentOptions struct {
	QuestionCount     int
	MaxQuestionLength int
	SummaryMaxChars   int
	SourceChunkID     string
}

type GeneratedQuestion struct {
	Text          string
	SourceChunkID string
}

type GenerateQuestionsResult struct {
	Questions    []GeneratedQuestion
	RejectedRefs int
}

type DocumentEnricher interface {
	Summarize(context.Context, string, EnrichmentOptions) (string, error)
	GenerateQuestions(context.Context, string, string, EnrichmentOptions) (GenerateQuestionsResult, error)
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

var questionRefRE = regexp.MustCompile(`(?i)\[\[(r[1-9][0-9]*)\]\]`)

func (e *llmDocumentEnricher) GenerateQuestions(_ context.Context, title string, content string, options EnrichmentOptions) (GenerateQuestionsResult, error) {
	if e == nil || e.client == nil {
		return GenerateQuestionsResult{}, fmt.Errorf("document enrichment chat client is required")
	}
	sourceChunkID := strings.TrimSpace(options.SourceChunkID)
	sourceHint := ""
	var handles *llmgen.HandleSet
	if sourceChunkID != "" {
		handles = llmgen.NewHandleSet("r")
		if sourceHandle, ok := handles.Encode(sourceChunkID); ok {
			sourceHint = fmt.Sprintf("\n当前内容块句柄为 %s；若问题依赖本段内容，请在问题末尾追加 [[%s]]。", sourceHandle, sourceHandle)
		}
	}
	prompt := fmt.Sprintf("为下面内容生成最多 %d 个可由该段充分回答的用户问题，每行一个，不要编号。标题：%s\n\n内容：%s%s", options.QuestionCount, strings.TrimSpace(title), strings.TrimSpace(content), sourceHint)
	response, err := e.client.Chat(prompt)
	if err != nil {
		return GenerateQuestionsResult{}, err
	}
	raw := normalizeGeneratedQuestions(strings.Split(response, "\n"), options.QuestionCount, options.MaxQuestionLength)
	return resolveGeneratedQuestions(raw, handles, sourceChunkID), nil
}

// resolveGeneratedQuestions 解析问题行尾的 [[rN]] 引用：命中句柄则记录来源并剥离标记；
// 未命中句柄（幻觉引用）只剥除标记、不设置来源，并计入 RejectedRefs；问题本身保留。
func resolveGeneratedQuestions(values []string, handles *llmgen.HandleSet, sourceChunkID string) GenerateQuestionsResult {
	validator := llmgen.NewRefValidator([]string{sourceChunkID})
	result := GenerateQuestionsResult{Questions: make([]GeneratedQuestion, 0, len(values))}
	for _, value := range values {
		question := GeneratedQuestion{Text: strings.TrimSpace(value)}
		if handles == nil {
			result.Questions = append(result.Questions, question)
			continue
		}
		rules := make([]llmgen.RewriteRule, 0)
		for _, match := range questionRefRE.FindAllStringSubmatch(question.Text, -1) {
			handle := strings.ToLower(match[1])
			rules = append(rules, llmgen.RewriteRule{Find: match[0], Replace: ""})
			if id, ok := handles.Resolve(handle); ok {
				if _, reason := validator.Validate(id); reason == llmgen.ReasonOK {
					question.SourceChunkID = id
				} else {
					result.RejectedRefs++
				}
			} else {
				result.RejectedRefs++
			}
		}
		if len(rules) > 0 {
			cleaned, _ := llmgen.RewriteRefs(question.Text, rules, llmgen.RewriteOptions{SkipLinks: true, WordBoundary: true})
			question.Text = strings.TrimSpace(cleaned)
		}
		result.Questions = append(result.Questions, question)
	}
	return result
}

// questionTexts 提取问题文本，供 runner 写回 Chunk.Questions。
func questionTexts(questions []GeneratedQuestion) []string {
	result := make([]string, 0, len(questions))
	for _, question := range questions {
		result = append(result, question.Text)
	}
	return result
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
