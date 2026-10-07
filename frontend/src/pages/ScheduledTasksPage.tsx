import * as React from "react";
import { Link, useSearchParams } from "react-router-dom";
import { getKnowledgeBases, type KnowledgeBase } from "@/services/knowledgeService";
import { describeTaskSchedule as scheduleLabel, taskDisplayName } from "@/lib/scheduledTaskPreview";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/components/ui/dialog";
import { toast } from "sonner";

import { MainLayout } from "@/components/layout/MainLayout";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import {
  confirmTaskDraft, createTaskDraft, deleteTask, listScheduledTasks, listTaskRuns, getTaskRun, getLatestTaskReport,
  pauseTask, resumeTask, type ScheduleKind, type TaskConfig, type TaskDetail,
  type TaskDraft, type TaskRun, type TaskRunDetail
} from "@/services/scheduledTaskService";

const browserTimezone = Intl.DateTimeFormat().resolvedOptions().timeZone || "Asia/Shanghai";

function localInput(iso?: string): string {
  if (!iso) return "";
  const date = new Date(iso);
  const offset = date.getTimezoneOffset() * 60000;
  return new Date(date.getTime() - offset).toISOString().slice(0, 16);
}

function emptyConfig(): TaskConfig {
  return {
    name: "", prompt: "", schedule: { kind: "daily", timezone: browserTimezone, localTime: "09:00" },
    reportMode: "always", conditionKind: "none", knowledgeBaseIds: [],
    allowedWebDomains: [], allowedToolIds: ["web_search", "web_fetch"]
  };
}

function fmt(iso?: string): string {
  if (!iso || iso.startsWith("0001-")) return "—";
  return new Date(iso).toLocaleString("zh-CN");
}

