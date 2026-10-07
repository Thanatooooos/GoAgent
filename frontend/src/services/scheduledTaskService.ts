import { api } from "@/services/api";

export type ScheduleKind = "once" | "interval" | "daily" | "weekly" | "monthly";
export type TaskStatus = "active" | "paused" | "completed";

export interface TaskSchedule {
  kind: ScheduleKind;
  timezone: string;
  at?: string;
  everySeconds?: number;
  localTime?: string;
  weekday?: number;
  monthDay?: number;
}

export interface TaskConfig {
  name?: string;
  prompt: string;
  schedule: TaskSchedule;
  reportMode: "always" | "on_condition";
  conditionKind: "none" | "event" | "state";
  knowledgeBaseIds: string[];
  allowedWebDomains: string[];
  allowedToolIds: string[];
}

export interface ScheduledTask {
  id: string;
  status: TaskStatus;
  currentVersion: number;
  conversationId?: string;
  confirmedAt: string;
  nextDueAt?: string;
  lastReportedAt?: string;
}

export interface TaskVersion extends TaskConfig {
  taskId: string;
  number: number;
  confirmedAt: string;
}

export interface TaskDetail { task: ScheduledTask; version: TaskVersion }
export interface TaskDraft { id: string; taskId?: string; duplicateTaskId?: string; baseVersion?: number; config: TaskConfig; expiresAt: string }
export interface TaskRun { id: string; version: number; scheduledAt: string; status: string; resultSignal?: string; publishedMessageId?: string }
export interface TaskRunDetail { run: TaskRun; version: TaskVersion; outcome?: { signal: string; body?: string; reason?: string; sources?: string[] }; attempts: { id: string; status: string; runtimeSessionId?: string; errorMessage?: string; modelOutput?: string; startedAt: string; finishedAt?: string; tools: { sequence: number; eventType: string; toolName: string; toolState: string; detail: string; evidence: unknown }[] }[] }
export interface ConversationUnread { conversationId: string; unreadCount: number }

function normalizeConfig<T extends TaskConfig>(config: T): T {
  return { ...config, knowledgeBaseIds: config.knowledgeBaseIds || [], allowedWebDomains: config.allowedWebDomains || [], allowedToolIds: config.allowedToolIds || [] };
}
const normalizeDetail = (detail: TaskDetail): TaskDetail => ({ ...detail, version: normalizeConfig(detail.version) });
// Drafts also arrive as structured tool results in chat, where a Go nil slice
// is serialized as null rather than [].
export const normalizeTaskDraft = (draft: TaskDraft): TaskDraft => ({ ...draft, config: normalizeConfig(draft.config) });

export const getScheduledTask = (id: string) => (api.get<TaskDetail>(`/scheduled-tasks/${id}`) as unknown as Promise<TaskDetail>).then(normalizeDetail);

export const listScheduledTasks = () => (api.get<TaskDetail[]>("/scheduled-tasks") as unknown as Promise<TaskDetail[]>).then((items) => items.map(normalizeDetail));
export const listTaskRuns = (id: string) => (api.get<TaskRun[]>(`/scheduled-tasks/${id}/runs`) as unknown as Promise<TaskRun[]>).then((runs) => runs || []);
export const getTaskRun = (id: string, runId: string) => api.get<TaskRunDetail>(`/scheduled-tasks/${id}/runs/${runId}`) as unknown as Promise<TaskRunDetail>;
export const getLatestTaskReport = (id: string) => api.get<TaskRunDetail | null>(`/scheduled-tasks/${id}/latest-report`) as unknown as Promise<TaskRunDetail | null>;
export const createTaskDraft = (input: { taskId?: string; baseVersion?: number; originConversationId?: string; config: TaskConfig }) =>
  (api.post<TaskDraft>("/scheduled-tasks/drafts", input) as unknown as Promise<TaskDraft>).then(normalizeTaskDraft);
// Pending drafts of one conversation, used to restore the confirmation cards
// after a reload, which drops the tool events that carried them.
export const listPendingTaskDrafts = (conversationId: string) =>
  (api.get<TaskDraft[]>(`/scheduled-tasks/conversations/${conversationId}/drafts`) as unknown as Promise<TaskDraft[]>).then(
    (items) => (items || []).map(normalizeTaskDraft)
  );
export const confirmTaskDraft = (id: string, allowDuplicate = false) =>
  (api.post<TaskDetail>(`/scheduled-tasks/drafts/${id}/confirm`, { allowDuplicate }) as unknown as Promise<TaskDetail>).then(normalizeDetail);
export const pauseTask = (id: string) => api.post<void>(`/scheduled-tasks/${id}/pause`);
export const resumeTask = (id: string) => api.post<void>(`/scheduled-tasks/${id}/resume`);
export const deleteTask = (id: string) => api.delete<void>(`/scheduled-tasks/${id}`);
export const listScheduledUnread = () => (api.get<ConversationUnread[]>("/scheduled-tasks/unread") as unknown as Promise<ConversationUnread[]>).then((items) => items || []);
export const markScheduledConversationRead = (id: string) => api.post<void>(`/scheduled-tasks/conversations/${id}/read`);
