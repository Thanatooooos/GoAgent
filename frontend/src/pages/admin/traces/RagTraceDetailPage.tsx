import { type ComponentType, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import {
  Activity,
  AlertTriangle,
  ArrowLeft,
  Calendar,
  CheckCircle2,
  Clock,
  Hash,
  Loader2,
  RefreshCw,
  User,
  Wrench,
  XCircle,
  Zap
} from "lucide-react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { getRagTraceDetail, type RagTraceDetail, type RagTraceNode } from "@/services/ragTraceService";
import {
  clamp,
  formatDateTime,
  formatDuration,
  normalizeStatus,
  resolveNodeDuration,
  statusBadgeVariant,
  statusLabel,
  toTimestamp,
  type TimelineNode
} from "@/pages/admin/traces/traceUtils";
import { getErrorMessage } from "@/utils/error";

const decodeTraceId = (value?: string): string => {
  if (!value) return "";
  try {
    return decodeURIComponent(value);
  } catch {
    return value;
  }
};

const copyToClipboard = (text: string, label: string) => {
  navigator.clipboard
    .writeText(text)
    .then(() => toast.success(`${label} copied`))
    .catch(() => toast.error("Copy failed"));
};

type StatusType = "success" | "failed" | "running" | "default";

const STATUS_COLORS: Record<StatusType, { dot: string; bar: string }> = {
  success: { dot: "bg-emerald-500", bar: "bg-emerald-400" },
  failed: { dot: "bg-red-500", bar: "bg-red-400" },
  running: { dot: "bg-amber-500", bar: "bg-amber-400" },
  default: { dot: "bg-slate-300", bar: "bg-slate-300" }
};

const getStatusColors = (status?: string | null) => {
  const normalized = normalizeStatus(status) as StatusType | null;
  return STATUS_COLORS[normalized || "default"];
};

type TraceExtra = Record<string, unknown>;

type RetrieveDecision = {
  query: string;
  source: string;
  reason: string;
  signals: string[];
};

type RetrieveChannelStat = {
  name: string;
  chunkCount: number;
  latencyMs: number;
  error?: string;
  metadata?: Record<string, unknown>;
};

type ToolCallInsight = {
  nodeId: string;
  name: string;
  status: string;
  summary: string;
  durationMs: number;
  error: string;
};

const parseTraceExtra = (raw?: string | null): TraceExtra => {
  if (!raw?.trim()) return {};
  try {
    const parsed = JSON.parse(raw) as TraceExtra;
    return parsed && typeof parsed === "object" ? parsed : {};
  } catch {
    return {};
  }
};

const readString = (value: unknown): string => {
  if (typeof value === "string") return value.trim();
  if (value === undefined || value === null) return "";
  return String(value).trim();
};

const readNumber = (value: unknown): number => {
  if (typeof value === "number" && Number.isFinite(value)) return value;
  const next = Number(value);
  return Number.isFinite(next) ? next : 0;
};

const readBoolean = (value: unknown): boolean => value === true;

const readStringArray = (value: unknown): string[] => {
  if (!Array.isArray(value)) return [];
  return value.map((item) => readString(item)).filter(Boolean);
};

const readObjectArray = <T extends Record<string, unknown>>(value: unknown): T[] => {
  if (!Array.isArray(value)) return [];
  return value.filter((item): item is T => Boolean(item) && typeof item === "object");
};

const formatMetadata = (metadata?: Record<string, unknown>) => {
  if (!metadata || Object.keys(metadata).length === 0) return "";
  return Object.entries(metadata)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `${key}=${typeof value === "object" ? JSON.stringify(value) : String(value)}`)
    .join(", ");
};

function MetricItem({
  icon: Icon,
  label,
  value,
  variant = "default"
}: {
  icon: ComponentType<{ className?: string }>;
  label: string;
  value: string | number;
  variant?: "default" | "success" | "error" | "warning" | "primary";
}) {
  const styles = {
    default: "text-slate-600",
    success: "text-emerald-600",
    error: "text-red-600",
    warning: "text-amber-600",
    primary: "text-blue-600"
  };

  return (
    <div className="flex items-center gap-2 px-4 py-2">
      <Icon className={cn("h-4 w-4", styles[variant])} />
      <span className={cn("text-lg font-semibold", styles[variant])}>{value}</span>
      <span className="text-xs text-slate-500">{label}</span>
    </div>
  );
}