export function ScheduledTasksPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [knowledgeBases, setKnowledgeBases] = React.useState<KnowledgeBase[]>([]);
  const [items, setItems] = React.useState<TaskDetail[]>([]);
  const [selected, setSelected] = React.useState<string | null>(null);
  const [runs, setRuns] = React.useState<TaskRun[]>([]);
  const [runDetail, setRunDetail] = React.useState<TaskRunDetail | null>(null);
  const [latestReport, setLatestReport] = React.useState<TaskRunDetail | null>(null);
  const [loadingDetails, setLoadingDetails] = React.useState(false);
  const [detailsError, setDetailsError] = React.useState("");
  const detailRequest = React.useRef(0);
  const [config, setConfig] = React.useState<TaskConfig>(emptyConfig);
  const [editing, setEditing] = React.useState<TaskDetail | null>(null);
  const [draft, setDraft] = React.useState<TaskDraft | null>(null);
  const [allowDuplicate, setAllowDuplicate] = React.useState(false);
  const [busy, setBusy] = React.useState(false);
  const [atInput, setAtInput] = React.useState("");
  const [domains, setDomains] = React.useState("");
  const [editorOpen, setEditorOpen] = React.useState(false);
  const [search, setSearch] = React.useState("");

  const refresh = React.useCallback(async () => setItems(await listScheduledTasks()), []);
  React.useEffect(() => { refresh().catch((err) => toast.error(err.message)); }, [refresh]);
  React.useEffect(() => { getKnowledgeBases().then(setKnowledgeBases).catch((err) => toast.error(err.message)); }, []);
  React.useEffect(() => {
    const id = searchParams.get("taskId");
    setSelected(id);
  }, [searchParams, items]);
  React.useEffect(() => {
    let cancelled = false;
    detailRequest.current++;
    setRunDetail(null); setLatestReport(null); setRuns([]); setDetailsError("");
    if (!selected) { setLoadingDetails(false); return; }
    setLoadingDetails(true);
    Promise.all([listTaskRuns(selected), getLatestTaskReport(selected)]).then(([history, report]) => {
      if (!cancelled) { setRuns(history); setLatestReport(report); }
    }).catch((err) => { if (!cancelled) setDetailsError(err.message); })
      .finally(() => { if (!cancelled) setLoadingDetails(false); });
    return () => { cancelled = true; };
  }, [selected, items]);

  function openTask(id: string | null) {
    setSearchParams(id ? { taskId: id } : {});
  }

  async function openRun(run: TaskRun) {
    if (!selected) return;
    const request = ++detailRequest.current;
    setRunDetail(null);
    try {
      const detail = await getTaskRun(selected, run.id);
      if (request === detailRequest.current) setRunDetail(detail);
    } catch (err) { if (request === detailRequest.current) toast.error((err as Error).message); }
  }

  function beginEdit(item: TaskDetail) {
    setEditorOpen(true);
    setEditing(item);
    setConfig({
      name: item.version.name || "", prompt: item.version.prompt, schedule: { ...item.version.schedule },
      reportMode: item.version.reportMode, conditionKind: item.version.conditionKind,
      knowledgeBaseIds: item.version.knowledgeBaseIds || [],
      allowedWebDomains: item.version.allowedWebDomains || [],
      allowedToolIds: item.version.allowedToolIds || []
    });
    setAtInput(localInput(item.version.schedule.at));
    setDomains((item.version.allowedWebDomains || []).join(", "));
    setDraft(null);
    setAllowDuplicate(false);
  }

  function beginCreate() {
    setEditorOpen(true);
    setEditing(null); setConfig(emptyConfig()); setAtInput(""); setDomains(""); setDraft(null); setAllowDuplicate(false);
  }

  function updateSchedule(patch: Partial<TaskConfig["schedule"]>) {
    setConfig((old) => ({ ...old, schedule: { ...old.schedule, ...patch } }));
    setDraft(null);
  }

  async function preview() {
    setBusy(true);
    try {
      const schedule = { ...config.schedule };
      if (schedule.kind === "once" || schedule.kind === "interval") {
        if (!atInput || Number.isNaN(new Date(atInput).getTime())) throw new Error("请选择首次执行时间");
        schedule.at = new Date(atInput).toISOString();
      }
      const normalized: TaskConfig = {
        ...config, schedule,
        allowedWebDomains: domains.split(",").map((part) => part.trim()).filter(Boolean),
        conditionKind: config.reportMode === "always" ? "none" : config.conditionKind
      };
      if (normalized.allowedWebDomains.length && !normalized.allowedToolIds.some((tool) => tool === "web_search" || tool === "web_fetch")) {
        normalized.allowedToolIds = [...normalized.allowedToolIds, "web_search", "web_fetch"];
      }
      const result = await createTaskDraft({ taskId: editing?.task.id,
        baseVersion: editing?.task.currentVersion, config: normalized });
      setDraft(result);
      setAllowDuplicate(false);
      toast.success("预览已生成，请核对后确认");
    } catch (err) { toast.error((err as Error).message); }
    finally { setBusy(false); }
  }

  async function confirm() {
    if (!draft) return;
    setBusy(true);
    try {
      const result = await confirmTaskDraft(draft.id, allowDuplicate);
      setDraft(null); setEditorOpen(false); openTask(result.task.id); await refresh();
      toast.success("定时任务已确认");
    } catch (err) { toast.error((err as Error).message); }
    finally { setBusy(false); }
  }

  async function mutate(id: string, action: "pause" | "resume" | "delete") {
    if (action === "delete" && !window.confirm("永久删除这个定时任务？原有汇报会话会保留。")) return;
    setBusy(true);
    try {
      if (action === "pause") await pauseTask(id);
      if (action === "resume") await resumeTask(id);
      if (action === "delete") await deleteTask(id);
      if (action === "delete" && selected === id) openTask(null);
      await refresh();
    } catch (err) { toast.error((err as Error).message); }
    finally { setBusy(false); }
  }

  const field = "w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm text-slate-800";
  const button = "rounded-xl border border-slate-200 px-3 py-2 text-sm hover:bg-slate-50 disabled:opacity-50";
  const statusLabel = { active: "已安排", paused: "已暂停", completed: "已完成" };
  const visibleItems = items.filter((item) => `${taskDisplayName(item.version)} ${item.version.prompt}`.toLowerCase().includes(search.toLowerCase()));
  const selectedItem = items.find((item) => item.task.id === selected);
  const runLabel = (run: TaskRun) => ({ pending: "等待运行", running: "运行中", reported: "已汇报", report: "已汇报", no_report: "无新消息", uncertain: "结果不确定", failed: "运行失败", missed: "已错过", superseded: "已被新配置替代", cancelled: "已取消", completed: "已完成" }[run.status] || ({ report: "已汇报", no_report: "无新消息", uncertain: "结果不确定" }[run.resultSignal || ""]) || run.status);

  return <MainLayout variant="brief">
    <div className="h-full overflow-y-auto bg-white px-5 py-9">
      <div className={`mx-auto ${selected ? "max-w-7xl" : "max-w-5xl"}`}>
        <div className={selected ? "grid gap-8 lg:grid-cols-[320px_minmax(0,1fr)]" : "space-y-6"}>
        <div className="space-y-6">
        <header className="flex flex-wrap items-center justify-between gap-4"><h1 className="text-2xl font-semibold text-slate-900">定时任务</h1>
          <div className="flex gap-3"><input aria-label="搜索任务" placeholder="搜索任务" value={search} onChange={(event) => setSearch(event.target.value)} className="w-44 rounded-full border px-4 py-2 text-sm" />
            <button className="rounded-full bg-slate-900 px-4 py-2 text-sm text-white" onClick={beginCreate}>＋ 添加任务</button></div></header>
        <section>
          {items.length === 0 ? <p className="py-12 text-sm text-slate-500">暂无定时任务，点击添加或在聊天中告诉我需要做什么。</p> :
            <div className="overflow-x-auto"><table className="w-full text-left text-sm"><thead className={`${selected ? "hidden" : ""} text-xs font-normal text-slate-500`}><tr className="border-b"><th className="py-4 font-normal">任务</th><th className="py-4 font-normal">下次运行</th><th className="py-4 font-normal">状态</th><th className="py-4 font-normal"><span className="sr-only">操作</span></th></tr></thead>
              <tbody>{visibleItems.map((item) => <tr key={item.task.id} className="group border-b border-slate-100 hover:bg-slate-50">
                <td className="min-w-52 py-5 pr-5"><button className={`text-left font-medium hover:underline ${selected === item.task.id ? "text-blue-700" : "text-slate-900"}`} onClick={() => openTask(item.task.id)}>{taskDisplayName(item.version)}</button>
                  <p className="mt-1 text-xs text-slate-500">{scheduleLabel(item.version)}</p></td>
                <td className={`${selected ? "hidden" : ""} whitespace-nowrap pr-5 text-slate-600`}>{item.task.status === "active" ? fmt(item.task.nextDueAt) : "—"}</td>
                <td className={`${selected ? "hidden" : ""} whitespace-nowrap pr-5 text-slate-500`}>{statusLabel[item.task.status]}</td>
                <td className={selected ? "hidden" : "whitespace-nowrap text-right"}>
                  <button className="px-2 py-1 text-blue-600 hover:text-blue-800 disabled:opacity-50" disabled={busy} onClick={() => mutate(item.task.id, item.task.status === "active" ? "pause" : "resume")}>{item.task.status === "active" ? "暂停" : item.task.status === "paused" ? "恢复运行" : "重新启用"}</button>
                  <button className="px-2 py-1 text-slate-600 hover:text-slate-900" onClick={() => beginEdit(item)} aria-label={`编辑 ${taskDisplayName(item.version)}`}>编辑</button>
                  <details className="relative inline-block text-left"><summary className="cursor-pointer list-none px-2 py-1" aria-label={`更多操作 ${taskDisplayName(item.version)}`}>···</summary>
                    <div className="absolute right-0 z-20 w-36 rounded-xl border bg-white p-1 shadow-lg">
                      <button className="block w-full rounded-lg px-3 py-2 text-left hover:bg-slate-50" onClick={() => openTask(item.task.id)}>查看详情</button>
                      {item.task.conversationId && <Link className="block rounded-lg px-3 py-2 hover:bg-slate-50" to={`/chat/${item.task.conversationId}`}>打开汇报会话</Link>}
                      <button className="block w-full rounded-lg px-3 py-2 text-left text-red-600 hover:bg-slate-50" disabled={busy} onClick={() => mutate(item.task.id, "delete")}>删除</button>
                    </div></details></td>
              </tr>)}</tbody></table>{!visibleItems.length && <p className="py-8 text-sm text-slate-500">没有匹配的任务</p>}</div>}
        </section>
        </div>
        {selected && <section className="min-w-0 border-t pt-5 lg:border-l lg:border-t-0 lg:pl-8 lg:pt-0">
          <header className="flex items-start justify-between gap-3 border-b pb-5"><div><h2 className="text-xl font-semibold">{selectedItem ? taskDisplayName(selectedItem.version) : "任务详情"}</h2>
            {selectedItem && <p className="mt-1 text-xs text-slate-500">{scheduleLabel(selectedItem.version)} · {statusLabel[selectedItem.task.status]}</p>}</div>
            <div className="flex shrink-0 flex-wrap gap-2">{selectedItem && <><button className={`${button} text-blue-600`} disabled={busy} onClick={() => mutate(selectedItem.task.id, selectedItem.task.status === "active" ? "pause" : "resume")}>{selectedItem.task.status === "active" ? "暂停" : selectedItem.task.status === "paused" ? "恢复运行" : "重新启用"}</button><button className={button} onClick={() => beginEdit(selectedItem)}>编辑</button></>}<button className={button} onClick={() => openTask(null)} aria-label="关闭任务详情">关闭</button></div></header>
          {loadingDetails ? <p className="py-8 text-sm text-slate-500">正在加载任务详情…</p> : detailsError ? <p role="alert" className="py-8 text-sm text-red-700">无法加载任务详情：{detailsError}</p> : <>
          <div className="py-6"><h3 className="mb-4 text-sm text-slate-500">最新汇报</h3>
            {latestReport ? <><p className="mb-3 text-xs text-slate-500">{fmt(latestReport.run.scheduledAt)}{latestReport.run.version !== selectedItem?.task.currentVersion && " · 来自此前配置"}</p>
              <MarkdownRenderer content={latestReport.outcome?.body || ""} />
              {!!latestReport.outcome?.sources?.length && <div className="mt-4 flex flex-wrap gap-3 text-sm">{latestReport.outcome.sources.map((source, index) => /^https?:\/\//i.test(source) ? <a key={index} href={source} target="_blank" rel="noopener noreferrer" className="break-all text-blue-600 underline">来源 {index + 1}</a> : <span key={index} className="break-all text-slate-500">{source}</span>)}</div>}
            </> : <p className="text-sm text-slate-500">尚无汇报。任务运行后，有需要汇报的结果会显示在这里。</p>}
            {selectedItem?.task.conversationId && <Link className="mt-4 inline-block text-sm text-blue-600" to={`/chat/${selectedItem.task.conversationId}`}>打开汇报会话 ↗</Link>}
          </div>
          <div className="border-t pt-6"><h3 className="font-medium">历史运行记录</h3>
          <p className="mt-1 text-xs text-slate-500">无新消息是模型结论，不代表系统核实了所有资料。显示最近 100 次运行。</p>
          <div className="mt-3 space-y-2">{runs.length ? runs.map((run) => <button key={run.id} className="flex w-full justify-between gap-3 border-b py-3 text-left text-sm hover:bg-slate-50" onClick={() => openRun(run)}>
            <span>{fmt(run.scheduledAt)}</span><span>{runLabel(run)} · 查看详情</span>
          </button>) : <p className="text-sm text-slate-500">尚无运行记录</p>}</div>
          {runDetail && <div className="mt-4 space-y-3 rounded-xl bg-slate-50 p-4 text-sm">
            <p>计划时间：{fmt(runDetail.run.scheduledAt)} · 配置版本 {runDetail.run.version}</p>
            <details><summary>本次运行使用的确认配置</summary><div className="mt-2 space-y-1 break-words">
              <p className="whitespace-pre-wrap">{runDetail.version.prompt}</p>
              <p>{scheduleLabel(runDetail.version)} · {runDetail.version.schedule.timezone}</p>
              <p>确认基线：{fmt(runDetail.version.confirmedAt)} · {runDetail.version.reportMode} / {runDetail.version.conditionKind}</p>
              <p>网页：{runDetail.version.allowedWebDomains?.join(", ") || "公开网页"} · 知识库：{runDetail.version.knowledgeBaseIds?.join(", ") || "无"}</p>
              <p>工具：{runDetail.version.allowedToolIds?.join(", ") || "无"}</p>
            </div></details>
            {runDetail.outcome && <><p className="whitespace-pre-wrap">{runDetail.outcome.body || runDetail.outcome.reason || "本次模型没有值得汇报的结果"}</p>
              <p className="break-words">来源：{runDetail.outcome.sources?.join(", ") || "无"}</p></>}
            {runDetail.attempts.map((attempt) => <div key={attempt.id} className="border-t pt-3"><p>尝试 {attempt.id} · {attempt.status} · {fmt(attempt.startedAt)} → {fmt(attempt.finishedAt)}</p>
              {attempt.errorMessage && <p className="mt-1 whitespace-pre-wrap text-red-700">技术错误：{attempt.errorMessage}</p>}
              {attempt.modelOutput && <details className="mt-2"><summary>原始模型输出</summary><pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words text-xs">{attempt.modelOutput}</pre></details>}
              {attempt.tools.map((tool) => <details key={tool.sequence} className="mt-2"><summary>{tool.toolName} · {tool.toolState}</summary><pre className="max-h-72 overflow-auto whitespace-pre-wrap break-words text-xs">{tool.detail}</pre></details>)}
            </div>)}
          </div>}
          </div></>}
        </section>}
        </div>
        <Dialog open={editorOpen} onOpenChange={(open) => { if (!busy) setEditorOpen(open); }}>
          <DialogContent className="max-h-[88vh] max-w-2xl overflow-y-auto rounded-2xl bg-white p-6">
          <DialogHeader><DialogTitle>{editing ? "编辑任务" : "添加任务"}</DialogTitle>
            <DialogDescription>修改后先预览，确认后生效。</DialogDescription></DialogHeader>
          <div className="mt-4 grid gap-4 md:grid-cols-2">
            <label className="md:col-span-2 text-sm">任务名称<input className={`${field} mt-1`} maxLength={60} placeholder="留空时会根据任务内容自动命名" value={config.name || ""}
              onChange={(event) => { setConfig({ ...config, name: event.target.value }); setDraft(null); }} /></label>
            <label className="md:col-span-2 text-sm">任务内容<textarea className={`${field} mt-1 min-h-32`} placeholder="告诉我到时需要做什么" value={config.prompt}
              onChange={(event) => { setConfig({ ...config, prompt: event.target.value }); setDraft(null); }} /></label>
            <label className="text-sm">执行频率<select className={`${field} mt-1`} value={config.schedule.kind}
              onChange={(event) => updateSchedule({ kind: event.target.value as ScheduleKind })}>
              <option value="once">一次</option><option value="interval">固定间隔</option><option value="daily">每天</option>
              <option value="weekly">每周</option><option value="monthly">每月</option></select></label>
            {(config.schedule.kind === "once" || config.schedule.kind === "interval") &&
              <label className="text-sm">首次执行<input className={`${field} mt-1`} type="datetime-local" value={atInput}
                onChange={(event) => { setAtInput(event.target.value); setDraft(null); }} /></label>}
            {config.schedule.kind === "interval" && <label className="text-sm">间隔（分钟）<input className={`${field} mt-1`} type="number" min="1"
              value={(config.schedule.everySeconds || 3600) / 60} onChange={(event) => updateSchedule({ everySeconds: Number(event.target.value) * 60 })} /></label>}
            {["daily", "weekly", "monthly"].includes(config.schedule.kind) && <label className="text-sm">当地时间<input className={`${field} mt-1`} type="time"
              value={config.schedule.localTime || "09:00"} onChange={(event) => updateSchedule({ localTime: event.target.value })} /></label>}
            {config.schedule.kind === "weekly" && <label className="text-sm">星期<select className={`${field} mt-1`} value={config.schedule.weekday || 0}
              onChange={(event) => updateSchedule({ weekday: Number(event.target.value) })}>{["日", "一", "二", "三", "四", "五", "六"].map((day, index) =>
                <option key={index} value={index}>周{day}</option>)}</select></label>}
            {config.schedule.kind === "monthly" && <label className="text-sm">每月几日<input className={`${field} mt-1`} type="number" min="1" max="31"
              value={config.schedule.monthDay || 1} onChange={(event) => updateSchedule({ monthDay: Number(event.target.value) })} /></label>}
          </div>
          <p className="mt-3 text-xs text-slate-500">按 {config.schedule.timezone} 时间执行</p>
          <details className="mt-4"><summary className="cursor-pointer text-sm text-slate-500">高级设置</summary>
            <div className="mt-4 grid gap-4 md:grid-cols-2">
            <label className="text-sm">时区<input className={`${field} mt-1`} value={config.schedule.timezone} onChange={(event) => updateSchedule({ timezone: event.target.value })} /></label>
            <label className="text-sm">汇报规则<select className={`${field} mt-1`} value={config.reportMode}
              onChange={(event) => { const mode = event.target.value as TaskConfig["reportMode"];
                setConfig({ ...config, reportMode: mode, conditionKind: mode === "always" ? "none" : "event" }); setDraft(null); }}>
              <option value="always">每次汇报</option><option value="on_condition">条件满足时汇报</option></select></label>
            {config.reportMode === "on_condition" && <label className="text-sm">条件类型<select className={`${field} mt-1`} value={config.conditionKind}
              onChange={(event) => { setConfig({ ...config, conditionKind: event.target.value as TaskConfig["conditionKind"] }); setDraft(null); }}>
              <option value="event">新事件</option><option value="state">当前状态</option></select></label>}
            <label className="md:col-span-2 text-sm">允许访问的网站域名（逗号分隔；留空表示公开网页）<input className={`${field} mt-1`}
              value={domains} onChange={(event) => { setDomains(event.target.value); setDraft(null); }} /></label>
            <fieldset className="md:col-span-2 text-sm"><legend>允许访问的知识库</legend>
              {knowledgeBases.map((kb) => <label key={kb.id} className="mt-2 mr-4 inline-flex gap-2"><input type="checkbox"
                checked={config.knowledgeBaseIds.includes(kb.id)} onChange={(event) => {
                  setConfig({ ...config,
                    knowledgeBaseIds: event.target.checked ? [...config.knowledgeBaseIds, kb.id] : config.knowledgeBaseIds.filter((id) => id !== kb.id),
                    allowedToolIds: event.target.checked && !config.allowedToolIds.includes("retrieve_knowledge") ? [...config.allowedToolIds, "retrieve_knowledge"] : config.allowedToolIds
                  }); setDraft(null);
                }} />{kb.name}</label>)}
              {!knowledgeBases.length && <p className="mt-2 text-slate-500">暂无可选知识库</p>}
            </fieldset>
            </div>
          </details>
          <div className="mt-5 flex justify-end gap-2"><button className={button} disabled={busy} onClick={() => setEditorOpen(false)}>取消</button><button className={`${button} bg-slate-900 text-white hover:bg-slate-800`} disabled={busy} onClick={preview}>{busy ? "正在生成…" : "预览修改"}</button></div>
          {draft && <div className="mt-4 rounded-xl border border-blue-200 bg-blue-50 p-4 text-sm">
            <p className="font-semibold">{taskDisplayName(draft.config)} · 待确认</p>
            {editing && <div className="mt-2 space-y-2"><p>版本 {draft.baseVersion} → {(draft.baseVersion || 0) + 1}；以下字段发生变化：</p>
              {(["name", "prompt", "schedule", "reportMode", "conditionKind", "knowledgeBaseIds", "allowedWebDomains", "allowedToolIds"] as const)
                .filter((key) => JSON.stringify(editing.version[key]) !== JSON.stringify(draft.config[key]))
                .map((key) => <div key={key} className="break-words"><p className="font-medium">{({ name: "任务名称", prompt: "任务内容", schedule: "执行时间", reportMode: "汇报规则", conditionKind: "条件类型", knowledgeBaseIds: "知识库", allowedWebDomains: "网页范围", allowedToolIds: "查询能力" })[key]}</p>
                  <p className="whitespace-pre-wrap text-slate-500">原：{JSON.stringify(editing.version[key])}</p>
                  <p className="whitespace-pre-wrap">新：{JSON.stringify(draft.config[key])}</p></div>)}
            </div>}
            <p className="mt-2 whitespace-pre-wrap">{draft.config.prompt}</p>
            <p className="mt-2">{scheduleLabel(draft.config)} · {draft.config.schedule.timezone}</p>
            <p>汇报：{draft.config.reportMode === "always" ? "每次汇报" : "条件满足时汇报"}</p>
            <p>网页范围：{draft.config.allowedToolIds.some((tool) => tool === "web_search" || tool === "web_fetch") ? draft.config.allowedWebDomains.join(", ") || "公开网页" : "不使用网页资料"}</p>
            <p>知识库：{draft.config.knowledgeBaseIds.join(", ") || "无"}</p>
            {draft.duplicateTaskId && <label className="mt-2 flex gap-2 text-amber-800"><input type="checkbox" checked={allowDuplicate}
              onChange={(event) => setAllowDuplicate(event.target.checked)} />已有相同任务 {draft.duplicateTaskId}。我仍要另建一个。</label>}
            <button className={`${button} mt-3 bg-blue-600 text-white hover:bg-blue-700`} disabled={busy || Boolean(draft.duplicateTaskId && !allowDuplicate)} onClick={confirm}>确认创建或修改</button>
          </div>}
          </DialogContent>
        </Dialog>
      </div>
    </div>
  </MainLayout>;
}
