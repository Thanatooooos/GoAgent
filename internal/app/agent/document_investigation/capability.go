package document_investigation

import (
	"context"
	"fmt"
	"strings"

	ingestiondomain "local/rag-project/internal/app/ingestion/domain"
	knowledgedomain "local/rag-project/internal/app/knowledge/domain"
	knowledgeservice "local/rag-project/internal/app/knowledge/service"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// Investigator is the application-layer dependency needed by the
// document-investigation workflow capability.
type Investigator interface {
	Get(ctx context.Context, input knowledgeservice.GetKnowledgeDocumentInput) (knowledgedomain.KnowledgeDocument, error)
	PageChunkLogs(ctx context.Context, input knowledgeservice.KnowledgeDocumentChunkLogPageInput) (knowledgeservice.KnowledgeDocumentChunkLogPageResult, error)
}

// CapabilityInput is the typed invocation input for document investigation.
type CapabilityInput struct {
	DocumentID string `json:"document_id"`
}

// CapabilityOutput is the normalized workflow result returned to runtime.
type CapabilityOutput struct {
	DocumentID      string   `json:"document_id"`
	DocumentName    string   `json:"document_name,omitempty"`
	DocumentStatus  string   `json:"document_status,omitempty"`
	ProcessMode     string   `json:"process_mode,omitempty"`
	PipelineID      string   `json:"pipeline_id,omitempty"`
	ChunkCount      int      `json:"chunk_count,omitempty"`
	LatestTaskID    string   `json:"latest_task_id,omitempty"`
	LatestNodeID    string   `json:"latest_node_id,omitempty"`
	LatestNodeError string   `json:"latest_node_error,omitempty"`
	LatestLogStatus string   `json:"latest_log_status,omitempty"`
	LatestLogError  string   `json:"latest_log_error,omitempty"`
	Conclusion      string   `json:"conclusion,omitempty"`
	Confidence      string   `json:"confidence,omitempty"`
	Evidence        []string `json:"evidence,omitempty"`
	Suggestions     []string `json:"suggestions,omitempty"`
}

type capabilityAdapter struct {
	spec         agentcapability.Spec
	investigator Investigator
}

// NewCapability builds the high-level document investigation workflow capability.
func NewCapability(investigator Investigator, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if investigator == nil {
		return nil, fmt.Errorf("document investigator is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameDocumentInvestigation,
		Kind:             agentcapability.KindWorkflow,
		Family:           agentcapability.FamilyDocumentInvestigation,
		Roles:            []string{agentcapability.RoleInvestigateDocument},
		Description:      "Investigates a knowledge document's ingestion state and returns a structured diagnosis.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: false,
		SupportsResume:   false,
		ProducesEvidence: true,
		Idempotency:      agentcapability.IdempotencyIdempotent,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "document_id",
				Requirement: "non_empty",
				Description: "Document investigation requires a non-empty document id.",
			},
		},
	}
	for _, option := range options {
		if option != nil {
			option(&spec)
		}
	}

	return capabilityAdapter{
		spec:         spec,
		investigator: investigator,
	}, nil
}

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return decodeCapabilityInput(raw)
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := decodeCapabilityInput(req.Input)
	if err != nil {
		return validationFailure(c.spec, err), err
	}
	if err := agentcapability.ValidateInput(c.spec, input); err != nil {
		return validationFailure(c.spec, err), err
	}

	document, err := c.investigator.Get(ctx, knowledgeservice.GetKnowledgeDocumentInput{
		DocumentID: input.DocumentID,
	})
	if err != nil {
		return dependencyFailure(c.spec, "document lookup failed", err), err
	}
	chunkLogs, err := c.investigator.PageChunkLogs(ctx, knowledgeservice.KnowledgeDocumentChunkLogPageInput{
		DocumentID: input.DocumentID,
		Page:       1,
		PageSize:   3,
	})
	if err != nil {
		return dependencyFailure(c.spec, "document chunk-log lookup failed", err), err
	}

	output := diagnose(document, chunkLogs)
	summary := fmt.Sprintf("document=%s confidence=%s conclusion=%s", output.DocumentID, output.Confidence, output.Conclusion)

	return agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: fmt.Sprintf("investigate document %q", strings.TrimSpace(output.DocumentID)),
		},
		Observation: agentcapability.ObservationRecord{
			Summary: summary,
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				Notes: append([]string(nil), output.Evidence...),
			},
			Evidence: &agentstate.EvidenceDelta{
				AddItems: []agentstate.EvidenceItem{
					{
						ID:        "document_investigation:" + output.DocumentID,
						Source:    "document_investigation",
						Content:   output.Conclusion,
						Level:     output.Confidence,
						SourceRef: output.DocumentID,
					},
				},
			},
		},
		Status: agentcapability.StatusSucceeded,
		EvidenceRefs: []agentstate.EvidenceRef{
			{
				EvidenceID: "document_investigation:" + output.DocumentID,
				SourceRef:  output.DocumentID,
			},
		},
	}, nil
}

func decodeCapabilityInput(raw any) (CapabilityInput, error) {
	input, err := agentcapability.DecodeStructuredInput[CapabilityInput](raw, "document investigation input is required")
	if err != nil {
		return CapabilityInput{}, fmt.Errorf("document investigation input has unexpected type %T: %w", raw, err)
	}
	return input, nil
}

func validationFailure(spec agentcapability.Spec, err error) agentcapability.InvocationResult {
	return agentcapability.InvocationResult{
		Action: agentcapability.ActionRecord{
			Name:    spec.Name,
			Summary: "document investigation rejected",
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    err.Error(),
			Degraded:   true,
			ErrorClass: agentcapability.ErrorClassValidation,
		},
		Status:     agentcapability.StatusDegraded,
		ErrorClass: agentcapability.ErrorClassValidation,
	}
}

func dependencyFailure(spec agentcapability.Spec, summary string, err error) agentcapability.InvocationResult {
	return agentcapability.InvocationResult{
		Action: agentcapability.ActionRecord{
			Name:    spec.Name,
			Summary: summary,
		},
		Observation: agentcapability.ObservationRecord{
			Summary:    err.Error(),
			Degraded:   true,
			ErrorClass: agentcapability.ErrorClassDependency,
		},
		Status:     agentcapability.StatusDegraded,
		ErrorClass: agentcapability.ErrorClassDependency,
	}
}

var (
	_ Investigator = (*knowledgeservice.KnowledgeDocumentService)(nil)
	_              = ingestiondomain.Task{}
)
