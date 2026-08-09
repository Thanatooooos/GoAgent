package wiki_write

import (
	"context"
	"fmt"
	"strings"

	"local/rag-project/internal/app/knowledge/domain"
	wikiservice "local/rag-project/internal/app/knowledge/service/wiki"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// DocumentContentReader 取文档解析内容（标题 + 正文）。
type DocumentContentReader interface {
	ReadContent(ctx context.Context, documentID string) (title string, content string, err error)
}

// WikiWriter 是 wiki 持久化端口（由 *wikiservice.WikiPageService 满足）。
type WikiWriter interface {
	UpsertPagesFromDocument(ctx context.Context, kbID string, pages []domain.WikiPage, links []domain.WikiLink) error
	LinkifyAndPersist(ctx context.Context, kbID string, pages []domain.WikiPage, extraLinks []domain.WikiLink) (int, error)
}

// PromptCompleter 供 WikiGenerator 调用。
type PromptCompleter interface {
	Chat(prompt string) (string, error)
}

type CapabilityInput struct {
	KnowledgeBaseID string `json:"knowledge_base_id"`
	DocumentID      string `json:"document_id"`
	MaxPages        int    `json:"max_pages,omitempty"`
	PageType        string `json:"page_type,omitempty"`
	Instruction     string `json:"instruction,omitempty"`
}

type CapabilityOutput struct {
	DocumentID string   `json:"document_id"`
	PageIDs    []string `json:"page_ids,omitempty"`
	PageCount  int      `json:"page_count"`
	LinkCount  int      `json:"link_count"`
}

type capabilityAdapter struct {
	spec      agentcapability.Spec
	reader    DocumentContentReader
	writer    WikiWriter
	completer PromptCompleter
}

func NewCapability(reader DocumentContentReader, writer WikiWriter, completer PromptCompleter, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if reader == nil || writer == nil || completer == nil {
		return nil, fmt.Errorf("wiki write deps are required")
	}
	spec := agentcapability.Spec{
		Name:             agentcapability.NameWikiWrite,
		Kind:             agentcapability.KindWorkflow,
		Family:           agentcapability.FamilyWiki,
		Roles:            []string{agentcapability.RoleWriteWiki},
		Description:      "Generates wiki pages from a knowledge document and persists them as a linked wiki.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelMedium,
		SupportsParallel: false,
		SupportsResume:   false,
		ProducesEvidence: true,
		Idempotency:      agentcapability.IdempotencyBestEffort,
		Preconditions: []agentcapability.Precondition{
			{Field: "knowledge_base_id", Requirement: agentcapability.PreconditionRequirementNonEmpty, Description: "Wiki write requires a knowledge base id."},
			{Field: "document_id", Requirement: agentcapability.PreconditionRequirementNonEmpty, Description: "Wiki write requires a document id."},
		},
	}
	agentcapability.ApplyOptions(&spec, options...)
	return capabilityAdapter{spec: spec, reader: reader, writer: writer, completer: completer}, nil
}

func (c capabilityAdapter) Spec() agentcapability.Spec { return c.spec }

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "wiki write input is required", "wiki write input")
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "wiki write input is required", "wiki write input")
	if err != nil {
		return agentcapability.ValidationFailureResult(c.spec, "wiki write rejected", err), err
	}
	title, content, err := c.reader.ReadContent(ctx, input.DocumentID)
	if err != nil {
		return agentcapability.DependencyFailureResult(c.spec, "document content read failed", err), err
	}
	if strings.TrimSpace(content) == "" {
		return agentcapability.ExternalFailureResult(c.spec, "empty document content", nil), nil
	}
	generator := wikiservice.NewLLMWikiGenerator(c.completer)
	result, err := generator.GenerateFromDocument(ctx, title, content, wikiservice.WikiGenerationOptions{
		MaxPages: input.MaxPages,
		PageType: input.PageType,
	})
	if err != nil || len(result.Pages) == 0 {
		return agentcapability.ExternalFailureResult(c.spec, "wiki generation empty", nil), nil
	}
	if err := c.writer.UpsertPagesFromDocument(ctx, input.KnowledgeBaseID, result.Pages, result.Links); err != nil {
		return agentcapability.DependencyFailureResult(c.spec, "wiki persist failed", err), err
	}
	linkCount, err := c.writer.LinkifyAndPersist(ctx, input.KnowledgeBaseID, result.Pages, result.Links)
	if err != nil {
		return agentcapability.DependencyFailureResult(c.spec, "wiki linkify failed", err), err
	}
	output := CapabilityOutput{
		DocumentID: input.DocumentID,
		PageCount:  len(result.Pages),
		LinkCount:  linkCount,
	}
	return agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: fmt.Sprintf("generate wiki from document %q: %d pages, %d links", output.DocumentID, output.PageCount, output.LinkCount),
		},
		Observation: agentcapability.ObservationRecord{Summary: fmt.Sprintf("wiki generated %d pages / %d links", output.PageCount, output.LinkCount)},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{Notes: []string{fmt.Sprintf("wiki pages generated from document %s", output.DocumentID)}},
		},
		Status: agentcapability.StatusSucceeded,
	}, nil
}
