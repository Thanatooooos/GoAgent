import { api } from "@/services/api";

export interface RagTraceRun {
  runtimeSessionId: string;
  traceId: string;
  conversationId: string;
  userMessageId: string;
  userId: string;
  status: string;
  startTime: string;
  endTime?: string;
  durationMs?: number;
  turnCount: number;
  toolCallCount: number;
  firstThinkingAt?: string;
  firstContentAt?: string;
  errorMessage?: string;
}

export interface RagTraceSpan {
  id: string;
  parentId?: string;
  kind: "model_turn" | "tool_call" | "history_compression" | string;
  name: string;
  status: string;
  startTime: string;
  endTime?: string;
  durationMs?: number;
  turn?: number;
  finishReason?: string;
  toolCallId?: string;
  toolName?: string;
  evidenceCount?: number;
  errorClass?: string;
  inputSummary?: string;
  resultSummary?: string;
  errorMessage?: string;
}

export interface RagTraceDetail {
  run: RagTraceRun;
  spans: RagTraceSpan[];
}

export interface PageResult<T> {
  records: T[];
  total: number;
  size: number;
  current: number;
  pages: number;
}

export interface RagTraceRunQuery {
  current?: number;
  size?: number;
  traceId?: string;
  conversationId?: string;
  status?: string;
}

export function getRagTraceRuns(query: RagTraceRunQuery = {}): Promise<PageResult<RagTraceRun>> {
  return api.get<PageResult<RagTraceRun>, PageResult<RagTraceRun>>("/rag/traces/runs", {
    params: {
      current: query.current ?? 1,
      size: query.size ?? 10,
      traceId: query.traceId || undefined,
      conversationId: query.conversationId || undefined,
      status: query.status || undefined
    }
  });
}

export function getRagTraceDetail(traceId: string): Promise<RagTraceDetail> {
  return api.get<RagTraceDetail, RagTraceDetail>(`/rag/traces/runs/${traceId}`);
}
