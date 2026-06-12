package knowledge_discovery

import (
	"context"
	"fmt"
	"strings"

	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

const (
	ActionListBases       = "list_bases"
	ActionListDocuments   = "list_documents"
	ActionSearchDocuments = "search_documents"

	defaultPageSize = 10
	maxPageSize     = 50
)

// KnowledgeDiscoverer is the narrow dependency surface for knowledge discovery.
type KnowledgeDiscoverer interface {
	PageBases(ctx context.Context, input knowledgeservice.PageKnowledgeBaseInput) (knowledgeservice.KnowledgeBasePageResult, error)
	PageDocuments(ctx context.Context, input knowledgeservice.PageKnowledgeDocumentInput) (knowledgeservice.KnowledgeDocumentPageResult, error)
	SearchDocuments(ctx context.Context, input knowledgeservice.SearchKnowledgeDocumentsInput) ([]knowledgeservice.KnowledgeDocumentSearchItem, error)
}

// CapabilityInput describes a discovery request.
type CapabilityInput struct {
	Action          string `json:"action"`
	KnowledgeBaseID string `json:"knowledge_base_id,omitempty"`
	Query           string `json:"query,omitempty"`
	Status          string `json:"status,omitempty"`
	Page            int    `json:"page,omitempty"`
}

// BaseItem summarizes a knowledge base row.
type BaseItem struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	DocumentCount int    `json:"document_count"`
}

// DocumentItem summarizes a knowledge document row.
type DocumentItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	KnowledgeBaseID string `json:"knowledge_base_id"`
	Status          string `json:"status,omitempty"`
	ChunkCount      int    `json:"chunk_count,omitempty"`
}

// CapabilityOutput is the normalized discovery result.
type CapabilityOutput struct {
	Action     string         `json:"action"`
	Bases      []BaseItem     `json:"bases,omitempty"`
	Documents  []DocumentItem `json:"documents,omitempty"`
	Total      int            `json:"total"`
	Page       int            `json:"page"`
	Conclusion string         `json:"conclusion"`
}

type capabilityAdapter struct {
	spec       agentcapability.Spec
	discoverer KnowledgeDiscoverer
}

// NewCapability builds the knowledge discovery capability.
func NewCapability(discoverer KnowledgeDiscoverer, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if discoverer == nil {
		return nil, fmt.Errorf("knowledge discoverer is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameKnowledgeDiscovery,
		Kind:             agentcapability.KindTool,
		Family:           agentcapability.FamilyDiscovery,
		Roles:            []string{agentcapability.RoleDiscover},
		Description:      "Discovers knowledge bases and documents for open-ended listing or search queries.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: true,
		SupportsResume:   false,
		ProducesEvidence: false,
		Idempotency:      agentcapability.IdempotencyIdempotent,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "action",
				Requirement: agentcapability.PreconditionRequirementNonEmpty,
				Description: "Knowledge discovery requires a non-empty action.",
			},
		},
	}
	agentcapability.ApplyOptions(&spec, options...)
	return capabilityAdapter{spec: spec, discoverer: discoverer}, nil
}

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "knowledge discovery input is required", "knowledge discovery input")
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "knowledge discovery input is required", "knowledge discovery input")
	if err != nil {
		return agentcapability.ValidationFailureResult(c.spec, "knowledge discovery rejected", err), err
	}

	action := strings.TrimSpace(strings.ToLower(input.Action))
	page := input.Page
	if page <= 0 {
		page = 1
	}

	var output CapabilityOutput
	switch action {
	case ActionListBases:
		output, err = c.listBases(ctx, input, page)
	case ActionListDocuments:
		output, err = c.listDocuments(ctx, input, page)
	case ActionSearchDocuments:
		output, err = c.searchDocuments(ctx, input)
	default:
		err = fmt.Errorf("unsupported knowledge discovery action %q", action)
		return agentcapability.ValidationFailureResult(c.spec, "knowledge discovery rejected", err), err
	}
	if err != nil {
		return agentcapability.DependencyFailureResult(c.spec, fmt.Sprintf("knowledge discovery %s failed", action), err), err
	}

	return agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: output.Conclusion,
		},
		Observation: agentcapability.ObservationRecord{
			Summary: output.Conclusion,
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: agentcapability.AppendNonEmpty(nil, output.Conclusion),
			},
		},
		Status: agentcapability.StatusSucceeded,
	}, nil
}