function TimeScale({ totalMs }: { totalMs: number }) {
  const ticks = [0, 25, 50, 75, 100];
  return (
    <div className="relative h-6 border-b border-slate-200">
      {ticks.map((percent) => (
        <div
          key={percent}
          className="absolute top-0 bottom-0 flex flex-col items-center"
          style={{ left: `${percent}%`, transform: "translateX(-50%)" }}
        >
          <div className="h-2 w-px bg-slate-300" />
          <span className="mt-0.5 text-[10px] text-slate-400">
            {formatDuration((totalMs * percent) / 100)}
          </span>
        </div>
      ))}
    </div>
  );
}

function WaterfallRow({
  node,
  nodeDisplayName,
  nodeStatus,
  isTopSlowest
}: {
  node: TimelineNode & {
    depthValue: number;
    resolvedDurationMs: number;
    offsetMs: number;
    leftPercent: number;
    widthPercent: number;
  };
  nodeDisplayName: string;
  nodeStatus: string | null;
  isTopSlowest?: boolean;
}) {
  const colors = getStatusColors(nodeStatus);

  return (
    <div
      className={cn(
        "group grid grid-cols-[minmax(180px,1fr)_120px_2fr_100px] gap-4 px-4 py-2.5 transition-colors",
        "hover:bg-slate-50/80",
        isTopSlowest && "bg-amber-50/40"
      )}
    >
      <div className="flex min-w-0 items-center gap-2" style={{ paddingLeft: `${Math.min(node.depthValue, 6) * 16}px` }}>
        <span className={cn("h-2 w-2 shrink-0 rounded-full transition-transform group-hover:scale-125", colors.dot)} />
        <span className="truncate text-sm text-slate-700" title={nodeDisplayName}>
          {nodeDisplayName}
        </span>
        {isTopSlowest ? <Zap className="h-3 w-3 shrink-0 text-amber-500" /> : null}
      </div>

      <div className="flex items-center">
        <span className="truncate rounded bg-slate-100 px-2 py-0.5 text-xs text-slate-500" title={node.nodeType || "-"}>
          {node.nodeType || "-"}
        </span>
      </div>

      <div className="flex items-center">
        <div className="relative h-6 w-full overflow-hidden rounded bg-slate-50">
          {[25, 50, 75].map((percent) => (
            <div
              key={percent}
              className="absolute top-0 bottom-0 w-px bg-slate-200"
              style={{ left: `${percent}%` }}
            />
          ))}
          <div
            className={cn("absolute top-1 bottom-1 rounded transition-all group-hover:brightness-110", colors.bar)}
            style={{
              left: `${node.leftPercent}%`,
              width: `${Math.max(node.widthPercent, 0.5)}%`,
              minWidth: "4px"
            }}
            title={`${nodeDisplayName} - ${formatDuration(node.resolvedDurationMs)}`}
          />
        </div>
      </div>

      <div className="text-right">
        <p className="text-sm font-medium text-slate-700">{formatDuration(node.resolvedDurationMs)}</p>
        <p className="text-[10px] text-slate-400">@{formatDuration(node.offsetMs)}</p>
      </div>
    </div>
  );
}

function SectionTitle({ title, description }: { title: string; description?: string }) {
  return (
    <div>
      <h3 className="text-sm font-semibold text-slate-900">{title}</h3>
      {description ? <p className="mt-1 text-xs text-slate-500">{description}</p> : null}
    </div>
  );
}

function findNode(nodes: RagTraceNode[], nodeId: string) {
  return nodes.find((node) => node.nodeId === nodeId);
}

