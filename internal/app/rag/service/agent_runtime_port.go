package service

import (
	"context"
	"time"

	agentapp "local/rag-project/internal/app/agent"
)

// AgentRuntimeService describes the agent runtime contract consumed by rag chat orchestration.
type AgentRuntimeService interface {
	RunDetailed(ctx context.Context, req agentapp.Request) (agentapp.RunResponse, error)
	ResumeAfterApproval(ctx context.Context, req agentapp.ResumeApprovalRequest) (agentapp.RunResponse, error)
	GetPendingApproval(ctx context.Context, req agentapp.PendingApprovalLookupRequest) (*agentapp.ApprovalPending, bool, error)
}

type RagChatApprovalResumeInput struct {
	ConversationID string
	UserID         string
	Question       string
	CheckpointID   string
	Decision       string
	DecisionNote   string
}

type RagChatApprovalPendingQueryInput struct {
	ConversationID string
	UserID         string
}

type RagChatAgentOutcomePayload struct {
	Status          string `json:"status"`
	Interrupted     bool   `json:"interrupted"`
	InterruptReason string `json:"interruptReason,omitempty"`
	CheckpointID    string `json:"checkpointId,omitempty"`
}

type RagChatApprovalPendingPayload struct {
	Required              bool      `json:"required"`
	Status                string    `json:"status,omitempty"`
	Reason                string    `json:"reason,omitempty"`
	ReasonCode            string    `json:"reasonCode,omitempty"`
	ReasonMessage         string    `json:"reasonMessage,omitempty"`
	Trigger               string    `json:"trigger,omitempty"`
	Node                  string    `json:"node,omitempty"`
	RerunNode             string    `json:"rerunNode,omitempty"`
	Capability            string    `json:"capability,omitempty"`
	CapabilityName        string    `json:"capabilityName,omitempty"`
	CapabilityKind        string    `json:"capabilityKind,omitempty"`
	CapabilityFamily      string    `json:"capabilityFamily,omitempty"`
	CapabilityDescription string    `json:"capabilityDescription,omitempty"`
	RiskLevel             string    `json:"riskLevel,omitempty"`
	SupportsResume        bool      `json:"supportsResume"`
	Idempotency           string    `json:"idempotency,omitempty"`
	CheckpointID          string    `json:"checkpointId,omitempty"`
	SessionID             string    `json:"sessionId,omitempty"`
	RequestedAt           time.Time `json:"requestedAt,omitempty"`
	ResumeCount           int       `json:"resumeCount,omitempty"`
	Question              string    `json:"question,omitempty"`
	SearchQuery           string    `json:"searchQuery,omitempty"`
	CurrentStepID         string    `json:"currentStepId,omitempty"`
	CurrentStepTitle      string    `json:"currentStepTitle,omitempty"`
	CandidateURLs         []string  `json:"candidateUrls,omitempty"`
	CanApprove            bool      `json:"canApprove"`
	CanReject             bool      `json:"canReject"`
	RejectOutcome         string    `json:"rejectOutcome,omitempty"`
}

type RagChatAgentServiceErrorPayload struct {
	Code      string `json:"code,omitempty"`
	Message   string `json:"message,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Retryable bool   `json:"retryable"`
}

func newRagChatAgentOutcomePayload(outcome agentapp.RunOutcome) RagChatAgentOutcomePayload {
	return RagChatAgentOutcomePayload{
		Status:          outcome.Status,
		Interrupted:     outcome.Interrupted,
		InterruptReason: outcome.InterruptReason,
		CheckpointID:    outcome.CheckpointID,
	}
}

func newRagChatApprovalPendingPayload(pending *agentapp.ApprovalPending) *RagChatApprovalPendingPayload {
	if pending == nil {
		return nil
	}
	return &RagChatApprovalPendingPayload{
		Required:              pending.Required,
		Status:                pending.Status,
		Reason:                pending.Reason,
		ReasonCode:            pending.ReasonCode,
		ReasonMessage:         pending.ReasonMessage,
		Trigger:               pending.Trigger,
		Node:                  pending.Node,
		RerunNode:             pending.RerunNode,
		Capability:            pending.Capability,
		CapabilityName:        pending.CapabilityName,
		CapabilityKind:        pending.CapabilityKind,
		CapabilityFamily:      pending.CapabilityFamily,
		CapabilityDescription: pending.CapabilityDescription,
		RiskLevel:             pending.RiskLevel,
		SupportsResume:        pending.SupportsResume,
		Idempotency:           pending.Idempotency,
		CheckpointID:          pending.CheckpointID,
		SessionID:             pending.SessionID,
		RequestedAt:           pending.RequestedAt,
		ResumeCount:           pending.ResumeCount,
		Question:              pending.Question,
		SearchQuery:           pending.SearchQuery,
		CurrentStepID:         pending.CurrentStepID,
		CurrentStepTitle:      pending.CurrentStepTitle,
		CandidateURLs:         append([]string(nil), pending.CandidateURLs...),
		CanApprove:            pending.CanApprove,
		CanReject:             pending.CanReject,
		RejectOutcome:         pending.RejectOutcome,
	}
}

func newRagChatAgentServiceErrorPayload(err error) RagChatAgentServiceErrorPayload {
	desc := agentapp.DescribeServiceError(err)
	return RagChatAgentServiceErrorPayload{
		Code:      desc.Code,
		Message:   desc.Message,
		Kind:      firstNonEmptyString(desc.Kind, agentapp.ErrorKindInternal),
		Retryable: desc.Retryable,
	}
}
