package rag

import ragservice "local/rag-project/internal/app/rag/service"

type agentStatusEventPayload struct {
	Type            string                                      `json:"type"`
	Outcome         *ragservice.RagChatAgentOutcomePayload      `json:"outcome,omitempty"`
	ApprovalPending *ragservice.RagChatApprovalPendingPayload   `json:"approvalPending,omitempty"`
	ServiceError    *ragservice.RagChatAgentServiceErrorPayload `json:"serviceError,omitempty"`
}

func newAgentOutcomeStatusEventPayload(outcome ragservice.RagChatAgentOutcomePayload) agentStatusEventPayload {
	return agentStatusEventPayload{
		Type:    "outcome",
		Outcome: &outcome,
	}
}

func newAgentApprovalStatusEventPayload(approval ragservice.RagChatApprovalPendingPayload) agentStatusEventPayload {
	return agentStatusEventPayload{
		Type:            "approval_pending",
		ApprovalPending: &approval,
	}
}

func newAgentServiceErrorStatusEventPayload(serviceError ragservice.RagChatAgentServiceErrorPayload) agentStatusEventPayload {
	return agentStatusEventPayload{
		Type:         "service_error",
		ServiceError: &serviceError,
	}
}
