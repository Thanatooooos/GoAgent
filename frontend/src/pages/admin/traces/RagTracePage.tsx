import { useEffect, useRef, useState } from "react";
import { RefreshCw, Search } from "lucide-react";
import { useNavigate } from "react-router-dom";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { RunsTable } from "@/pages/admin/traces/components/RunsTable";
import { PAGE_SIZE } from "@/pages/admin/traces/traceUtils";
import { getRagTraceRuns, type PageResult, type RagTraceRun } from "@/services/ragTraceService";
import { getErrorMessage } from "@/utils/error";

export function RagTracePage() {
  const navigate = useNavigate();
  const requestID = useRef(0);
  const [filter, setFilter] = useState("");
  const [traceID, setTraceID] = useState("");
  const [page, setPage] = useState(1);
  const [data, setData] = useState<PageResult<RagTraceRun>>();
  const [loading, setLoading] = useState(false);

  const load = async (nextPage = page, nextTraceID = traceID) => {
    const id = ++requestID.current;
    setLoading(true);
    try {
      const result = await getRagTraceRuns({ current: nextPage, size: PAGE_SIZE, traceId: nextTraceID || undefined });
      if (id === requestID.current) setData(result);
    } catch (error) {
      if (id === requestID.current) toast.error(getErrorMessage(error, "加载运行记录失败"));
    } finally {
      if (id === requestID.current) setLoading(false);
    }
  };

  useEffect(() => { void load(); }, [page, traceID]);

  const search = () => {
    setPage(1);
    setTraceID(filter.trim());
  };

  return (
    <div className="admin-page space-y-5">
      <div className="flex flex-wrap items-end justify-between gap-3">
        <div>
          <h1 className="admin-page-title">Runtime Trace</h1>
          <p className="admin-page-subtitle">会话级执行时间线：模型轮次、工具调用和历史压缩。</p>
        </div>
        <div className="flex gap-2">
          <Input className="w-72" value={filter} placeholder="按 Trace ID 查询" onChange={(event) => setFilter(event.target.value)} onKeyDown={(event) => event.key === "Enter" && search()} />
          <Button onClick={search}><Search className="mr-2 h-4 w-4" />查询</Button>
          <Button variant="outline" onClick={() => void load()}><RefreshCw className="mr-2 h-4 w-4" />刷新</Button>
        </div>
      </div>
      <RunsTable
        runs={data?.records || []}
        loading={loading}
        current={data?.current || page}
        pages={data?.pages || 1}
        total={data?.total || 0}
        onOpenRun={(id) => navigate(`/admin/traces/${encodeURIComponent(id)}`)}
        onPrevPage={() => setPage((current) => Math.max(1, current - 1))}
        onNextPage={() => setPage((current) => current + 1)}
      />
    </div>
  );
}
