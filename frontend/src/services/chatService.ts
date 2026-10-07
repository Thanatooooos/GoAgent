import { api } from "@/services/api";
import type { ApprovalPendingLookupPayload, ChunkDetail, ImageEvidenceDetail } from "@/types";

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

export async function getChunkDetail(chunkId: string) {
  return api.get<ChunkDetail, ChunkDetail>(`/knowledge-base/chunks/${encodeURIComponent(chunkId)}`);
}

export async function getImageEvidenceDetail(evidenceId: string) {
  return api.get<ImageEvidenceDetail, ImageEvidenceDetail>(`/knowledge-base/images/${encodeURIComponent(evidenceId)}`);
}

export async function getImageEvidenceOriginal(evidenceId: string) {
  return api.get<Blob, Blob>(`/knowledge-base/images/${encodeURIComponent(evidenceId)}/original`, {
    responseType: "blob"
  });
}
