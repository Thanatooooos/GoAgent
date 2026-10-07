import type {
  AgentServiceErrorPayload,
  ApprovalPendingPayload,
  FeedbackValue,
  Message,
  MessageSource,
  Session,
  ToolCallPayload,
  ExecutionSegment
} from "@/types";

const ACTIVE_SESSION_STORAGE_KEY = "chat.activeSessionId";

export interface PersistedChatMessage {
  id: number | string;
  role: string;
  content: string;
  rawContent?: string | null;
  sources?: MessageSource[];
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
    originalName:
      (readLooseToolCallField<string>(payload, "originalName", "OriginalName") || "").trim() ||
      undefined,
    status: (readLooseToolCallField<string>(payload, "status", "Status") || "").trim(),
    summary: (readLooseToolCallField<string>(payload, "summary", "Summary") || "").trim() || undefined,
    durationMs: readLooseToolCallField<number>(payload, "durationMs", "DurationMs"),
    arguments: readLooseToolCallField<Record<string, unknown>>(payload, "arguments", "Arguments"),
    data: readLooseToolCallField<Record<string, unknown>>(payload, "data", "Data")
  };
}

export function appendThinkingSegment(
  segments: ExecutionSegment[] | undefined,
  delta: string
): ExecutionSegment[] {
  if (!delta) return segments ?? [];
  const next = [...(segments ?? [])];
  const last = next[next.length - 1];
  if (last?.kind === "thinking" && last.status === "working") {
    next[next.length - 1] = {
      ...last,
      content: last.content + delta,
      status: "working"
    };
  } else {
    next.push({
      id: `thinking-${Date.now()}-${next.length}`,
      kind: "thinking",
      content: delta,
      status: "working",
      startedAt: Date.now()
    });
  }
  return next;
}

export function settleThinkingSegments(segments: ExecutionSegment[] | undefined, stopped = false) {
  return (segments ?? []).map((segment) =>
    segment.kind === "thinking" && segment.status === "working"
      ? { ...segment, status: stopped ? "stopped" as const : "done" as const, durationMs: Date.now() - (segment.startedAt ?? Date.now()) }
      : segment
  );
}

export function appendAnswerSegment(segments: ExecutionSegment[] | undefined, delta: string): ExecutionSegment[] {
  const next = settleThinkingSegments(segments);
  const last = next[next.length - 1];
  if (last?.kind === "text") {
    next[next.length - 1] = { ...last, content: last.content + delta };
  } else {
    next.push({ id: `text-${next.length}`, kind: "text", content: delta });
  }
  return next;
}

export function isToolSettled(status: string) {
  return ["success", "completed", "failed", "error", "cancelled", "rejected", "interrupted"].includes(status);
}

export function upsertToolSegment(
  segments: ExecutionSegment[] | undefined,
  call: ToolCallPayload
): ExecutionSegment[] {
  const next = settleThinkingSegments(segments);
  const callId = call.callId?.trim();
  let index = callId
    ? next.findIndex((segment) => segment.kind === "tool_call" && segment.tool.callId === callId)
    : -1;
  // Legacy events without IDs can update the latest unfinished call of the
  // same name. Concurrent calls with IDs always use their exact identity.
  if (!callId) {
    for (let candidate = next.length - 1; candidate >= 0; candidate -= 1) {
      const segment = next[candidate];
      if (segment.kind === "tool_call" && !segment.tool.callId && segment.tool.name === call.name && !isToolSettled(segment.tool.status)) {
        index = candidate;
        break;
      }
    }
  }
  if (index >= 0) {
    const current = next[index];
    if (current.kind === "tool_call") {
      call = { ...current.tool, ...call, name: call.name || current.tool.name, arguments: call.arguments ?? current.tool.arguments, data: call.data ?? current.tool.data };
      next[index] = { ...current, tool: call };
    }
  } else {
    next.push({ id: `tool-${callId || next.length}`, kind: "tool_call", tool: call });
  }
  if (isToolSettled(call.status)) {
    const resultIndex = callId
      ? next.findIndex((segment) => segment.kind === "tool_result" && segment.tool.callId === callId)
      : -1;
    if (resultIndex >= 0) {
      next[resultIndex] = { ...next[resultIndex], kind: "tool_result", tool: call };
    } else {
      next.push({ id: `result-${callId || next.length}`, kind: "tool_result", tool: call });
    }
  }
  return next;
}

// Render streamed narration in place and the final prose once. A corrected
// finish body stays authoritative even if a replay duplicated text deltas.
export function executionPresentation(message: Message) {
  let segments = message.executionSegments;
  if (!segments?.length) {
    segments = [];
    if (message.thinking?.trim() || message.isThinking) {
      segments.push({ id: "legacy-thinking", kind: "thinking", content: message.thinking ?? "", status: message.isThinking ? "working" : "done", durationMs: (message.thinkingDuration ?? 0) * 1000 });
    }
    for (const [index, content] of (message.agentThinks ?? []).entries()) {
      segments.push({ id: `legacy-agent-${index}`, kind: "thinking", content, status: "done" });
    }
    for (const [index, tool] of (message.toolCalls ?? []).entries()) {
      segments.push({ id: `legacy-tool-${index}`, kind: "tool_call", tool });
      if (isToolSettled(tool.status)) segments.push({ id: `legacy-result-${index}`, kind: "tool_result", tool });
    }
  }
  const last = segments[segments.length - 1];
  const execution = last?.kind === "text" ? segments.slice(0, -1) : segments;
  const prefix = execution.filter((segment) => segment.kind === "text").map((segment) => segment.content).join("");
  if (!message.content.startsWith(prefix)) {
    return { segments: execution.filter((segment) => segment.kind !== "text"), answer: message.content };
  }
  return { segments: execution, answer: message.content.slice(prefix.length) };
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
    content: item.rawContent?.trim() ? item.rawContent : item.content,
    sources: item.sources,
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
      executionSegments: settleThinkingSegments(message.executionSegments),
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
          executionSegments: settleThinkingSegments(message.executionSegments, true),
          thinkingDuration: message.thinkingDuration ?? computeThinkingDuration(thinkingStartAt),
          agentServiceError: serviceError
        }
      : message
  );
}
