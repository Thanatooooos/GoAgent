import { useEffect, useState } from "react";
import { ArrowLeft, Brain, Database, RefreshCw, Wrench } from "lucide-react";
import { Link, useParams } from "react-router-dom";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";
import { getRagTraceDetail, type RagTraceDetail, type RagTraceSpan } from "@/services/ragTraceService";
import { formatDateTime, formatDuration, statusBadgeVariant, statusLabel } from "@/pages/admin/traces/traceUtils";
import { getErrorMessage } from "@/utils/error";

function spanIcon(kind: string) {
  if (kind === "model_turn") return Brain;
  if (kind === "tool_call") return Wrench;
  return Database;
}

function SpanCard({ span }: { span: RagTraceSpan }) {
  const Icon = spanIcon(span.kind);
  const toolDetails = span.kind === "tool_call";
  return (
    <article className="relative border-l-2 border-slate-200 pl-5 pb-5 last:pb-0">
      <span className="absolute -left-[9px] top-0 flex h-4 w-4 items-center justify-center rounded-full bg-slate-100 text-slate-600"><Icon className="h-3 w-3" /></span>
      <div className="rounded-lg border bg-white p-4">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="font-medium text-slate-900">{span.kind === "model_turn" ? `模型调用 #${span.turn}` : span.name}</h3>
          <Badge variant="outline">{span.kind}</Badge>
          <Badge variant={statusBadgeVariant(span.status)}>{statusLabel(span.status)}</Badge>
          <span className="ml-auto text-sm text-slate-500">{formatDuration(span.durationMs)}</span>
        </div>
        <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-xs text-slate-500">
          <span>开始：{formatDateTime(span.startTime)}</span>
          {span.finishReason ? <span>结束原因：{span.finishReason}</span> : null}
          {toolDetails && span.evidenceCount !== undefined ? <span>证据：{span.evidenceCount}</span> : null}
          {span.errorClass ? <span>错误分类：{span.errorClass}</span> : null}
        </div>
        {span.inputSummary ? <pre className="mt-3 max-h-40 overflow-auto rounded bg-slate-50 p-3 text-xs text-slate-700">输入：{span.inputSummary}</pre> : null}
        {span.resultSummary ? <pre className="mt-3 max-h-40 overflow-auto rounded bg-slate-50 p-3 text-xs text-slate-700">结果：{span.resultSummary}</pre> : null}
        {span.errorMessage ? <p className="mt-3 rounded bg-red-50 p-3 text-sm text-red-700">{span.errorMessage}</p> : null}
      </div>
    </article>
  );
}

export function RagTraceDetailPage() {
  const { traceId: encodedTraceID } = useParams<{ traceId: string }>();
  const traceID = encodedTraceID ? decodeURIComponent(encodedTraceID) : "";
  const [detail, setDetail] = useState<RagTraceDetail>();
  const [loading, setLoading] = useState(false);

  const load = async () => {
    if (!traceID) return;
    setLoading(true);
    try {
      setDetail(await getRagTraceDetail(traceID));
    } catch (error) {
      toast.error(getErrorMessage(error, "加载 Trace 详情失败"));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => { void load(); }, [traceID]);

  if (loading && !detail) return <div className="admin-page text-slate-500">加载 Trace 详情…</div>;
  if (!detail) return <div className="admin-page text-slate-500">未找到 Trace。</div>;

  const { run, spans } = detail;
  return (
    <div className="admin-page space-y-5 pb-8">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <Link to="/admin/traces" className="mb-2 inline-flex items-center text-sm text-slate-500 hover:text-slate-800"><ArrowLeft className="mr-1 h-4 w-4" />返回运行列表</Link>
          <div className="flex items-center gap-2"><h1 className="admin-page-title">Runtime Trace</h1><Badge variant={statusBadgeVariant(run.status)}>{statusLabel(run.status)}</Badge></div>
          <p className="mt-1 max-w-2xl break-all font-mono text-xs text-slate-500">{run.traceId}</p>
        </div>
        <Button variant="outline" disabled={loading} onClick={() => void load()}><RefreshCw className={cn("mr-2 h-4 w-4", loading && "animate-spin")} />刷新</Button>
      </div>
      <Card>
        <CardContent className="grid gap-4 p-5 text-sm md:grid-cols-4">
          <div><p className="text-slate-500">总耗时</p><p className="mt-1 font-medium">{formatDuration(run.durationMs)}</p></div>
          <div><p className="text-slate-500">模型轮次 / 工具调用</p><p className="mt-1 font-medium">{run.turnCount} / {run.toolCallCount}</p></div>
          <div><p className="text-slate-500">首个思考 / 内容</p><p className="mt-1 text-xs">{formatDateTime(run.firstThinkingAt)} / {formatDateTime(run.firstContentAt)}</p></div>
          <div><p className="text-slate-500">会话</p><p className="mt-1 truncate font-mono text-xs" title={run.conversationId}>{run.conversationId}</p></div>
        </CardContent>
      </Card>
      {run.errorMessage ? <div className="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700">{run.errorMessage}</div> : null}
      <Card>
        <CardHeader><CardTitle className="text-base">执行时间线</CardTitle></CardHeader>
        <CardContent>
          {spans.length === 0 ? <p className="text-sm text-slate-500">尚未记录执行 span。</p> : <div>{spans.map((span) => <SpanCard key={span.id} span={span} />)}</div>}
        </CardContent>
      </Card>
    </div>
  );
}
