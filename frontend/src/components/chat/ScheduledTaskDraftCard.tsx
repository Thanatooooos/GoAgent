import * as React from "react";
import { useNavigate } from "react-router-dom";
import { CalendarClock, CheckCircle2 } from "lucide-react";
import { toast } from "sonner";

import { describeTaskSchedule, taskDisplayName } from "@/lib/scheduledTaskPreview";
import { confirmTaskDraft, normalizeTaskDraft, type TaskDraft } from "@/services/scheduledTaskService";
import type { ToolCallPayload } from "@/types";

// draftsFromToolCalls collects the structured result the runtime attaches to a
// settled create_scheduled_task call. A draft only becomes a task after the
// user confirms it here.
export function draftsFromToolCalls(toolCalls: ToolCallPayload[]): TaskDraft[] {
  const drafts: TaskDraft[] = [];
  for (const call of toolCalls) {
    const candidate = (call.data as { draft?: TaskDraft } | undefined)?.draft;
    if (!candidate?.id || !candidate.config) continue;
    drafts.push(normalizeTaskDraft(candidate));
  }
  return drafts;
}

export function ScheduledTaskDraftCard({ draft }: { draft: TaskDraft }) {
  const navigate = useNavigate();
  const [confirming, setConfirming] = React.useState(false);
  const [confirmed, setConfirmed] = React.useState(false);
  const [dismissed, setDismissed] = React.useState(false);
  if (dismissed) return null;

  const name = taskDisplayName(draft.config);
  const finishLine =
    draft.config.reportMode === "on_condition"
      ? "条件满足并汇报后完成"
      : draft.config.schedule.kind === "once"
        ? "执行一次后完成"
        : "持续按计划运行，直到暂停";

  const confirm = async () => {
    setConfirming(true);
    try {
      await confirmTaskDraft(draft.id, Boolean(draft.duplicateTaskId));
      setConfirmed(true);
      toast.success("定时任务已创建，可在定时任务页面管理");
    } catch (err) {
      toast.error((err as Error).message);
    } finally {
      setConfirming(false);
    }
  };

  return (
    <div className="rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm text-slate-800">
      <div className="flex items-center gap-2">
        <CalendarClock className="h-4 w-4 text-blue-700" />
        <span className="font-semibold">{confirmed ? "定时任务已创建" : "定时任务待确认"}</span>
      </div>
      <p className="mt-2 font-medium">{name}</p>
      <p className="mt-2 whitespace-pre-wrap">{draft.config.prompt}</p>
      <p className="mt-2 text-xs">触发：{describeTaskSchedule(draft.config)} · {draft.config.schedule.timezone}</p>
      <p className="text-xs">汇报：{draft.config.reportMode === "always" ? "每次汇报" : "条件满足时汇报"}</p>
      <p className="text-xs">网页范围：{draft.config.allowedWebDomains.join(", ") || "公开网页"}</p>
      <p className="text-xs">
        知识库：{draft.config.knowledgeBaseIds.join(", ") || "无"} · {finishLine}
      </p>
      {draft.duplicateTaskId ? (
        <p className="mt-2 text-amber-800">已有相同任务 {draft.duplicateTaskId}，确认后会再创建一个。</p>
      ) : null}
      {confirmed ? (
        <div className="mt-3 flex items-center gap-3">
          <span className="flex items-center gap-1 text-xs text-emerald-700">
            <CheckCircle2 className="h-3.5 w-3.5" />
            任务将在计划时间自动运行
          </span>
          <button type="button" className="text-blue-700" onClick={() => navigate("/scheduled-tasks")}>
            打开定时任务
          </button>
        </div>
      ) : (
        <div className="mt-3 flex items-center gap-3">
          <button
            type="button"
            disabled={confirming}
            className="rounded-lg bg-blue-600 px-3 py-2 text-white disabled:opacity-50"
            onClick={confirm}
          >
            {draft.duplicateTaskId ? "仍要另建一个" : "确认创建"}
          </button>
          <button type="button" disabled={confirming} onClick={() => setDismissed(true)}>
            取消
          </button>
        </div>
      )}
    </div>
  );
}
