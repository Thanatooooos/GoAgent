export const PAGE_SIZE = 10;

export type BadgeVariant = "default" | "secondary" | "destructive" | "outline";

export const normalizeStatus = (status?: string | null): string => (status || "").trim().toLowerCase();

export const statusLabel = (status?: string | null): string => {
  const normalized = normalizeStatus(status);
  const labels: Record<string, string> = {
    completed: "已完成",
    degraded: "降级完成",
    failed: "失败",
    cancelled: "已取消",
    running: "运行中",
    pending: "等待中",
    executing: "执行中",
    denied: "已拒绝"
  };
  return labels[normalized] || normalized || "未知";
};

export const statusBadgeVariant = (status?: string | null): BadgeVariant => {
  const normalized = normalizeStatus(status);
  if (["failed", "denied", "cancelled"].includes(normalized)) return "destructive";
  if (["running", "pending", "executing"].includes(normalized)) return "secondary";
  if (["completed", "degraded"].includes(normalized)) return "default";
  return "outline";
};

export const formatDateTime = (value?: string | null): string => {
  if (!value) return "-";
  const timestamp = new Date(value).getTime();
  return Number.isNaN(timestamp) ? "-" : new Date(timestamp).toLocaleString("zh-CN");
};

export const formatDuration = (value?: number | null): string => {
  if (value === null || value === undefined || Number.isNaN(value)) return "-";
  if (value < 1000) return `${Math.round(value)}ms`;
  if (value < 60_000) return `${(value / 1000).toFixed(2)}s`;
  return `${Math.floor(value / 60_000)}m ${((value % 60_000) / 1000).toFixed(1)}s`;
};