export function RagTraceDetailPage() {
  const params = useParams<{ traceId: string }>();
  const traceId = decodeTraceId(params.traceId);
  const detailRequestRef = useRef(0);
  const [detail, setDetail] = useState<RagTraceDetail | null>(null);
  const [detailLoading, setDetailLoading] = useState(false);

  const loadDetail = async (nextTraceId: string) => {
    if (!nextTraceId) return;
    const requestId = ++detailRequestRef.current;
    setDetailLoading(true);
    try {
      const result = await getRagTraceDetail(nextTraceId);
      if (detailRequestRef.current !== requestId) return;
      setDetail(result);
    } catch (error) {
      if (detailRequestRef.current !== requestId) return;
      toast.error(getErrorMessage(error, "Failed to load trace detail"));
      console.error(error);
      setDetail(null);
    } finally {
      if (detailRequestRef.current !== requestId) return;
      setDetailLoading(false);
    }
  };

  useEffect(() => {
    if (!traceId) {
      detailRequestRef.current += 1;
      setDetail(null);
      setDetailLoading(false);
      return;
    }
    loadDetail(traceId);
  }, [traceId]);

  const selectedRun = detail?.run || null;

  const timeline = useMemo(() => {
    const nodes = detail?.nodes || [];
    if (!nodes.length) return { totalWindowMs: 0, nodes: [] as TimelineNode[] };

    const normalized = nodes.map((node) => {
      const startTs = toTimestamp(node.startTime);
      const endTs = toTimestamp(node.endTime);
      const resolvedDurationMs = resolveNodeDuration(node);
      const depthValue = Math.max(0, Number(node.depth ?? 0));
      const resolvedStartTs = startTs ?? 0;
      const resolvedEndTs = endTs ?? (resolvedStartTs > 0 ? resolvedStartTs + resolvedDurationMs : 0);
      return { ...node, depthValue, resolvedDurationMs, startTs: resolvedStartTs, endTs: resolvedEndTs };
    });

    const withTime = normalized.filter((item) => item.startTs > 0);
    const baseStart = withTime.length
      ? withTime.reduce((min, item) => Math.min(min, item.startTs), withTime[0].startTs)
      : Date.now();
    const maxEnd = withTime.length
      ? withTime.reduce((max, item) => Math.max(max, item.endTs || item.startTs), withTime[0].endTs || withTime[0].startTs)
      : baseStart;
    const runDuration = Number(selectedRun?.durationMs ?? 0);
    const windowDuration = Math.max(runDuration > 0 ? runDuration : maxEnd - baseStart, 1);

    const rows = normalized
      .sort((a, b) => a.startTs - b.startTs || a.depthValue - b.depthValue)
      .map((node) => {
        const offsetMs = node.startTs > 0 ? Math.max(0, node.startTs - baseStart) : 0;
        const leftPercent = clamp((offsetMs / windowDuration) * 100, 0, 99.2);
        const widthPercent = clamp(
          (Math.max(node.resolvedDurationMs, 1) / windowDuration) * 100,
          0.8,
          100 - leftPercent
        );
        return { ...node, offsetMs, leftPercent, widthPercent };
      });

    return { totalWindowMs: windowDuration, nodes: rows };
  }, [detail?.nodes, selectedRun?.durationMs]);

  const stats = useMemo(() => {
    const nodes = detail?.nodes || [];
    const total = nodes.length;
    const failed = nodes.filter((node) => normalizeStatus(node.status) === "failed").length;
    const success = nodes.filter((node) => normalizeStatus(node.status) === "success").length;
    const running = nodes.filter((node) => normalizeStatus(node.status) === "running").length;
    const durations = nodes.map((node) => resolveNodeDuration(node));
    const avgDuration = total > 0 ? Math.round(durations.reduce((sum, value) => sum + value, 0) / total) : 0;
    const topSlowestId = [...nodes].sort((a, b) => resolveNodeDuration(b) - resolveNodeDuration(a))[0]?.nodeId;
    return { total, failed, success, running, avgDuration, topSlowestId };
  }, [detail?.nodes]);

  const observability = useMemo(() => {
    const nodes = detail?.nodes || [];
    const retrieveNode = findNode(nodes, "retrieve");
    const toolWorkflowNode = findNode(nodes, "tool_workflow");
    const toolCallNodes = nodes.filter((node) => node.nodeType === "tool_call");
    const fallbackNode = nodes.find((node) => node.nodeId === "fallback" || node.nodeType === "fallback");

    const retrieveExtra = parseTraceExtra(retrieveNode?.extraData);
    const toolExtra = parseTraceExtra(toolWorkflowNode?.extraData);
    const runExtra = parseTraceExtra(selectedRun?.extraData);
    const fallbackFromRun =
      runExtra.fallback && typeof runExtra.fallback === "object" ? (runExtra.fallback as TraceExtra) : {};
    const fallbackExtra = fallbackNode ? parseTraceExtra(fallbackNode.extraData) : fallbackFromRun;

    const retrieve = {
      chunkCount: readNumber(retrieveExtra.chunkCount),
      topScore: readNumber(retrieveExtra.topScore),
      searchChannels: readStringArray(retrieveExtra.searchChannels),
      searchDecisions: readObjectArray<Record<string, unknown>>(retrieveExtra.searchDecisions).map((item) => ({
        query: readString(item.query),
        source: readString(item.source),
        reason: readString(item.reason),
        signals: readStringArray(item.signals)
      })) as RetrieveDecision[],
      channelStats: readObjectArray<Record<string, unknown>>(retrieveExtra.channelStats).map((item) => ({
        name: readString(item.name),
        chunkCount: readNumber(item.chunkCount),
        latencyMs: readNumber(item.latencyMs),
        error: readString(item.error) || undefined,
        metadata: item.metadata && typeof item.metadata === "object" ? (item.metadata as Record<string, unknown>) : undefined
      })) as RetrieveChannelStat[]
    };

    const toolWorkflow = {
      used: readBoolean(toolExtra.used),
      toolCallCount: readNumber(toolExtra.toolCallCount),
      toolNames: readStringArray(toolExtra.toolNames),
      degraded: readBoolean(toolExtra.degraded),
      degradeReason: readString(toolExtra.degradeReason),
      calls: toolCallNodes.map((node) => {
        const extra = parseTraceExtra(node.extraData);
        return {
          nodeId: node.nodeId,
          name: readString(node.nodeName) || readString(extra.toolName) || node.nodeId,
          status: normalizeStatus(node.status),
          summary: readString(extra.summary),
          durationMs: resolveNodeDuration(node),
          error: readString(node.errorMessage) || readString(extra.error)
        };
      }) as ToolCallInsight[]
    };

    const fallback = {
      triggered: readBoolean(fallbackExtra.triggered) || Boolean(readString(fallbackExtra.reason)),
      reason: readString(fallbackExtra.reason),
      durationMs: fallbackNode ? resolveNodeDuration(fallbackNode) : 0
    };

    return { retrieve, toolWorkflow, fallback };
  }, [detail?.nodes, selectedRun?.extraData]);

  if (detailLoading) {
    return (
      <div className="flex min-h-[400px] items-center justify-center">
        <div className="flex flex-col items-center gap-3 text-slate-500">
          <Loader2 className="h-8 w-8 animate-spin" />
          <p>Loading trace detail...</p>
        </div>
      </div>
    );
  }

  if (!traceId || !selectedRun) {
    return (
      <div className="space-y-6">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-1.5 text-sm">
            <Link to="/admin/traces" className="text-slate-500 hover:text-slate-700">
              Trace List
            </Link>
            <span className="text-slate-300">/</span>
            <span className="text-slate-400">Detail</span>
          </div>
          <Button asChild variant="outline" size="sm" className="text-slate-600 hover:text-slate-800">
            <Link to="/admin/traces">
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              Back
            </Link>
          </Button>
        </div>
        <div className="flex min-h-[300px] items-center justify-center">
          <div className="text-center text-slate-500">
            <AlertTriangle className="mx-auto mb-4 h-12 w-12 text-slate-300" />
            <p>{!traceId ? "Missing trace id" : "No trace detail found"}</p>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="space-y-4 pb-8">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1.5 text-sm">
            <Link to="/admin/traces" className="text-slate-500 transition-colors hover:text-slate-700">
              RAG Traces
            </Link>
            <span className="text-slate-300">/</span>
          </div>
          <div className="flex items-center gap-2">
            <h1 className="text-lg font-semibold text-slate-900">{selectedRun.traceName || "Unnamed Trace"}</h1>
            <Badge variant={statusBadgeVariant(selectedRun.status)} className="text-xs">
              {statusLabel(selectedRun.status)}
            </Badge>
          </div>
        </div>

        <div className="flex items-center gap-2">
          <Button asChild variant="outline" size="sm" className="text-slate-600 hover:text-slate-800">
            <Link to="/admin/traces">
              <ArrowLeft className="mr-1.5 h-4 w-4" />
              Back
            </Link>
          </Button>
          <Button
            variant="outline"
            size="sm"
            className="text-slate-600 hover:text-slate-800"
            onClick={() => loadDetail(traceId)}
            disabled={detailLoading}
          >
            <RefreshCw className={cn("mr-1.5 h-4 w-4", detailLoading && "animate-spin")} />
            Refresh
          </Button>
        </div>
      </div>

      <div className="flex items-center gap-4 text-xs text-slate-500">
        <span
          className="flex cursor-pointer items-center gap-1 font-mono transition-colors hover:text-slate-700"
          onClick={() => copyToClipboard(traceId, "Trace ID")}
          title="Copy trace id"
        >
          <Hash className="h-3 w-3" />
          {traceId.length > 28 ? `${traceId.slice(0, 12)}...${traceId.slice(-8)}` : traceId}
        </span>
        <span className="flex items-center gap-1">
          <Calendar className="h-3 w-3" />
          {formatDateTime(selectedRun.startTime ?? undefined)}
        </span>
        {(selectedRun.username || selectedRun.userId) && (
          <span className="flex items-center gap-1">
            <User className="h-3 w-3" />
            {selectedRun.username || selectedRun.userId}
          </span>
        )}
      </div>

      {selectedRun.errorMessage ? (
        <div className="flex items-start gap-3 rounded-lg border border-red-200 bg-red-50 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-red-500" />
          <div className="text-sm">
            <span className="font-medium text-red-800">Trace Error:</span>
            <span className="ml-1 text-red-600">{selectedRun.errorMessage}</span>
          </div>
        </div>
      ) : null}

      {observability.fallback.triggered ? (
        <div className="flex items-start gap-3 rounded-lg border border-amber-200 bg-amber-50 p-3">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0 text-amber-600" />
          <div className="text-sm">
            <span className="font-medium text-amber-900">Fallback Triggered:</span>
            <span className="ml-1 text-amber-700">
              {observability.fallback.reason || "Low-confidence retrieval caused a general-model fallback."}
            </span>
          </div>
        </div>
      ) : null}

      <div className="flex items-center divide-x divide-slate-200 rounded-lg border border-slate-200 bg-slate-50">
        <MetricItem icon={Clock} label="Total" value={formatDuration(selectedRun.durationMs ?? undefined)} variant="primary" />
        <MetricItem icon={Activity} label="Nodes" value={stats.total} />
        <MetricItem icon={CheckCircle2} label="Success" value={stats.success} variant="success" />
        <MetricItem icon={XCircle} label="Failed" value={stats.failed} variant={stats.failed > 0 ? "error" : "default"} />
        {stats.running > 0 ? <MetricItem icon={Loader2} label="Running" value={stats.running} variant="warning" /> : null}
        <MetricItem icon={Zap} label="Avg" value={formatDuration(stats.avgDuration)} />
      </div>

      <div className="grid gap-4 xl:grid-cols-3">
        <Card className="xl:col-span-2">
          <CardHeader className="space-y-3 pb-3">
            <SectionTitle
              title="Retrieve Observability"
              description="Channel-level stats and retrieve-routing decisions."
            />
            <div className="flex flex-wrap gap-2">
              <Badge variant="outline">chunks: {observability.retrieve.chunkCount}</Badge>
              <Badge variant="outline">
                topScore: {observability.retrieve.topScore > 0 ? observability.retrieve.topScore.toFixed(2) : "-"}
              </Badge>
              {observability.retrieve.searchChannels.map((channel) => (
                <Badge key={channel} variant="outline">
                  channel: {channel}
                </Badge>
              ))}
            </div>
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">Channel Stats</p>
              {observability.retrieve.channelStats.length === 0 ? (
                <p className="text-sm text-slate-400">No channel stats recorded.</p>
              ) : (
                <div className="space-y-2">
                  {observability.retrieve.channelStats.map((stat) => (
                    <div key={stat.name} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-3">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="text-sm font-medium text-slate-800">{stat.name || "unknown"}</span>
                        <Badge variant="outline">{stat.chunkCount} chunks</Badge>
                        <Badge variant="outline">{formatDuration(stat.latencyMs)}</Badge>
                        {stat.error ? <Badge variant="destructive">partial failure</Badge> : null}
                      </div>
                      {stat.error ? <p className="mt-2 text-xs text-red-600">{stat.error}</p> : null}
                      {stat.metadata ? <p className="mt-2 break-words text-xs text-slate-500">{formatMetadata(stat.metadata)}</p> : null}
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div>
              <p className="mb-2 text-xs font-medium uppercase tracking-wide text-slate-500">Search Decisions</p>
              {observability.retrieve.searchDecisions.length === 0 ? (
                <p className="text-sm text-slate-400">No search decisions recorded.</p>
              ) : (
                <div className="space-y-2">
                  {observability.retrieve.searchDecisions.map((decision, index) => (
                    <div key={`${decision.query}-${index}`} className="rounded-lg border border-slate-200 bg-white px-3 py-3">
                      <p className="text-sm font-medium text-slate-800">{decision.query || "(empty query)"}</p>
                      <div className="mt-2 flex flex-wrap gap-2">
                        <Badge variant="outline">source: {decision.source || "-"}</Badge>
                      </div>
                      {decision.reason ? <p className="mt-2 text-xs text-slate-600">{decision.reason}</p> : null}
                      {decision.signals.length > 0 ? (
                        <div className="mt-2 flex flex-wrap gap-2">
                          {decision.signals.map((signal) => (
                            <span key={signal} className="rounded-full bg-sky-50 px-2 py-0.5 text-[11px] text-sky-700">
                              {signal}
                            </span>
                          ))}
                        </div>
                      ) : null}
                    </div>
                  ))}
                </div>
              )}
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="space-y-3 pb-3">
            <SectionTitle title="Tool Workflow" description="Tool usage, degraded execution and per-call summaries." />
            <div className="flex flex-wrap gap-2">
              <Badge variant="outline">used: {observability.toolWorkflow.used ? "yes" : "no"}</Badge>
              <Badge variant="outline">calls: {observability.toolWorkflow.toolCallCount}</Badge>
              {observability.toolWorkflow.degraded ? (
                <Badge variant="destructive">degraded</Badge>
              ) : (
                <Badge variant="default">healthy</Badge>
              )}
            </div>
          </CardHeader>
          <CardContent className="space-y-3">
            {observability.toolWorkflow.degradeReason ? (
              <div className="rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-xs text-red-700">
                {observability.toolWorkflow.degradeReason}
              </div>
            ) : null}

            {observability.toolWorkflow.toolNames.length > 0 ? (
              <div className="flex flex-wrap gap-2">
                {observability.toolWorkflow.toolNames.map((name) => (
                  <span key={name} className="rounded-full bg-slate-100 px-2 py-1 text-[11px] text-slate-700">
                    {name}
                  </span>
                ))}
              </div>
            ) : null}

            {observability.toolWorkflow.calls.length === 0 ? (
              <p className="text-sm text-slate-400">No tool call trace nodes recorded.</p>
            ) : (
              <div className="space-y-2">
                {observability.toolWorkflow.calls.map((call) => (
                  <div key={call.nodeId} className="rounded-lg border border-slate-200 bg-slate-50 px-3 py-3">
                    <div className="flex items-center gap-2">
                      <Wrench className="h-4 w-4 text-amber-600" />
                      <span className="text-sm font-medium text-slate-800">{call.name}</span>
                      <Badge variant={statusBadgeVariant(call.status)}>{statusLabel(call.status)}</Badge>
                    </div>
                    <div className="mt-2 flex flex-wrap gap-2">
                      <Badge variant="outline">{formatDuration(call.durationMs)}</Badge>
                    </div>
                    {call.summary ? <p className="mt-2 text-xs text-slate-600">{call.summary}</p> : null}
                    {call.error ? <p className="mt-2 text-xs text-red-600">{call.error}</p> : null}
                  </div>
                ))}
              </div>
            )}
          </CardContent>
        </Card>
      </div>

      <Card>
        <CardHeader className="px-4 py-3">
          <div className="flex items-center justify-between">
            <CardTitle className="text-sm font-medium text-slate-700">Execution Timeline</CardTitle>
            <span className="text-xs text-slate-500">window {formatDuration(timeline.totalWindowMs)}</span>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {timeline.nodes.length === 0 ? (
            <div className="py-16 text-center text-slate-400">
              <Activity className="mx-auto mb-3 h-10 w-10 opacity-50" />
              <p>No trace nodes recorded</p>
            </div>
          ) : (
            <div>
              <div className="grid grid-cols-[minmax(180px,1fr)_120px_2fr_100px] gap-4 border-y border-slate-100 bg-slate-50 px-4 py-2 text-xs font-medium text-slate-500">
                <span>Node</span>
                <span>Type</span>
                <span>Timeline</span>
                <span className="text-right">Duration</span>
              </div>

              <div className="grid grid-cols-[minmax(180px,1fr)_120px_2fr_100px] gap-4 bg-white px-4">
                <div />
                <div />
                <TimeScale totalMs={timeline.totalWindowMs} />
                <div />
              </div>

              <div className="divide-y divide-slate-50">
                {timeline.nodes.map((node) => {
                  const nodeDisplayName = node.nodeName || node.methodName || node.nodeId;
                  const nodeStatus = normalizeStatus(node.status);
                  const isTopSlowest = node.nodeId === stats.topSlowestId;
                  return (
                    <WaterfallRow
                      key={`${node.nodeId}-${node.startTime}-${node.endTime}`}
                      node={node}
                      nodeDisplayName={nodeDisplayName}
                      nodeStatus={nodeStatus}
                      isTopSlowest={isTopSlowest}
                    />
                  );
                })}
              </div>
            </div>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
