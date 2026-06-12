import type {
  AgentServiceErrorPayload,
  ApprovalPendingPayload,
  FeedbackValue,
  Message,
  Session,
  ToolCallPayload
} from "@/types";

const ACTIVE_SESSION_STORAGE_KEY = "chat.activeSessionId";

export interface PersistedChatMessage {
  id: number | string;
  role: string;
  content: string;
  thinkingContent?: string | null;
  thinkingDuration?: number | null;
  vote: number | null;
  createTime?: string;
}

function readLooseToolCallField<T = unknown>(
  payload: Record<string, unknown>,
  camel: string,
  pascal: string
): T | undefined {
  const camelValue = payload[camel];
  if (camelValue !== undefined) {
    return camelValue as T;
  }
  const pascalValue = payload[pascal];
  if (pascalValue !== undefined) {
    return pascalValue as T;
  }
  return undefined;
}

export function normalizeToolCallPayload(
  call: ToolCallPayload | Record<string, unknown>
): ToolCallPayload {
  const payload = call as Record<string, unknown>;
  return {
    callId: (readLooseToolCallField<string>(payload, "callId", "CallID") || "").trim() || undefined,
    round: readLooseToolCallField<number>(payload, "round", "Round"),
    sequence: readLooseToolCallField<number>(payload, "sequence", "Sequence"),
    name: (readLooseToolCallField<string>(payload, "name", "Name") || "").trim(),
    status: (readLooseToolCallField<string>(payload, "status", "Status") || "").trim(),
    summary: (readLooseToolCallField<string>(payload, "summary", "Summary") || "").trim() || undefined,
    durationMs: readLooseToolCallField<number>(payload, "durationMs", "DurationMs"),
    arguments: readLooseToolCallField<Record<string, unknown>>(payload, "arguments", "Arguments"),
    data: readLooseToolCallField<Record<string, unknown>>(payload, "data", "Data")
  };
}

export function mapVoteToFeedback(vote?: number | null): FeedbackValue {
  if (vote === 1) return "like";
  if (vote === -1) return "dislike";
  return null;
}

export function upsertSession(sessions: Session[], next: Session) {
  const index = sessions.findIndex((session) => session.id === next.id);
  const updated = [...sessions];
  if (index >= 0) {
    updated[index] = { ...sessions[index], ...next };
  } else {
    updated.unshift(next);
  }
  return updated.sort((a, b) => {
    const timeA = a.lastTime ? new Date(a.lastTime).getTime() : 0;
    const timeB = b.lastTime ? new Date(b.lastTime).getTime() : 0;
    return timeB - timeA;
  });
}

export function computeThinkingDuration(startAt?: number | null) {
  if (!startAt) return undefined;
  const seconds = Math.round((Date.now() - startAt) / 1000);
  return Math.max(1, seconds);
}

export function readActiveSessionId() {
  if (typeof window === "undefined") return null;
  const value = window.localStorage.getItem(ACTIVE_SESSION_STORAGE_KEY);
  return value?.trim() || null;
}

export function writeActiveSessionId(sessionId?: string | null) {
  if (typeof window === "undefined") return;
  const value = sessionId?.trim();
  if (!value) {
    window.localStorage.removeItem(ACTIVE_SESSION_STORAGE_KEY);
    return;
  }
  window.localStorage.setItem(ACTIVE_SESSION_STORAGE_KEY, value);
}

export function logChatDebug(event: string, payload: Record<string, unknown>) {
  console.info(`[chat-debug] ${event}`, payload);
}

export function mapPersistedChatMessage(item: PersistedChatMessage): Message {
  return {
    id: String(item.id),
    role: item.role === "assistant" ? "assistant" : "user",
    content: item.content,
    thinking: item.thinkingContent || undefined,
    thinkingDuration: item.thinkingDuration || undefined,
    isDeepThinking: Boolean(item.thinkingContent),
    createdAt: item.createTime,
    feedback: mapVoteToFeedback(item.vote),
    status: "done"
  };
}

function approvalMessageId(approval: ApprovalPendingPayload) {
  const checkpointId = approval.checkpointId?.trim();
  if (checkpointId) {
    return `approval-${checkpointId}`;
  }
  const requestedAt = approval.requestedAt?.trim();
  if (requestedAt) {
    return `approval-${requestedAt}`;
  }
  return `approval-${Date.now()}`;
}

export function buildPendingApprovalMessage(approval: ApprovalPendingPayload): Message {
  return {
    id: approvalMessageId(approval),
    role: "assistant",
    content: "",
    createdAt: approval.requestedAt || new Date().toISOString(),
    status: "awaiting_approval",
    feedback: null,
    approvalPending: approval
  };
}

export function mergePendingApprovalMessage(
  messages: Message[],
  approval: ApprovalPendingPayload
): Message[] {
  const checkpointId = approval.checkpointId?.trim();
  const existingIndex = messages.findIndex((message) => {
    const existingCheckpointId = message.approvalPending?.checkpointId?.trim();
    return Boolean(
      (checkpointId && existingCheckpointId === checkpointId) ||
        message.id === approvalMessageId(approval)
    );
  });

  if (existingIndex >= 0) {
    return messages.map((message, index) =>
      index === existingIndex
        ? {
            ...message,
            status: "awaiting_approval",
            approvalPending: approval,
            agentServiceError: undefined
          }
        : message
    );
  }

  return [...messages, buildPendingApprovalMessage(approval)];
}

export function applyPendingApprovalToStreamingMessage(
  messages: Message[],
  streamingMessageId: string | null,
  approval: ApprovalPendingPayload,
  thinkingStartAt?: number | null
): Message[] {
  if (!streamingMessageId) {
    return mergePendingApprovalMessage(messages, approval);
  }

  let updated = false;
  const nextMessages = messages.map((message) => {
    if (message.id !== streamingMessageId) {
      return message;
    }
    updated = true;
    return {
      ...message,
      status: "awaiting_approval",
      isThinking: false,
      thinkingDuration: message.thinkingDuration ?? computeThinkingDuration(thinkingStartAt),
      approvalPending: approval,
      agentServiceError: undefined
    };
  });

  return updated ? nextMessages : mergePendingApprovalMessage(messages, approval);
}

export function applyAgentServiceErrorToStreamingMessage(
  messages: Message[],
  streamingMessageId: string | null,
  serviceError: AgentServiceErrorPayload,
  thinkingStartAt?: number | null
): Message[] {
  if (!streamingMessageId) {
    return messages;
  }

  return messages.map((message) =>
    message.id === streamingMessageId
      ? {
          ...message,
          status: "error",
          isThinking: false,
          thinkingDuration: message.thinkingDuration ?? computeThinkingDuration(thinkingStartAt),
          agentServiceError: serviceError
        }
      : message
  );
}
