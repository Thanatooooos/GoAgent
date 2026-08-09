import { useEffect, useMemo, useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { ArrowLeft, BookOpen, Network } from "lucide-react";

import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

import { getKnowledgeBase } from "@/services/knowledgeService";
import {
  getWikiGraph,
  getWikiPage,
  listWikiPages,
  type WikiGraphData,
  type WikiPageItem
} from "@/services/wikiService";
import { convertWikiLinks } from "@/lib/wikiLinks";
import { getErrorMessage } from "@/utils/error";
import { cn } from "@/lib/utils";

const truncateText = (value?: string | null, max = 100) => {
  if (!value) return "";
  if (value.length <= max) return value;
  return `${value.slice(0, max)}...`;
};

const graphNodeColor = (node: WikiGraphData["nodes"][number]) => {
  const inCount = node.inLinks ?? 0;
  const outCount = node.outLinks ?? 0;
  if (inCount === 0 && outCount === 0) return "#cbd5e1";
  if (outCount >= inCount) return "#10b981";
  return "#3b82f6";
};

export function WikiBrowserPage() {
  const { kbId } = useParams();
  const navigate = useNavigate();
  const [searchParams] = useSearchParams();
  const slugQuery = searchParams.get("slug");

  const [view, setView] = useState<"list" | "graph">("list");
  const [pages, setPages] = useState<WikiPageItem[]>([]);
  const [selected, setSelected] = useState<WikiPageItem | null>(null);
  const [graph, setGraph] = useState<WikiGraphData | null>(null);
  const [kbName, setKbName] = useState("");
  const [loading, setLoading] = useState(false);
  const [pageLoading, setPageLoading] = useState(false);
  const [graphLoading, setGraphLoading] = useState(false);
  const [error, setError] = useState("");
  const [pageError, setPageError] = useState("");
  const [graphError, setGraphError] = useState("");

  useEffect(() => {
    if (!kbId) return;
    let cancelled = false;
    getKnowledgeBase(kbId)
      .then((kb) => {
        if (!cancelled) setKbName(kb.name || kbId);
      })
      .catch(() => {
        if (!cancelled) setKbName(kbId);
      });
    return () => {
      cancelled = true;
    };
  }, [kbId]);

  useEffect(() => {
    if (!kbId) return;
    let cancelled = false;
    setLoading(true);
    setError("");
    listWikiPages(kbId)
      .then((data) => {
        if (!cancelled) setPages(data);
      })
      .catch((err) => {
        if (!cancelled) {
          setError(getErrorMessage(err, "加载 wiki 页面失败"));
          console.error(err);
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [kbId]);

  useEffect(() => {
    if (!kbId || !slugQuery) {
      setSelected(null);
      setPageError("");
      return;
    }
    let cancelled = false;
    setPageLoading(true);
    setPageError("");
    getWikiPage(kbId, slugQuery)
      .then((data) => {
        if (!cancelled) setSelected(data);
      })
      .catch((err) => {
        if (!cancelled) {
          setPageError(getErrorMessage(err, "加载页面失败"));
          console.error(err);
        }
      })
      .finally(() => {
        if (!cancelled) setPageLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [kbId, slugQuery]);

  const loadGraph = async () => {
    if (!kbId) return;
    setGraphLoading(true);
    setGraphError("");
    try {
      const data = await getWikiGraph(kbId);
      setGraph(data);
    } catch (err) {
      setGraphError(getErrorMessage(err, "加载图谱失败"));
      console.error(err);
    } finally {
      setGraphLoading(false);
    }
  };

  const handleSelectPage = (page: WikiPageItem) => {
    setView("list");
    navigate(`?slug=${encodeURIComponent(page.slug)}`);
  };

  const handleSwitchToGraph = () => {
    setView("graph");
    if (!graph && kbId) {
      loadGraph();
    }
  };

  const handleGraphNodeClick = (node: WikiGraphData["nodes"][number]) => {
    setView("list");
    navigate(`?slug=${encodeURIComponent(node.slug)}`);
  };

  const graphLayout = useMemo(() => {
    if (!graph) return null;
    const sorted = [...graph.nodes].sort((a, b) => (b.outLinks ?? 0) - (a.outLinks ?? 0));
    const radius = 180;
    const cx = 250;
    const cy = 250;
    const positions = new Map<string, { x: number; y: number }>();
    sorted.forEach((node, index) => {
      const angle = (2 * Math.PI * index) / sorted.length - Math.PI / 2;
      positions.set(node.id, { x: cx + radius * Math.cos(angle), y: cy + radius * Math.sin(angle) });
    });
    const edges = (graph.edges || []).filter((edge) => positions.has(edge.from) && positions.has(edge.to));
    const totalIn = sorted.reduce((sum, node) => sum + (node.inLinks ?? 0), 0);
    const totalOut = sorted.reduce((sum, node) => sum + (node.outLinks ?? 0), 0);
    return { sorted, positions, edges, totalIn, totalOut };
  }, [graph]);

  return (
    <div className="admin-page">
      <div className="admin-page-header">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="admin-page-title">{kbName || kbId || "Wiki"}</h1>
            <Badge variant="outline" className="border-blue-200 bg-blue-50 text-blue-700">
              Wiki
            </Badge>
          </div>
          <p className="admin-page-subtitle">浏览 wiki 页面与链接图谱</p>
        </div>
        <div className="admin-page-actions">
          <div className="inline-flex items-center rounded-lg border bg-muted p-0.5">
            <button
              type="button"
              onClick={() => setView("list")}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                view === "list"
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              <BookOpen className="h-4 w-4" />
              页面
            </button>
            <button
              type="button"
              onClick={handleSwitchToGraph}
              className={cn(
                "inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                view === "graph"
                  ? "bg-background text-foreground shadow-sm"
                  : "text-muted-foreground hover:text-foreground"
              )}
            >
              <Network className="h-4 w-4" />
              图谱
            </button>
          </div>
          <Link to={`/admin/knowledge/${kbId ?? ""}`}>
            <Button variant="outline">
              <ArrowLeft className="mr-2 h-4 w-4" />
              返回文档
            </Button>
          </Link>
        </div>
      </div>

      {view === "list" ? (
        <div className="grid items-start gap-4 lg:grid-cols-[320px_minmax(0,1fr)]">
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">页面列表</CardTitle>
            </CardHeader>
            <CardContent>
              {loading ? (
                <div className="py-8 text-center text-muted-foreground">加载中...</div>
              ) : error && pages.length === 0 ? (
                <div className="py-8 text-center text-sm text-red-600">{error}</div>
              ) : pages.length === 0 ? (
                <div className="py-8 text-center text-muted-foreground">暂无 wiki 页面</div>
              ) : (
                <ul className="sidebar-scroll max-h-[calc(100vh-260px)] space-y-2 overflow-y-auto pr-1">
                  {pages.map((page) => (
                    <li key={page.id}>
                      <button
                        type="button"
                        onClick={() => handleSelectPage(page)}
                        className={cn(
                          "w-full rounded-lg border p-3 text-left transition-colors",
                          selected?.slug === page.slug
                            ? "border-blue-500 bg-blue-50 dark:bg-blue-950/40"
                            : "hover:bg-accent"
                        )}
                      >
                        <div className="truncate text-sm font-medium">{page.title || page.slug}</div>
                        <div className="mt-1 line-clamp-2 text-xs text-muted-foreground">
                          {truncateText(page.summary, 100) || "暂无摘要"}
                        </div>
                        {page.inLinks != null || page.outLinks != null ? (
                          <div className="mt-2 flex flex-wrap items-center gap-2">
                            {page.inLinks != null ? (
                              <Badge variant="secondary">入链 {page.inLinks}</Badge>
                            ) : null}
                            {page.outLinks != null ? (
                              <Badge variant="secondary">出链 {page.outLinks}</Badge>
                            ) : null}
                          </div>
                        ) : null}
                      </button>
                    </li>
                  ))}
                </ul>
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-3">
              {selected ? (
                <CardTitle className="text-lg">{selected.title || selected.slug}</CardTitle>
              ) : (
                <CardTitle className="text-base">页面内容</CardTitle>
              )}
            </CardHeader>
            <CardContent>
              {pageLoading ? (
                <div className="py-8 text-center text-muted-foreground">加载中...</div>
              ) : pageError ? (
                <div className="py-8 text-center text-sm text-red-600">{pageError}</div>
              ) : selected ? (
                <MarkdownRenderer content={convertWikiLinks(selected.content || "", kbId || "")} />
              ) : (
                <div className="py-8 text-center text-muted-foreground">请选择左侧页面查看内容</div>
              )}
            </CardContent>
          </Card>
        </div>
      ) : (
        <Card>
          <CardHeader>
            <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <CardTitle>链接图谱</CardTitle>
                <CardDescription>节点按出链数排序分布在圆周，点击节点跳转页面</CardDescription>
              </div>
              {graph ? (
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline">节点 {graph.nodes.length}</Badge>
                  <Badge variant="outline">边 {graph.edges.length}</Badge>
                  <Badge variant="secondary">入链 {graphLayout?.totalIn ?? 0}</Badge>
                  <Badge variant="secondary">出链 {graphLayout?.totalOut ?? 0}</Badge>
                </div>
              ) : null}
            </div>
          </CardHeader>
          <CardContent>
            <div className="mb-4 flex flex-wrap items-center gap-4 text-xs text-muted-foreground">
              <span className="inline-flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-full bg-blue-500" />
                入链为主
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-full bg-emerald-500" />
                出链为主
              </span>
              <span className="inline-flex items-center gap-1.5">
                <span className="h-2.5 w-2.5 rounded-full bg-slate-300" />
                无链接
              </span>
            </div>
            {graphLoading ? (
              <div className="py-8 text-center text-muted-foreground">加载中...</div>
            ) : graphError ? (
              <div className="py-8 text-center text-sm text-red-600">{graphError}</div>
            ) : graphLayout && graphLayout.sorted.length === 0 ? (
              <div className="py-8 text-center text-muted-foreground">暂无 wiki 页面</div>
            ) : graphLayout ? (
              <div className="overflow-x-auto">
                <svg viewBox="0 0 500 500" className="mx-auto h-auto w-full max-w-[560px]">
                  {graphLayout.edges.map((edge, index) => {
                    const from = graphLayout.positions.get(edge.from);
                    const to = graphLayout.positions.get(edge.to);
                    if (!from || !to) return null;
                    return (
                      <line
                        key={`${edge.from}-${edge.to}-${index}`}
                        x1={from.x}
                        y1={from.y}
                        x2={to.x}
                        y2={to.y}
                        stroke="#94a3b8"
                        strokeOpacity={0.45}
                        strokeWidth={1}
                      />
                    );
                  })}
                  {graphLayout.sorted.map((node) => {
                    const pos = graphLayout.positions.get(node.id);
                    if (!pos) return null;
                    const isSelected = selected?.slug === node.slug;
                    return (
                      <g
                        key={node.id}
                        transform={`translate(${pos.x}, ${pos.y})`}
                        className="cursor-pointer"
                        onClick={() => handleGraphNodeClick(node)}
                      >
                        <title>{`${node.title}\n入链 ${node.inLinks ?? 0} · 出链 ${node.outLinks ?? 0}`}</title>
                        <circle
                          r={12}
                          fill={graphNodeColor(node)}
                          stroke={isSelected ? "#2563eb" : "rgba(0,0,0,0.15)"}
                          strokeWidth={isSelected ? 3 : 1}
                        />
                        <text
                          y={30}
                          textAnchor="middle"
                          className="fill-slate-700 text-[11px] dark:fill-slate-200"
                        >
                          {truncateText(node.title || node.slug, 8)}
                        </text>
                      </g>
                    );
                  })}
                </svg>
              </div>
            ) : null}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