func (c capabilityAdapter) listBases(ctx context.Context, input CapabilityInput, page int) (CapabilityOutput, error) {
	result, err := c.discoverer.PageBases(ctx, knowledgeservice.PageKnowledgeBaseInput{
		Query:    strings.TrimSpace(input.Query),
		Page:     page,
		PageSize: defaultPageSize,
	})
	if err != nil {
		return CapabilityOutput{}, err
	}

	bases := make([]BaseItem, 0, len(result.Items))
	for _, item := range result.Items {
		bases = append(bases, BaseItem{
			ID:            strings.TrimSpace(item.ID),
			Name:          strings.TrimSpace(item.Name),
			DocumentCount: result.DocumentCounts[item.ID],
		})
	}
	return CapabilityOutput{
		Action:     ActionListBases,
		Bases:      bases,
		Total:      result.Total,
		Page:       result.Page,
		Conclusion: fmt.Sprintf("Found %d knowledge bases", len(bases)),
	}, nil
}

func (c capabilityAdapter) listDocuments(ctx context.Context, input CapabilityInput, page int) (CapabilityOutput, error) {
	result, err := c.discoverer.PageDocuments(ctx, knowledgeservice.PageKnowledgeDocumentInput{
		KnowledgeBaseID: strings.TrimSpace(input.KnowledgeBaseID),
		Query:           strings.TrimSpace(input.Query),
		Status:          strings.TrimSpace(input.Status),
		Page:            page,
		PageSize:        defaultPageSize,
	})
	if err != nil {
		return CapabilityOutput{}, err
	}

	documents := make([]DocumentItem, 0, len(result.Items))
	failedCount := 0
	for _, item := range result.Items {
		if strings.TrimSpace(item.Status) == knowledgedomain.KnowledgeDocumentStatusFailed {
			failedCount++
		}
		documents = append(documents, DocumentItem{
			ID:              strings.TrimSpace(item.ID),
			Name:            strings.TrimSpace(item.Name),
			KnowledgeBaseID: strings.TrimSpace(item.KnowledgeBaseID),
			Status:          strings.TrimSpace(item.Status),
			ChunkCount:      item.ChunkCount,
		})
	}
	conclusion := fmt.Sprintf("Found %d documents", len(documents))
	if kbID := strings.TrimSpace(input.KnowledgeBaseID); kbID != "" {
		conclusion = fmt.Sprintf("Found %d documents in knowledge base %s", len(documents), kbID)
	}
	if failedCount > 0 {
		conclusion = fmt.Sprintf("%s, %d failed", conclusion, failedCount)
	}
	return CapabilityOutput{
		Action:     ActionListDocuments,
		Documents:  documents,
		Total:      result.Total,
		Page:       result.Page,
		Conclusion: conclusion,
	}, nil
}

func (c capabilityAdapter) searchDocuments(ctx context.Context, input CapabilityInput) (CapabilityOutput, error) {
	items, err := c.discoverer.SearchDocuments(ctx, knowledgeservice.SearchKnowledgeDocumentsInput{
		Query: strings.TrimSpace(input.Query),
		Limit: defaultPageSize,
	})
	if err != nil {
		return CapabilityOutput{}, err
	}

	documents := make([]DocumentItem, 0, len(items))
	for _, item := range items {
		documents = append(documents, DocumentItem{
			ID:              strings.TrimSpace(item.ID),
			Name:            strings.TrimSpace(item.Name),
			KnowledgeBaseID: strings.TrimSpace(item.KnowledgeBaseID),
		})
	}
	return CapabilityOutput{
		Action:     ActionSearchDocuments,
		Documents:  documents,
		Total:      len(documents),
		Page:       1,
		Conclusion: fmt.Sprintf("Found %d matching documents", len(documents)),
	}, nil
}
