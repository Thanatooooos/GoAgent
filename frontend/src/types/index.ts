export type Role = "user" | "assistant";

export type FeedbackValue = "like" | "dislike" | null;

export type MessageStatus = "streaming" | "awaiting_approval" | "done" | "cancelled" | "error";

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
  memoryEvents?: MemoryStoredPayload[];
  sessionRecallEvents?: SessionRecallPayload[];
  fallbackReason?: string;
  agentOutcome?: AgentOutcomePayload;
  approvalPending?: ApprovalPendingPayload;
  agentServiceError?: AgentServiceErrorPayload;
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

export interface AgentOutcomePayload {
  status: string;
  interrupted: boolean;
  interruptReason?: string;
  checkpointId?: string;
}

export interface AgentThinkPayload {
  message: string;
}

export interface ApprovalPendingPayload {
  required: boolean;
  status?: string;
  reason?: string;
  reasonCode?: string;
  reasonMessage?: string;
  trigger?: string;
  node?: string;
  rerunNode?: string;
  capability?: string;
  capabilityName?: string;
  capabilityKind?: string;
  capabilityFamily?: string;
  capabilityDescription?: string;
  riskLevel?: string;
  supportsResume: boolean;
  idempotency?: string;
  checkpointId?: string;
  sessionId?: string;
  requestedAt?: string;
  resumeCount?: number;
  question?: string;
  searchQuery?: string;
  currentStepId?: string;
  currentStepTitle?: string;
  candidateUrls?: string[];
  canApprove: boolean;
  canReject: boolean;
  rejectOutcome?: string;
}

export interface ApprovalPendingLookupPayload {
  pending: boolean;
  approval?: ApprovalPendingPayload;
}

export interface AgentServiceErrorPayload {
  code?: string;
  message?: string;
  kind?: string;
  retryable: boolean;
}

export interface MemoryStoredPayload {
  conversationId: string;
  messageId: string;
  isSummarized: boolean;
  contentSummary?: string;
  rawContentLength?: number;
}

export interface SessionRecallHitPayload {
  messageId: string;
  chunkIndex: number;
  score: number;
  summary?: string;
  excerpt?: string;
  sourceChunkId?: string;
}

export interface SessionRecallPayload {
  query?: string;
  used: boolean;
  hitCount: number;
  topScore: number;
  truncatedBy?: string;
  candidateCount?: number;
  hits?: SessionRecallHitPayload[];
}
