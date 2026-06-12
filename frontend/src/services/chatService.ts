import { api } from "@/services/api";
import type { ApprovalPendingLookupPayload } from "@/types";

export async function stopTask(taskId: string) {
  return api.post<void>(`/rag/v3/stop?taskId=${encodeURIComponent(taskId)}`);
}

export async function getPendingApproval(conversationId: string) {
  return api.get<ApprovalPendingLookupPayload>(
    `/rag/v3/chat/approval/pending?conversationId=${encodeURIComponent(conversationId)}`
  );
}

export async function submitFeedback(messageId: string, vote: number) {
  return api.post<void>(`/conversations/messages/${messageId}/feedback`, {
    vote
  });
}
