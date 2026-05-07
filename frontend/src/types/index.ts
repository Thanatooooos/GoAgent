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
  retrievalMode?: string;
  retrievalModeLabel?: string;
  toolCalls?: ToolCallPayload[];
}

export interface StreamMetaPayload {
  conversationId: string;
  taskId: string;
  searchMode?: string;
}

export interface MessageDeltaPayload {
  type: string;
  delta: string;
}

export interface ToolCallPayload {
  name: string;
  status: string;
  summary: string;
}

export interface CompletionPayload {
  messageId?: string | null;
  title?: string | null;
}
