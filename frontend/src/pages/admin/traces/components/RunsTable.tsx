import { Eye } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent } from "@/components/ui/card";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import type { RagTraceRun } from "@/services/ragTraceService";
import { formatDateTime, formatDuration, statusBadgeVariant, statusLabel } from "@/pages/admin/traces/traceUtils";

interface RunsTableProps {
  runs: RagTraceRun[];
  loading: boolean;
  current: number;
  pages: number;
  total: number;
  onOpenRun: (traceId: string) => void;
  onPrevPage: () => void;
  onNextPage: () => void;
}

export function RunsTable({ runs, loading, current, pages, total, onOpenRun, onPrevPage, onNextPage }: RunsTableProps) {
  return (
    <Card>
      <CardContent className="p-0">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>会话</TableHead>
              <TableHead>Trace ID</TableHead>
              <TableHead>模型轮次</TableHead>
              <TableHead>工具调用</TableHead>
              <TableHead>耗时</TableHead>
              <TableHead>状态</TableHead>
              <TableHead>开始时间</TableHead>
              <TableHead />
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              <TableRow><TableCell colSpan={8} className="h-24 text-center text-slate-500">加载中…</TableCell></TableRow>
            ) : runs.length === 0 ? (
              <TableRow><TableCell colSpan={8} className="h-24 text-center text-slate-500">暂无 runtime 运行记录</TableCell></TableRow>
            ) : runs.map((run) => (
              <TableRow key={run.traceId}>
                <TableCell className="max-w-40 truncate font-mono text-xs" title={run.conversationId}>{run.conversationId}</TableCell>
                <TableCell className="max-w-40 truncate font-mono text-xs" title={run.traceId}>{run.traceId}</TableCell>
                <TableCell>{run.turnCount}</TableCell>
                <TableCell>{run.toolCallCount}</TableCell>
                <TableCell>{formatDuration(run.durationMs)}</TableCell>
                <TableCell><Badge variant={statusBadgeVariant(run.status)}>{statusLabel(run.status)}</Badge></TableCell>
                <TableCell className="text-xs text-slate-500">{formatDateTime(run.startTime)}</TableCell>
                <TableCell className="text-right">
                  <Button size="sm" variant="outline" onClick={() => onOpenRun(run.traceId)}><Eye className="mr-1 h-3.5 w-3.5" />详情</Button>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
        <div className="flex items-center justify-between border-t px-4 py-3 text-sm text-slate-500">
          <span>第 {current} / {Math.max(pages, 1)} 页，共 {total} 条</span>
          <div className="flex gap-2">
            <Button size="sm" variant="outline" disabled={loading || current <= 1} onClick={onPrevPage}>上一页</Button>
            <Button size="sm" variant="outline" disabled={loading || current >= pages} onClick={onNextPage}>下一页</Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}
