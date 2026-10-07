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

const graphEdgeColor = (node: WikiGraphData["nodes"][number] | undefined, highlighted: boolean) => {
  if (highlighted) return "#2563eb";
  return node ? graphNodeColor(node) : "#94a3b8";
};

type GraphMode = "focus" | "all";

type GraphPosition = { x: number; y: number };

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
  const [graphMode, setGraphMode] = useState<GraphMode>("focus");
  const [graphSearch, setGraphSearch] = useState("");
  const [graphFocusID, setGraphFocusID] = useState<string | null>(null);

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
    setGraphMode("focus");
    if (!graph && kbId) {
      loadGraph();
    }
  };

  const handleGraphNodeClick = (node: WikiGraphData["nodes"][number]) => {
    setGraphFocusID(node.id);
    setGraphMode("focus");
  };

  const handleGraphSearchChange = (value: string) => {
    setGraphSearch(value);
    if (!graph || !value.trim()) return;
    const query = value.trim().toLowerCase();
    const match = graph.nodes.find((node) =>
      `${node.title} ${node.slug}`.toLowerCase().includes(query)
    );
    if (match) {
      setGraphFocusID(match.id);
      setGraphMode("focus");
    }
  };

  const handleOpenGraphPage = (node: WikiGraphData["nodes"][number]) => {
    setView("list");
    navigate(`?slug=${encodeURIComponent(node.slug)}`);
  };

  const graphLayout = useMemo(() => {
    if (!graph) return null;
    const sorted = [...graph.nodes].sort((a, b) => {
      const degreeA = (a.inLinks ?? 0) + (a.outLinks ?? 0);
      const degreeB = (b.inLinks ?? 0) + (b.outLinks ?? 0);
      return degreeB - degreeA || (a.title || a.slug).localeCompare(b.title || b.slug);
    });
    const nodesByID = new Map(sorted.map((node) => [node.id, node]));
    const focusNode =
      nodesByID.get(graphFocusID || "") ||
      (selected ? sorted.find((node) => node.slug === selected.slug) : undefined) ||
      sorted[0];
    const allEdges = (graph.edges || []).filter((edge) => nodesByID.has(edge.from) && nodesByID.has(edge.to));
    const relatedEdges = focusNode
      ? allEdges.filter((edge) => edge.from === focusNode.id || edge.to === focusNode.id)
      : [];
    const relatedNodeIDs = new Set<string>();
    if (focusNode) {
      relatedNodeIDs.add(focusNode.id);
      relatedEdges.forEach((edge) => {
        relatedNodeIDs.add(edge.from);
        relatedNodeIDs.add(edge.to);
      });
    }

    const positions = new Map<string, GraphPosition>();
    let visibleNodes = sorted;
    let visibleEdges = allEdges;
    let hiddenNodeCount = 0;
    let hiddenRelatedCount = 0;

    if (graphMode === "focus" && focusNode) {
      const relatedNodes = sorted.filter((node) => relatedNodeIDs.has(node.id) && node.id !== focusNode.id);
      const maxVisibleRelated = 14;
      const visibleRelated = relatedNodes.slice(0, maxVisibleRelated);
      visibleNodes = [focusNode, ...visibleRelated];
      visibleEdges = relatedEdges.filter(
        (edge) => visibleNodes.some((node) => node.id === edge.from) && visibleNodes.some((node) => node.id === edge.to)
      );
      hiddenNodeCount = sorted.length - visibleNodes.length;
      hiddenRelatedCount = Math.max(0, relatedNodes.length - visibleRelated.length);
      positions.set(focusNode.id, { x: 280, y: 220 });
      const radius = visibleRelated.length > 8 ? 172 : 150;
      visibleRelated.forEach((node, index) => {
        const angle = (2 * Math.PI * index) / Math.max(visibleRelated.length, 1) - Math.PI / 2;
        positions.set(node.id, { x: 280 + radius * Math.cos(angle), y: 220 + radius * Math.sin(angle) });
      });
    } else {
      const radius = 180;
      sorted.forEach((node, index) => {
        const angle = (2 * Math.PI * index) / Math.max(sorted.length, 1) - Math.PI / 2;
        positions.set(node.id, { x: 280 + radius * Math.cos(angle), y: 220 + radius * Math.sin(angle) });
      });
    }

    const totalIn = sorted.reduce((sum, node) => sum + (node.inLinks ?? 0), 0);
    const totalOut = sorted.reduce((sum, node) => sum + (node.outLinks ?? 0), 0);
    return {
      sorted,
      visibleNodes,
      positions,
      nodesByID,
      edges: visibleEdges,
      focusEdges: relatedEdges,
      totalIn,
      totalOut,
      focusNode,
      relatedNodeIDs,
      hiddenNodeCount,
      hiddenRelatedCount
    };
  }, [graph, graphFocusID, graphMode, selected]);

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
                <MarkdownRenderer
                  content={convertWikiLinks(selected.content || "", kbId || "")}
                  openLinksInNewTab={false}
                />
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
                <CardDescription>默认聚焦当前节点及其直接关系，点击节点可切换焦点</CardDescription>
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
            {graphLoading ? (
              <div className="py-8 text-center text-muted-foreground">加载中...</div>
            ) : graphError ? (
              <div className="py-8 text-center text-sm text-red-600">{graphError}</div>
            ) : graphLayout && graphLayout.sorted.length === 0 ? (
              <div className="py-8 text-center text-muted-foreground">暂无 wiki 页面</div>
            ) : graphLayout ? (
              <div className="space-y-4">
                <div className="flex flex-col gap-3 rounded-lg border bg-muted/20 p-3 md:flex-row md:items-center md:justify-between">
                  <label className="flex min-w-0 flex-1 items-center gap-2">
                    <span className="shrink-0 text-sm font-medium">定位节点</span>
                    <input
                      value={graphSearch}
                      onChange={(event) => handleGraphSearchChange(event.target.value)}
                      placeholder="搜索标题或 slug"
                      aria-label="搜索 Wiki 图谱节点"
                      className="h-9 min-w-0 flex-1 rounded-md border bg-background px-3 text-sm outline-none ring-offset-background placeholder:text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring"
                    />
                  </label>
                  <div className="inline-flex shrink-0 items-center rounded-lg border bg-background p-0.5">
                    <button
                      type="button"
                      aria-pressed={graphMode === "focus"}
                      onClick={() => setGraphMode("focus")}
                      className={cn(
                        "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                        graphMode === "focus"
                          ? "bg-blue-600 text-white shadow-sm"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      聚焦关系
                    </button>
                    <button
                      type="button"
                      aria-pressed={graphMode === "all"}
                      onClick={() => setGraphMode("all")}
                      className={cn(
                        "rounded-md px-3 py-1.5 text-sm font-medium transition-colors",
                        graphMode === "all"
                          ? "bg-blue-600 text-white shadow-sm"
                          : "text-muted-foreground hover:text-foreground"
                      )}
                    >
                      全部关系
                    </button>
                  </div>
                </div>

                <div className="grid overflow-hidden rounded-lg border lg:grid-cols-[minmax(0,1fr)_280px]">
                  <div className="min-w-0 bg-slate-50/70 p-2 dark:bg-slate-950/20 md:p-4">
                    <div className="mb-2 flex flex-wrap items-center gap-4 px-2 text-xs text-muted-foreground">
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
                    <svg
                      data-testid="wiki-graph-svg"
                      viewBox="0 0 560 440"
                      className="mx-auto h-auto w-full max-w-[760px]"
                    >
                      {graphLayout.edges.map((edge, index) => {
                        const from = graphLayout.positions.get(edge.from);
                        const to = graphLayout.positions.get(edge.to);
                        const sourceNode = graphLayout.nodesByID.get(edge.from);
                        const targetNode = graphLayout.nodesByID.get(edge.to);
                        const highlighted = graphLayout.focusNode?.id === edge.from || graphLayout.focusNode?.id === edge.to;
                        if (!from || !to) return null;
                        return (
                          <line
                            key={`${edge.from}-${edge.to}-${index}`}
                            x1={from.x}
                            y1={from.y}
                            x2={to.x}
                            y2={to.y}
                            stroke={graphEdgeColor(sourceNode, highlighted)}
                            strokeOpacity={graphMode === "focus" ? 0.8 : highlighted ? 0.85 : 0.24}
                            strokeWidth={graphMode === "focus" && highlighted ? 2 : 1.1}
                          >
                            <title>{`${sourceNode?.title || edge.from} → ${targetNode?.title || edge.to}`}</title>
                          </line>
                        );
                      })}
                      {graphLayout.visibleNodes.map((node) => {
                        const pos = graphLayout.positions.get(node.id);
                        if (!pos) return null;
                        const isFocus = graphLayout.focusNode?.id === node.id;
                        const isRelated = graphLayout.relatedNodeIDs.has(node.id);
                        const showLabel = graphMode === "focus" || isFocus;
                        return (
                          <g
                            key={node.id}
                            data-node-id={node.id}
                            transform={`translate(${pos.x}, ${pos.y})`}
                            className="cursor-pointer"
                            onClick={() => handleGraphNodeClick(node)}
                          >
                            <title>{`${node.title}\n入链 ${node.inLinks ?? 0} · 出链 ${node.outLinks ?? 0}`}</title>
                            <circle
                              r={isFocus ? 20 : graphMode === "all" && !isRelated ? 8 : 13}
                              fill={graphNodeColor(node)}
                              fillOpacity={graphMode === "all" && !isRelated ? 0.7 : 1}
                              stroke={isFocus ? "#1d4ed8" : "rgba(0,0,0,0.15)"}
                              strokeWidth={isFocus ? 4 : 1}
                            />
                            {showLabel ? (
                              <text
                                y={isFocus ? 34 : 28}
                                textAnchor="middle"
                                className="fill-slate-700 text-[11px] dark:fill-slate-200"
                              >
                                {truncateText(node.title || node.slug, isFocus ? 16 : 12)}
                              </text>
                            ) : null}
                          </g>
                        );
                      })}
                    </svg>
                    <div className="mt-2 flex flex-wrap items-center justify-between gap-2 px-2 text-xs text-muted-foreground">
                      <span>
                        {graphMode === "focus"
                          ? `当前显示 ${graphLayout.visibleNodes.length} 个节点 · ${graphLayout.edges.length} 条直接关系`
                          : `当前显示全部 ${graphLayout.visibleNodes.length} 个节点 · ${graphLayout.edges.length} 条关系`}
                      </span>
                      {graphLayout.hiddenNodeCount > 0 ? (
                        <span>
                          已隐藏 {graphLayout.hiddenNodeCount} 个无关节点
                          {graphLayout.hiddenRelatedCount > 0 ? `，另有 ${graphLayout.hiddenRelatedCount} 条关系未展开` : ""}
                        </span>
                      ) : null}
                    </div>
                  </div>

                  <aside data-testid="wiki-graph-details" className="border-t bg-background p-4 lg:border-l lg:border-t-0">
                    {graphLayout.focusNode ? (
                      <div className="space-y-4">
                        <div>
                          <div className="text-xs font-medium uppercase tracking-wide text-muted-foreground">当前节点</div>
                          <h3 className="mt-1 break-words text-base font-semibold">
                            {graphLayout.focusNode.title || graphLayout.focusNode.slug}
                          </h3>
                          <p className="mt-1 break-all text-xs text-muted-foreground">{graphLayout.focusNode.slug}</p>
                        </div>
                        <div className="grid grid-cols-2 gap-2 text-sm">
                          <div className="rounded-md bg-blue-50 p-2 text-blue-800 dark:bg-blue-950/30 dark:text-blue-200">
                            <div className="text-xs opacity-75">入链</div>
                            <div className="mt-1 text-lg font-semibold">{graphLayout.focusNode.inLinks ?? 0}</div>
                          </div>
                          <div className="rounded-md bg-emerald-50 p-2 text-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-200">
                            <div className="text-xs opacity-75">出链</div>
                            <div className="mt-1 text-lg font-semibold">{graphLayout.focusNode.outLinks ?? 0}</div>
                          </div>
                        </div>
                        <Button className="w-full" onClick={() => handleOpenGraphPage(graphLayout.focusNode!)}>
                          打开 Wiki 页面
                        </Button>
                        <div>
                          <div className="mb-2 text-sm font-medium">直接关系</div>
                          {graphLayout.focusEdges.length === 0 ? (
                            <p className="text-xs text-muted-foreground">暂无直接关系</p>
                          ) : (
                            <ul className="max-h-52 space-y-1.5 overflow-y-auto pr-1">
                              {graphLayout.focusEdges.map((edge, index) => {
                                const otherID = edge.from === graphLayout.focusNode?.id ? edge.to : edge.from;
                                const otherNode = graphLayout.nodesByID.get(otherID);
                                if (!otherNode) return null;
                                return (
                                  <li key={`${edge.from}-${edge.to}-${index}`}>
                                    <button
                                      type="button"
                                      onClick={() => handleGraphNodeClick(otherNode)}
                                      className="w-full rounded-md border px-2.5 py-2 text-left text-xs transition-colors hover:bg-accent"
                                    >
                                      <span className="block truncate font-medium">{otherNode.title || otherNode.slug}</span>
                                      <span className="mt-0.5 block text-muted-foreground">
                                        {edge.from === graphLayout.focusNode?.id ? "出链" : "入链"}
                                      </span>
                                    </button>
                                  </li>
                                );
                              })}
                            </ul>
                          )}
                        </div>
                      </div>
                    ) : (
                      <p className="text-sm text-muted-foreground">暂无可展示节点</p>
                    )}
                  </aside>
                </div>
              </div>
            ) : null}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
