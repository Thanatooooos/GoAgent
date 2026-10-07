import type { TaskConfig } from "@/services/scheduledTaskService";

export function describeTaskSchedule(config: TaskConfig): string {
  const schedule = config.schedule;
  const first = schedule.at && !schedule.at.startsWith("0001-")
    ? new Intl.DateTimeFormat("zh-CN", { timeZone: schedule.timezone, dateStyle: "medium", timeStyle: "short" }).format(new Date(schedule.at)) : "—";
  switch (schedule.kind) {
    case "once": return `一次 · ${first}`;
    case "interval": return `每 ${schedule.everySeconds} 秒 · 首次 ${first}`;
    case "daily": return `每天 ${schedule.localTime}`;
    case "weekly": return `每周${["日", "一", "二", "三", "四", "五", "六"][schedule.weekday || 0]} ${schedule.localTime}`;
    case "monthly": return `每月 ${schedule.monthDay} 日 ${schedule.localTime}`;
  }
}
export function taskDisplayName(config: { name?: string; reportMode: string; conditionKind: string; schedule: { kind: string }; allowedWebDomains?: string[] }): string {
  if (config.name?.trim()) return config.name.trim();
  const label = config.reportMode === "on_condition" ? config.conditionKind === "event" ? "事件监测" : "状态监测" : config.schedule.kind === "once" ? "提醒事项" : "定期汇总";
  return config.allowedWebDomains?.length ? `${config.allowedWebDomains[0]} · ${label}` : label;
}
