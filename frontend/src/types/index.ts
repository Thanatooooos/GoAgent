export type Role = "user" | "assistant";

export type FeedbackValue = "like" | "dislike" | null;

export type MessageStatus = "streaming" | "done" | "cancelled" | "error";

export interface User {
  userId: string;
  username?: string;
  role: string;
  token: string;
  avatar?: string;
}

export type CurrentUser = Omit<User, "token">;

export interface Session {
  id: string;
  title: string;
  lastTime?: string;
}

export interface Message {
  id: string;
  role: Role;
  content: string;
  thinking?: string;
  thinkingDuration?: number;
  isDeepThinking?: boolean;
  isThinking?: boolean;
  createdAt?: string;
  feedback?: FeedbackValue;
  status?: MessageStatus;
  toolCalls?: ToolCallPayload[];
  agentThinks?: string[];
  fallbackReason?: string;
}

export interface StreamMetaPayload {
  conversationId: string;
  taskId: string;
}

export interface MessageDeltaPayload {
  type: string;
  delta: string;
}

export interface ToolCallPayload {
  callId?: string;
  round?: number;
  sequence?: number;
  name: string;
  status: string;
  summary?: string;
  durationMs?: number;
  arguments?: Record<string, unknown>;
  data?: Record<string, unknown>;
}

export interface FallbackPayload {
  reason: string;
}

export interface CompletionPayload {
  messageId?: string | null;
  title?: string | null;
}

export interface AgentThinkPayload {
  message: string;
}
