package memory_recall

import (
	"context"
	"fmt"
	"strings"

	longtermmemory "local/rag-project/internal/app/rag/service/longtermmemory"

	agentcapability "local/rag-project/internal/app/agent/capability"
	agentstate "local/rag-project/internal/app/agent/state"
)

// MemoryRecaller recalls long-term memories for the current user.
type MemoryRecaller interface {
	RecallMemories(ctx context.Context, input longtermmemory.RecallMemoriesInput) (longtermmemory.RecallMemoriesResult, error)
}

// CapabilityInput describes a memory recall request.
type CapabilityInput struct {
	Query            string   `json:"query"`
	UserID           string   `json:"user_id,omitempty"`
	KnowledgeBaseIDs []string `json:"knowledge_base_ids,omitempty"`
}

// RecallEntry summarizes one recalled memory item.
type RecallEntry struct {
	ID         string `json:"id"`
	MemoryType string `json:"memory_type"`
	Summary    string `json:"summary"`
	Score      int    `json:"score"`
}

// CapabilityOutput is the normalized recall result.
type CapabilityOutput struct {
	Used           bool          `json:"used"`
	Context        string        `json:"context"`
	SelectedCount  int           `json:"selected_count"`
	CandidateCount int           `json:"candidate_count"`
	Entries        []RecallEntry `json:"entries,omitempty"`
	Truncated      bool          `json:"truncated"`
}

type capabilityAdapter struct {
	spec     agentcapability.Spec
	recaller MemoryRecaller
}

// NewCapability builds the memory recall capability.
func NewCapability(recaller MemoryRecaller, options ...agentcapability.Option) (agentcapability.Handle, error) {
	if recaller == nil {
		return nil, fmt.Errorf("memory recaller is required")
	}

	spec := agentcapability.Spec{
		Name:             agentcapability.NameMemoryRecall,
		Kind:             agentcapability.KindTool,
		Family:           agentcapability.FamilyMemory,
		Roles:            []string{agentcapability.RoleRecall},
		Description:      "Recalls relevant long-term memories for the current user and knowledge base scope.",
		InputSchema:      agentcapability.NewSchema(CapabilityInput{}),
		OutputSchema:     agentcapability.NewSchema(CapabilityOutput{}),
		RiskLevel:        agentcapability.RiskLevelLow,
		SupportsParallel: true,
		SupportsResume:   false,
		ProducesEvidence: true,
		Idempotency:      agentcapability.IdempotencyIdempotent,
		Preconditions: []agentcapability.Precondition{
			{
				Field:       "query",
				Requirement: agentcapability.PreconditionRequirementNonEmpty,
				Description: "Memory recall requires a non-empty query.",
			},
		},
	}
	agentcapability.ApplyOptions(&spec, options...)
	return capabilityAdapter{spec: spec, recaller: recaller}, nil
}

func (c capabilityAdapter) Spec() agentcapability.Spec {
	return c.spec
}

func (c capabilityAdapter) NormalizeInput(raw any) (any, error) {
	return agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, raw, "memory recall input is required", "memory recall input")
}

func (c capabilityAdapter) Invoke(ctx context.Context, req agentcapability.InvocationRequest) (agentcapability.InvocationResult, error) {
	input, err := agentcapability.DecodeAndValidateInput[CapabilityInput](c.spec, req.Input, "memory recall input is required", "memory recall input")
	if err != nil {
		return agentcapability.ValidationFailureResult(c.spec, "memory recall rejected", err), err
	}

	userID := strings.TrimSpace(input.UserID)
	if userID == "" {
		userID = strings.TrimSpace(req.Snapshot.Request.UserID)
	}
	if userID == "" {
		err = fmt.Errorf("memory recall requires user_id")
		return agentcapability.ValidationFailureResult(c.spec, "memory recall rejected", err), err
	}

	recallResult, err := c.recaller.RecallMemories(ctx, longtermmemory.RecallMemoriesInput{
		UserID:           userID,
		Query:            strings.TrimSpace(input.Query),
		KnowledgeBaseIDs: input.KnowledgeBaseIDs,
	})
	if err != nil {
		return agentcapability.DependencyFailureResult(c.spec, "memory recall failed", err), err
	}

	output := buildOutput(recallResult)
	note := fmt.Sprintf("Recalled %d memories (%d candidates, truncated=%t)", output.SelectedCount, output.CandidateCount, output.Truncated)
	memoryRefs := toMemoryRefs(recallResult)
	evidenceRefs := toEvidenceRefs(recallResult)

	return agentcapability.InvocationResult{
		Output: output,
		Action: agentcapability.ActionRecord{
			Name:    c.spec.Name,
			Summary: note,
		},
		Observation: agentcapability.ObservationRecord{
			Summary: note,
		},
		Delta: agentstate.StateDelta{
			Context: &agentstate.ContextDelta{
				MemoryRefs: memoryRefs,
				Notes:      agentcapability.AppendNonEmpty(nil, note),
			},
		},
		EvidenceRefs: evidenceRefs,
		Status:       agentcapability.StatusSucceeded,
	}, nil
}

func buildOutput(result longtermmemory.RecallMemoriesResult) CapabilityOutput {
	entries := make([]RecallEntry, 0, len(result.SelectedEntries))
	for _, item := range result.SelectedEntries {
		entries = append(entries, RecallEntry{
			ID:         strings.TrimSpace(item.ID),
			MemoryType: strings.TrimSpace(item.MemoryType),
			Summary:    strings.TrimSpace(item.Summary),
			Score:      item.FinalScore,
		})
	}
	return CapabilityOutput{
		Used:           result.Used,
		Context:        strings.TrimSpace(result.Context),
		SelectedCount:  result.SelectedCount,
		CandidateCount: result.CandidateCount,
		Entries:        entries,
		Truncated:      result.Truncated,
	}
}

func toMemoryRefs(result longtermmemory.RecallMemoriesResult) []agentstate.MemoryRef {
	if len(result.SelectedEntries) == 0 {
		return nil
	}
	refs := make([]agentstate.MemoryRef, 0, len(result.SelectedEntries))
	for _, item := range result.SelectedEntries {
		refs = append(refs, agentstate.MemoryRef{
			ID:      strings.TrimSpace(item.ID),
			Summary: strings.TrimSpace(item.Summary),
		})
	}
	return refs
}

func toEvidenceRefs(result longtermmemory.RecallMemoriesResult) []agentstate.EvidenceRef {
	if len(result.SelectedMemoryIDs) == 0 {
		return nil
	}
	refs := make([]agentstate.EvidenceRef, 0, len(result.SelectedMemoryIDs))
	for _, id := range result.SelectedMemoryIDs {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		refs = append(refs, agentstate.EvidenceRef{
			EvidenceID: "memory:" + id,
			SourceRef:  id,
		})
	}
	return refs
}
