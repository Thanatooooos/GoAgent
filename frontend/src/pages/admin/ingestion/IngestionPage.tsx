import { useEffect, useState, type ReactNode } from "react";
import { useSearchParams } from "react-router-dom";
import {
  Activity,
  ClipboardList,
  FileUp,
  FolderKanban,
  Gauge,
  Pencil,
  Plus,
  RefreshCw,
  RotateCcw,
  TimerReset,
  TriangleAlert,
  Trash2
} from "lucide-react";
import { toast } from "sonner";
import { zodResolver } from "@hookform/resolvers/zod";
import { useForm } from "react-hook-form";
import * as z from "zod";

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle
} from "@/components/ui/alert-dialog";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle
} from "@/components/ui/dialog";
import {
  Form,
  FormControl,
  FormDescription,
  FormField,
  FormItem,
  FormLabel,
  FormMessage
} from "@/components/ui/form";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow
} from "@/components/ui/table";
import { Textarea } from "@/components/ui/textarea";
import type {
  IngestionMetricsSnapshot,
  IngestionNodeContract,
  IngestionNodeRequirement,
  IngestionNodeMetrics,
  IngestionPipeline,
  IngestionPipelineDefinition,
  IngestionPipelineNode,
  IngestionPipelinePayload,
  IngestionTask,
  IngestionTaskCreatePayload,
  IngestionTaskNode,
  PageResult
} from "@/services/ingestionService";
import {
  createIngestionPipeline,
  createIngestionTask,
  deleteIngestionPipeline,
  getIngestionMetrics,
  getIngestionPipeline,
  getIngestionPipelineContracts,
  getIngestionPipelines,
  getIngestionTask,
  getIngestionTaskNodes,
  getIngestionTasks,
  updateIngestionPipeline,
  uploadIngestionTask
} from "@/services/ingestionService";
import { getSystemSettings } from "@/services/settingsService";
import { getErrorMessage } from "@/utils/error";
const PIPELINE_PAGE_SIZE = 10;
const TASK_PAGE_SIZE = 10;
const METRICS_POLL_INTERVAL_MS = 10000;

const STATUS_OPTIONS = [
  { value: "pending", label: "pending" },
  { value: "running", label: "running" },
  { value: "completed", label: "completed" },
  { value: "failed", label: "failed" }
];

const SOURCE_OPTIONS = [
  { value: "file", label: "Local File" },
  { value: "url", label: "Remote URL" },
  { value: "feishu", label: "Feishu" },
  { value: "s3", label: "S3" }
];

const NODE_TYPE_OPTIONS = [
  { value: "fetcher", label: "fetcher" },
  { value: "parser", label: "parser" },
  { value: "enhancer", label: "enhancer" },
  { value: "chunker", label: "chunker" },
  { value: "enricher", label: "enricher" },
  { value: "indexer", label: "indexer" }
];

const CHUNK_STRATEGY_OPTIONS = [
  { value: "fixed_size", label: "fixed_size" },
  { value: "structure_aware", label: "structure_aware" }
];

const ENHANCER_TASK_OPTIONS = [
  { value: "context_enhance", label: "context_enhance" },
  { value: "keywords", label: "keywords" },
  { value: "questions", label: "questions" },
  { value: "metadata", label: "metadata" }
];

const ENRICHER_TASK_OPTIONS = [
  { value: "keywords", label: "keywords" },
  { value: "summary", label: "summary" },
  { value: "metadata", label: "metadata" }
];

const formatDate = (value?: string | null) => {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString("zh-CN");
};

const stringifyJson = (value: unknown) => {
  if (!value) return "-";
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
};

const truncateJson = (value: unknown, max = 120) => {
  const raw = stringifyJson(value);
  if (raw.length <= max) return raw;
  return `${raw.slice(0, max)}...`;
};

const formatPercent = (value?: number | null) => {
  if (typeof value !== "number" || Number.isNaN(value)) return "-";
  return `${(value * 100).toFixed(1)}%`;
};

const formatDuration = (value?: number | null) => {
  if (typeof value !== "number" || Number.isNaN(value)) return "-";
  return `${value} ms`;
};

const statusBadgeVariant = (status?: string | null) => {
  if (!status) return "outline";
  const normalized = status.toLowerCase();
  if (normalized === "completed") return "default";
  if (normalized === "failed") return "destructive";
  if (normalized === "running") return "secondary";
  return "outline";
};

const nodeStatusVariant = (status?: string | null) => {
  if (!status) return "outline";
  const normalized = status.toLowerCase();
  if (normalized === "success") return "default";
  if (normalized === "failed") return "destructive";
  return "secondary";
};

const buildDefinitionFromLegacyNodes = (
  nodes?: IngestionPipelineNode[] | null
): IngestionPipelineDefinition => {
  const normalizedNodes = nodes || [];
  const edges = normalizedNodes
    .filter((node) => node.nextNodeId)
    .map((node, index) => ({
      edgeId: `${node.nodeId}__to__${node.nextNodeId}`,
      fromNodeId: node.nodeId,
      toNodeId: String(node.nextNodeId),
      condition: (node.condition as Record<string, unknown> | null) ?? null,
      priority: index
    }));
  const inbound = new Set(edges.map((edge) => edge.toNodeId));
  const entryNodeIds = normalizedNodes
    .map((node) => node.nodeId)
    .filter((nodeId) => !inbound.has(nodeId));
  return {
    version: "v1",
    entryNodeIds,
    nodes: normalizedNodes,
    edges
  };
};

const pipelineSchema = z.object({
  name: z.string().min(1, "Enter pipeline name").max(60, "Name must be at most 60 characters"),
  description: z.string().optional(),
  nodesJson: z.string().optional()
});

type PipelineFormValues = z.infer<typeof pipelineSchema>;

type PipelineNodeType = "fetcher" | "parser" | "enhancer" | "chunker" | "enricher" | "indexer";

interface EnhancerTaskForm {
  id: string;
  type: string;
  systemPrompt: string;
  userPromptTemplate: string;
}

interface PipelineNodeForm {
  id: string;
  nodeId: string;
  nodeType: PipelineNodeType;
  nextNodeId: string;
  condition: string;
  chunker: {
    strategy: string;
    chunkSize: string;
    overlapSize: string;
    separator: string;
  };
  enhancer: {
    modelId: string;
    tasks: EnhancerTaskForm[];
  };
  enricher: {
    modelId: string;
    attachDocumentMetadata: boolean;
    tasks: EnhancerTaskForm[];
  };
  parser: {
    rulesJson: string;
  };
  indexer: {
    embeddingModel: string;
    metadataFields: string;
  };
}

const taskSchema = z
  .object({
    pipelineId: z.string().min(1, "Select a pipeline"),
    sourceType: z.string().min(1, "请选择来源类型"),
    location: z.string().optional(),
    fileName: z.string().optional(),
    credentialsJson: z.string().optional(),
    metadataJson: z.string().optional()
  })
  .superRefine((values, ctx) => {
    if (values.sourceType !== "file" && !values.location?.trim()) {
      ctx.addIssue({
        code: z.ZodIssueCode.custom,
        path: ["location"],
        message: "请输入来源地址"
      });
    }
  });

type TaskFormValues = z.infer<typeof taskSchema>;

export function IngestionPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const tabParam = searchParams.get("tab");
  const [activeTab, setActiveTab] = useState<"pipelines" | "tasks">(() =>
    tabParam === "tasks" ? "tasks" : "pipelines"
  );
  const [pipelinePage, setPipelinePage] = useState<PageResult<IngestionPipeline> | null>(null);
  const [pipelineKeyword, setPipelineKeyword] = useState("");
  const [pipelineSearch, setPipelineSearch] = useState("");
  const [pipelinePageNo, setPipelinePageNo] = useState(1);
  const [pipelineLoading, setPipelineLoading] = useState(false);
  const [pipelineDialog, setPipelineDialog] = useState<{
    open: boolean;
    mode: "create" | "edit";
    pipeline: IngestionPipeline | null;
  }>({ open: false, mode: "create", pipeline: null });
  const [pipelineNodesDialog, setPipelineNodesDialog] = useState<{
    open: boolean;
    pipeline: IngestionPipeline | null;
  }>({ open: false, pipeline: null });
  const [pipelineDeleteTarget, setPipelineDeleteTarget] = useState<IngestionPipeline | null>(null);
  const [pipelineContracts, setPipelineContracts] = useState<IngestionNodeContract[]>([]);
  const [pipelineOptions, setPipelineOptions] = useState<IngestionPipeline[]>([]);

  const [taskPage, setTaskPage] = useState<PageResult<IngestionTask> | null>(null);
  const [taskStatus, setTaskStatus] = useState<string | undefined>();
  const [taskPageNo, setTaskPageNo] = useState(1);
  const [taskLoading, setTaskLoading] = useState(false);
  const [metrics, setMetrics] = useState<IngestionMetricsSnapshot | null>(null);
  const [metricsLoading, setMetricsLoading] = useState(false);
  const [taskDialogOpen, setTaskDialogOpen] = useState(false);
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false);
  const [taskDetail, setTaskDetail] = useState<{ open: boolean; taskId: string | null }>({
    open: false,
    taskId: null
  });

  const pipelines = pipelinePage?.records || [];
  const tasks = taskPage?.records || [];

  const loadPipelines = async (pageNo = pipelinePageNo, keyword = pipelineKeyword) => {
    setPipelineLoading(true);
    try {
      const data = await getIngestionPipelines(pageNo, PIPELINE_PAGE_SIZE, keyword || undefined);
      setPipelinePage(data);
    } catch (error) {
      toast.error(getErrorMessage(error, "Failed to load pipelines"));
      console.error(error);
    } finally {
      setPipelineLoading(false);
    }
  };

  const loadPipelineOptions = async () => {
    try {
      const data = await getIngestionPipelines(1, 200);
      setPipelineOptions(data.records || []);
    } catch (error) {
      console.error(error);
    }
  };

  const loadPipelineContracts = async () => {
    try {
      const data = await getIngestionPipelineContracts();
      setPipelineContracts(data || []);
    } catch (error) {
      console.error(error);
    }
  };

  const loadTasks = async (pageNo = taskPageNo, status = taskStatus) => {
    setTaskLoading(true);
    try {
      const data = await getIngestionTasks(pageNo, TASK_PAGE_SIZE, status);
      setTaskPage(data);
    } catch (error) {
      toast.error(getErrorMessage(error, "加载任务失败"));
      console.error(error);
    } finally {
      setTaskLoading(false);
    }
  };

  const loadMetrics = async (silent = false) => {
    if (!silent) {
      setMetricsLoading(true);
    }
    try {
      const data = await getIngestionMetrics();
      setMetrics(data);
    } catch (error) {
      if (!silent) {
        toast.error(getErrorMessage(error, "加载指标失败"));
      }
      console.error(error);
    } finally {
      if (!silent) {
        setMetricsLoading(false);
      }
    }
  };

  useEffect(() => {
    loadPipelines();
  }, [pipelinePageNo, pipelineKeyword]);

  useEffect(() => {
    loadTasks();
  }, [taskPageNo, taskStatus]);

  useEffect(() => {
    loadPipelineOptions();
    loadPipelineContracts();
  }, []);

  useEffect(() => {
    if (activeTab !== "tasks") return;
    loadMetrics();
    const timer = window.setInterval(() => {
      loadMetrics(true);
    }, METRICS_POLL_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [activeTab]);

  useEffect(() => {
    if (tabParam === "tasks" || tabParam === "pipelines") {
      setActiveTab(tabParam);
      return;
    }
    setSearchParams({ tab: "pipelines" }, { replace: true });
  }, [tabParam, setSearchParams]);

  const handleTabChange = (next: "pipelines" | "tasks") => {
    setActiveTab(next);
    setSearchParams({ tab: next }, { replace: true });
  };

  const handlePipelineSearch = () => {
    setPipelinePageNo(1);
    setPipelineKeyword(pipelineSearch.trim());
  };

  const handlePipelineRefresh = () => {
    setPipelinePageNo(1);
    loadPipelines(1, pipelineKeyword);
    loadPipelineOptions();
  };

  const handleTaskRefresh = () => {
    setTaskPageNo(1);
    loadTasks(1, taskStatus);
    loadPipelineOptions();
    loadMetrics();
  };

  const handlePipelineDelete = async () => {
    if (!pipelineDeleteTarget) return;
    try {
      await deleteIngestionPipeline(pipelineDeleteTarget.id);
      toast.success("删除成功");
      setPipelineDeleteTarget(null);
      await loadPipelines(1, pipelineKeyword);
      await loadPipelineOptions();
    } catch (error) {
      toast.error(getErrorMessage(error, "删除失败"));
      console.error(error);
    }
  };

  const openPipelineNodes = async (pipeline: IngestionPipeline) => {
    try {
      const detail = await getIngestionPipeline(pipeline.id);
      setPipelineNodesDialog({ open: true, pipeline: detail });
    } catch (error) {
      toast.error(getErrorMessage(error, "Failed to load pipeline details"));
      console.error(error);
    }
  };

  const taskStatusLabel = (status?: string | null) => (status ? status.toLowerCase() : "unknown");

  return (
    <div className="admin-page">
      <div className="admin-page-header">
        <div>
          <h1 className="admin-page-title">数据通道</h1>
          <p className="admin-page-subtitle">管理通道流水线与任务执行情况</p>
        </div>
        <div className="admin-page-actions">
          <Button
            variant={activeTab === "pipelines" ? "default" : "outline"}
            size="sm"
            onClick={() => handleTabChange("pipelines")}
          >
            <FolderKanban className="mr-2 h-4 w-4" />
            流水�?          </Button>
          <Button
            variant={activeTab === "tasks" ? "default" : "outline"}
            size="sm"
            onClick={() => handleTabChange("tasks")}
          >
            <ClipboardList className="mr-2 h-4 w-4" />
            任务
          </Button>
        </div>
      </div>

      {activeTab === "pipelines" ? (
        <Card>
          <CardHeader>
            <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
              <div>
                <CardTitle>Channel Pipelines</CardTitle>
                <CardDescription>配置节点顺序与处理逻辑</CardDescription>
              </div>
              <div className="flex flex-1 items-center justify-end gap-2">
                <Input
                  value={pipelineSearch}
                  onChange={(event) => setPipelineSearch(event.target.value)}
                  placeholder="Search pipelines"
                  className="max-w-xs"
                />
                <Button variant="outline" onClick={handlePipelineSearch}>
                  搜索
                </Button>
                <Button variant="outline" onClick={handlePipelineRefresh}>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  刷新
                </Button>
                <Button
                  className="admin-primary-gradient"
                  onClick={() => setPipelineDialog({ open: true, mode: "create", pipeline: null })}
                >
                  <Plus className="mr-2 h-4 w-4" />
                  新建流水�?                </Button>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {pipelineLoading ? (
              <div className="py-10 text-center text-muted-foreground">加载�?..</div>
            ) : pipelines.length === 0 ? (
              <div className="py-10 text-center text-muted-foreground">No pipelines</div>
            ) : (
              <Table className="min-w-[920px]">
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-[260px]">名称</TableHead>
                    <TableHead>描述</TableHead>
                    <TableHead className="w-[90px]">Nodes</TableHead>
                    <TableHead className="w-[120px]">Owner</TableHead>
                    <TableHead className="w-[170px]">更新时间</TableHead>
                    <TableHead className="w-[180px] text-left">操作</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {pipelines.map((pipeline) => (
                    <TableRow key={pipeline.id}>
                      <TableCell className="font-medium">{pipeline.name}</TableCell>
                      <TableCell className="text-muted-foreground">
                        {pipeline.description || "-"}
                      </TableCell>
                      <TableCell>{pipeline.nodes?.length ?? 0}</TableCell>
                      <TableCell>{pipeline.createdBy || "-"}</TableCell>
                      <TableCell>{formatDate(pipeline.updateTime)}</TableCell>
                      <TableCell className="text-right">
                        <div className="flex justify-end gap-2">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => openPipelineNodes(pipeline)}
                          >
                            查看节点
                          </Button>
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              setPipelineDialog({ open: true, mode: "edit", pipeline })
                            }
                          >
                            <Pencil className="mr-0.1 h-4 w-4" />
                            编辑
                          </Button>
                          <Button
                            size="sm"
                            variant="ghost"
                            className="text-destructive hover:text-destructive"
                            onClick={() => setPipelineDeleteTarget(pipeline)}
                          >
                            <Trash2 className="mr-0.1 h-4 w-4" />
                            删除
                          </Button>
                        </div>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}

            <Pagination
              current={pipelinePage?.current || 1}
              pages={pipelinePage?.pages || 1}
              total={pipelinePage?.total || 0}
              onPrev={() => setPipelinePageNo((prev) => Math.max(1, prev - 1))}
              onNext={() =>
                setPipelinePageNo((prev) => Math.min(pipelinePage?.pages || 1, prev + 1))
              }
            />
          </CardContent>
        </Card>
      ) : (
        <div className="space-y-6">
          <Card>
            <CardHeader>
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <CardTitle>运行指标</CardTitle>
                  <CardDescription>查看当前执行器负载、任务结果与节点耗时分布</CardDescription>
                </div>
                <Button variant="outline" onClick={() => loadMetrics()} disabled={metricsLoading}>
                  <RefreshCw className="mr-2 h-4 w-4" />
                  {metricsLoading ? "刷新�?.." : "刷新指标"}
                </Button>
              </div>
            </CardHeader>
            <CardContent className="space-y-6">
              <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
                <MetricsStatCard
                  title="Running tasks"
                  value={String(metrics?.runningTasks ?? 0)}
                  description={`已占�?${metrics?.usedSlots ?? 0} / ${metrics?.maxConcurrent ?? 0} 并发`}
                  icon={<Activity className="h-4 w-4" />}
                />
                <MetricsStatCard
                  title="Task success rate"
                  value={formatPercent(metrics?.rates.successRate)}
                  description={`成功 ${metrics?.totals.succeeded ?? 0} / 完成 ${(metrics?.totals.succeeded ?? 0) + (metrics?.totals.failed ?? 0) + (metrics?.totals.canceled ?? 0)}`}
                  icon={<Gauge className="h-4 w-4" />}
                />
                <MetricsStatCard
                  title="Failures and cancels"
                  value={`${metrics?.totals.failed ?? 0} / ${metrics?.totals.canceled ?? 0}`}
                  description={`失败�?${formatPercent(metrics?.rates.failureRate)}`}
                  icon={<TriangleAlert className="h-4 w-4" />}
                />
                <MetricsStatCard
                  title="累计重试次数"
                  value={String(metrics?.totals.retries ?? 0)}
                  description={`累计提交 ${metrics?.totals.submitted ?? 0} 个任务`}
                  icon={<RotateCcw className="h-4 w-4" />}
                />
              </div>

              <div className="rounded-xl border border-border/70">
                <div className="flex items-center justify-between border-b border-border/70 px-4 py-3">
                  <div>
                    <h3 className="text-sm font-semibold text-foreground">节点指标</h3>
                    <p className="text-xs text-muted-foreground">
                      按节点类型聚合运行次数、重试次数与耗时
                    </p>
                  </div>
                  <Badge variant="outline">
                    <TimerReset className="mr-1 h-3.5 w-3.5" />
                    10s 自动刷新
                  </Badge>
                </div>
                {metricsLoading && !metrics ? (
                  <div className="py-10 text-center text-muted-foreground">加载指标�?..</div>
                ) : !metrics?.nodes?.length ? (
                  <div className="py-10 text-center text-muted-foreground">暂无节点指标</div>
                ) : (
                  <Table className="min-w-[900px]">
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-[140px]">节点类型</TableHead>
                        <TableHead className="w-[90px]">运行次数</TableHead>
                        <TableHead className="w-[90px]">成功</TableHead>
                        <TableHead className="w-[90px]">失败</TableHead>
                        <TableHead className="w-[90px]">重试</TableHead>
                        <TableHead className="w-[130px]">平均耗时</TableHead>
                        <TableHead className="w-[130px]">最大耗时</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {metrics.nodes.map((node) => (
                        <NodeMetricsRow key={node.nodeType} node={node} />
                      ))}
                    </TableBody>
                  </Table>
                )}
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <div className="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
                <div>
                  <CardTitle>通道任务</CardTitle>
                  <CardDescription>监控执行状态与节点日志</CardDescription>
                </div>
                <div className="flex flex-1 flex-wrap items-center justify-end gap-2">
                  <Select
                    value={taskStatus || "all"}
                    onValueChange={(value) => {
                      setTaskPageNo(1);
                      setTaskStatus(value === "all" ? undefined : value);
                    }}
                  >
                    <SelectTrigger className="w-[180px]">
                      <SelectValue placeholder="Task status" />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="all">All statuses</SelectItem>
                      {STATUS_OPTIONS.map((option) => (
                        <SelectItem key={option.value} value={option.value}>
                          {option.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <Button variant="outline" onClick={handleTaskRefresh}>
                    <RefreshCw className="mr-2 h-4 w-4" />
                    刷新
                  </Button>
                  <Button variant="outline" onClick={() => setUploadDialogOpen(true)}>
                    <FileUp className="mr-2 h-4 w-4" />
                    上传文件
                  </Button>
                  <Button
                    className="admin-primary-gradient"
                    onClick={() => setTaskDialogOpen(true)}
                  >
                    <Plus className="mr-2 h-4 w-4" />
                    新建任务
                  </Button>
                </div>
              </div>
            </CardHeader>
            <CardContent>
              {taskLoading ? (
                <div className="py-10 text-center text-muted-foreground">加载�?..</div>
              ) : tasks.length === 0 ? (
                <div className="py-10 text-center text-muted-foreground">暂无任务</div>
              ) : (
                <Table className="min-w-[980px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-[220px]">任务ID</TableHead>
                      <TableHead>来源</TableHead>
                      <TableHead className="w-[120px]">Status</TableHead>
                      <TableHead className="w-[120px]">Owner</TableHead>
                      <TableHead className="w-[90px]">Chunks</TableHead>
                      <TableHead className="w-[170px]">创建时间</TableHead>
                      <TableHead className="w-[140px] text-left">操作</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {tasks.map((task) => (
                      <TableRow key={task.id}>
                        <TableCell className="font-mono text-xs">{task.id}</TableCell>
                        <TableCell>
                          <div className="text-sm">
                            <span className="font-medium">{task.sourceType || "-"}</span>
                            <span className="text-muted-foreground">
                              {" "}
                              {task.sourceFileName || task.sourceLocation || ""}
                            </span>
                          </div>
                        </TableCell>
                        <TableCell>
                          <Badge variant={statusBadgeVariant(task.status)}>
                            {taskStatusLabel(task.status)}
                          </Badge>
                        </TableCell>
                        <TableCell>{task.createdBy || "-"}</TableCell>
                        <TableCell>{task.chunkCount ?? "-"}</TableCell>
                        <TableCell>{formatDate(task.createTime)}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            size="sm"
                            variant="outline"
                            onClick={() => setTaskDetail({ open: true, taskId: task.id })}
                          >
                            查看详情
                          </Button>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}

              <Pagination
                current={taskPage?.current || 1}
                pages={taskPage?.pages || 1}
                total={taskPage?.total || 0}
                onPrev={() => setTaskPageNo((prev) => Math.max(1, prev - 1))}
                onNext={() => setTaskPageNo((prev) => Math.min(taskPage?.pages || 1, prev + 1))}
              />
            </CardContent>
          </Card>
        </div>
      )}

      <PipelineDialog
        contracts={pipelineContracts}
        open={pipelineDialog.open}
        mode={pipelineDialog.mode}
        pipeline={pipelineDialog.pipeline}
        onOpenChange={(open) => setPipelineDialog((prev) => ({ ...prev, open }))}
        onSubmit={async (payload, mode) => {
          if (mode === "create") {
            await createIngestionPipeline(payload);
            toast.success("创建成功");
          } else if (pipelineDialog.pipeline) {
            await updateIngestionPipeline(pipelineDialog.pipeline.id, payload);
            toast.success("更新成功");
          }
          setPipelineDialog({ open: false, mode: "create", pipeline: null });
          await loadPipelines(1, pipelineKeyword);
          await loadPipelineOptions();
        }}
      />

      <PipelineNodesDialog
        open={pipelineNodesDialog.open}
        pipeline={pipelineNodesDialog.pipeline}
        onOpenChange={(open) =>
          setPipelineNodesDialog({ open, pipeline: open ? pipelineNodesDialog.pipeline : null })
        }
      />

      <AlertDialog
        open={Boolean(pipelineDeleteTarget)}
        onOpenChange={(open) => (!open ? setPipelineDeleteTarget(null) : null)}
      >
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除流水线？</AlertDialogTitle>
            <AlertDialogDescription>
              流水�?[{pipelineDeleteTarget?.name}] 将被永久删除�?            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={handlePipelineDelete}
              className="bg-destructive text-destructive-foreground"
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <TaskDialog
        open={taskDialogOpen}
        pipelineOptions={pipelineOptions}
        onOpenChange={setTaskDialogOpen}
        onSubmit={async (payload) => {
          const result = await createIngestionTask(payload);
          toast.success(`任务已创建：${result.taskId}`);
          setTaskDialogOpen(false);
          await loadTasks(1, taskStatus);
        }}
        onUpload={async (pipelineId, file) => {
          const result = await uploadIngestionTask(pipelineId, file);
          toast.success(`上传成功�?{result.taskId}`);
          setTaskDialogOpen(false);
          await loadTasks(1, taskStatus);
        }}
      />

      <UploadDialog
        open={uploadDialogOpen}
        pipelineOptions={pipelineOptions}
        onOpenChange={setUploadDialogOpen}
        onSubmit={async (pipelineId, file) => {
          const result = await uploadIngestionTask(pipelineId, file);
          toast.success(`上传成功�?{result.taskId}`);
          setUploadDialogOpen(false);
          await loadTasks(1, taskStatus);
        }}
      />

      <TaskDetailDialog
        open={taskDetail.open}
        taskId={taskDetail.taskId}
        onOpenChange={(open) => setTaskDetail({ open, taskId: open ? taskDetail.taskId : null })}
      />
    </div>
  );
}

interface MetricsStatCardProps {
  title: string;
  value: string;
  description: string;
  icon: ReactNode;
}

function MetricsStatCard({ title, value, description, icon }: MetricsStatCardProps) {
  return (
    <div className="rounded-xl border border-border/70 bg-card px-4 py-4 shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="space-y-1">
          <p className="text-xs font-medium uppercase tracking-[0.16em] text-muted-foreground">
            {title}
          </p>
          <p className="text-2xl font-semibold text-foreground">{value}</p>
        </div>
        <div className="rounded-full border border-border/70 bg-muted/60 p-2 text-muted-foreground">
          {icon}
        </div>
      </div>
      <p className="mt-3 text-xs text-muted-foreground">{description}</p>
    </div>
  );
}

function NodeMetricsRow({ node }: { node: IngestionNodeMetrics }) {
  return (
    <TableRow>
      <TableCell>
        <Badge variant="outline">{node.nodeType}</Badge>
      </TableCell>
      <TableCell>{node.runs}</TableCell>
      <TableCell>{node.successes}</TableCell>
      <TableCell>{node.failures}</TableCell>
      <TableCell>{node.retries}</TableCell>
      <TableCell>{formatDuration(node.avgDurationMs)}</TableCell>
      <TableCell>{formatDuration(node.maxDurationMs)}</TableCell>
    </TableRow>
  );
}

interface PaginationProps {
  current: number;
  pages: number;
  total: number;
  onPrev: () => void;
  onNext: () => void;
}

function Pagination({ current, pages, total, onPrev, onNext }: PaginationProps) {
  if (total === 0) return null;
  return (
    <div className="mt-4 flex flex-wrap items-center justify-between gap-2 text-sm text-slate-500">
      <span>Total {total}</span>
      <div className="flex items-center gap-2">
        <Button variant="outline" size="sm" onClick={onPrev} disabled={current <= 1}>
          上一�?        </Button>
        <span>
          {current} / {pages}
        </span>
        <Button variant="outline" size="sm" onClick={onNext} disabled={current >= pages}>
          下一�?        </Button>
      </div>
    </div>
  );
}

interface PipelineDialogProps {
  contracts: IngestionNodeContract[];
  open: boolean;
  mode: "create" | "edit";
  pipeline: IngestionPipeline | null;
  onOpenChange: (open: boolean) => void;
  onSubmit: (payload: IngestionPipelinePayload, mode: "create" | "edit") => Promise<void>;
}

interface PipelineNodeContractPreview {
  availableArtifacts: string[];
  missingRequirements: string[];
  entryError?: string;
}

const summarizeContractRequirement = (requirement: IngestionNodeRequirement) => {
  if (requirement.description?.trim()) {
    return requirement.description.trim();
  }
  return requirement.anyOf.join(" / ");
};

const buildNodeContractPreview = (
  definition: IngestionPipelineDefinition | null | undefined,
  contracts: IngestionNodeContract[]
) => {
  const previews = new Map<string, PipelineNodeContractPreview>();
  if (!definition?.nodes?.length) return previews;

  const contractByType = new Map(contracts.map((contract) => [contract.nodeType, contract]));
  const adjacency = new Map<string, string[]>();
  const predecessors = new Map<string, string[]>();
  const inDegree = new Map<string, number>();
  const nodeById = new Map(definition.nodes.map((node) => [node.nodeId, node]));
  const availableAfter = new Map<string, Set<string>>();
  const entryNodeIDs = new Set(definition.entryNodeIds || []);

  for (const node of definition.nodes) {
    adjacency.set(node.nodeId, []);
    predecessors.set(node.nodeId, []);
    inDegree.set(node.nodeId, 0);
  }

  for (const edge of definition.edges || []) {
    if (!nodeById.has(edge.fromNodeId) || !nodeById.has(edge.toNodeId)) continue;
    adjacency.set(edge.fromNodeId, [...(adjacency.get(edge.fromNodeId) || []), edge.toNodeId]);
    predecessors.set(edge.toNodeId, [...(predecessors.get(edge.toNodeId) || []), edge.fromNodeId]);
    inDegree.set(edge.toNodeId, (inDegree.get(edge.toNodeId) || 0) + 1);
  }

  const queue = definition.nodes
    .map((node) => node.nodeId)
    .filter((nodeId) => (inDegree.get(nodeId) || 0) === 0);

  while (queue.length > 0) {
    const nodeID = queue.shift();
    if (!nodeID) continue;
    const node = nodeById.get(nodeID);
    if (!node) continue;

    const availableBefore = new Set<string>();
    if (entryNodeIDs.has(nodeID)) {
      availableBefore.add("task");
    }
    for (const predecessorNodeID of predecessors.get(nodeID) || []) {
      for (const artifact of availableAfter.get(predecessorNodeID) || []) {
        availableBefore.add(artifact);
      }
    }

    const contract = contractByType.get(node.nodeType);
    const missingRequirements =
      contract?.requires
        ?.filter((requirement) => !requirement.anyOf.some((artifact) => availableBefore.has(artifact)))
        .map(summarizeContractRequirement) || [];
    const entryError =
      contract && entryNodeIDs.has(nodeID) && !contract.supportsEntry
        ? `${contract.displayName || contract.nodeType} cannot be used as an entry node.`
        : undefined;

    const availableNext = new Set(availableBefore);
    for (const artifact of contract?.produces || []) {
      availableNext.add(artifact);
    }
    availableAfter.set(nodeID, availableNext);
    previews.set(nodeID, {
      availableArtifacts: Array.from(availableBefore).sort(),
      missingRequirements,
      entryError
    });

    for (const nextNodeID of adjacency.get(nodeID) || []) {
      const nextDegree = (inDegree.get(nextNodeID) || 0) - 1;
      inDegree.set(nextNodeID, nextDegree);
      if (nextDegree === 0) {
        queue.push(nextNodeID);
      }
    }
  }

  return previews;
};

function PipelineDialog({
  contracts,
  open,
  mode,
  pipeline,
  onOpenChange,
  onSubmit
}: PipelineDialogProps) {
  const [saving, setSaving] = useState(false);
  const [nodeMode, setNodeMode] = useState<"form" | "json">("form");
  const [nodes, setNodes] = useState<PipelineNodeForm[]>([]);
  const defaultNodes = pipeline?.definition
    ? JSON.stringify(pipeline.definition, null, 2)
    : pipeline?.nodes?.length
      ? JSON.stringify(buildDefinitionFromLegacyNodes(pipeline.nodes), null, 2)
      : "";

  const form = useForm<PipelineFormValues>({
    resolver: zodResolver(pipelineSchema),
    defaultValues: {
      name: pipeline?.name || "",
      description: pipeline?.description || "",
      nodesJson: defaultNodes
    }
  });

  const createLocalId = () => `${Date.now()}-${Math.random().toString(16).slice(2, 8)}`;

  const createTask = (type: string) => ({
    id: createLocalId(),
    type,
    systemPrompt: "",
    userPromptTemplate: ""
  });

  const createNode = (nodeType: PipelineNodeType = "fetcher"): PipelineNodeForm => ({
    id: createLocalId(),
    nodeId: "",
    nodeType,
    nextNodeId: "",
    condition: "",
    chunker: {
      strategy: "structure_aware",
      chunkSize: "",
      overlapSize: "",
      separator: ""
    },
    enhancer: {
      modelId: "",
      tasks: []
    },
    enricher: {
      modelId: "",
      attachDocumentMetadata: true,
      tasks: []
    },
    parser: {
      rulesJson: ""
    },
    indexer: {
      embeddingModel: "",
      metadataFields: ""
    }
  });

  const mapSettingsTasks = (tasks: unknown): EnhancerTaskForm[] => {
    if (!Array.isArray(tasks)) return [];
    return tasks.map((task) => ({
      id: createLocalId(),
      type: String((task as { type?: string }).type || ""),
      systemPrompt: String((task as { systemPrompt?: string }).systemPrompt || ""),
      userPromptTemplate: String((task as { userPromptTemplate?: string }).userPromptTemplate || "")
    }));
  };

  const buildNodeForm = (node: IngestionPipelineNode): PipelineNodeForm => {
    const settings = (node.settings as Record<string, unknown>) || {};
    const rawCondition = node.condition as unknown;
    const condition = rawCondition
      ? typeof rawCondition === "string"
        ? rawCondition
        : JSON.stringify(rawCondition, null, 2)
      : "";
    const tasks = mapSettingsTasks((settings as { tasks?: unknown }).tasks);
    const nodeType = (node.nodeType as PipelineNodeType) || "fetcher";
    return {
      id: createLocalId(),
      nodeId: node.nodeId || "",
      nodeType,
      nextNodeId: node.nextNodeId || "",
      condition,
      chunker: {
        strategy: String((settings as { strategy?: string }).strategy || "structure_aware"),
        chunkSize: settings.chunkSize != null ? String(settings.chunkSize) : "",
        overlapSize: settings.overlapSize != null ? String(settings.overlapSize) : "",
        separator: String((settings as { separator?: string }).separator || "")
      },
      enhancer: {
        modelId: String((settings as { modelId?: string }).modelId || ""),
        tasks: nodeType === "enhancer" ? tasks : []
      },
      enricher: {
        modelId: String((settings as { modelId?: string }).modelId || ""),
        attachDocumentMetadata:
          (settings as { attachDocumentMetadata?: boolean }).attachDocumentMetadata ?? true,
        tasks: nodeType === "enricher" ? tasks : []
      },
      parser: {
        rulesJson: Array.isArray((settings as { rules?: unknown }).rules)
          ? JSON.stringify((settings as { rules?: unknown }).rules, null, 2)
          : ""
      },
      indexer: {
        embeddingModel: String((settings as { embeddingModel?: string }).embeddingModel || ""),
        metadataFields: Array.isArray((settings as { metadataFields?: string[] }).metadataFields)
          ? (settings as { metadataFields?: string[] }).metadataFields?.join(", ") || ""
          : ""
      }
    };
  };

  const buildNodesFromPipeline = (source?: IngestionPipelineNode[] | null) => {
    if (!source || source.length === 0) return [];
    return source.map(buildNodeForm);
  };

  const buildNodesFromDefinition = (definition?: IngestionPipelineDefinition | null) => {
    if (!definition?.nodes?.length) return [];
    const firstEdgeBySource = new Map<string, { nextNodeId: string; condition: unknown }>();
    for (const edge of definition.edges || []) {
      if (!firstEdgeBySource.has(edge.fromNodeId)) {
        firstEdgeBySource.set(edge.fromNodeId, {
          nextNodeId: edge.toNodeId,
          condition: edge.condition ?? null
        });
      }
    }
    return definition.nodes.map((node) => {
      const next = firstEdgeBySource.get(node.nodeId);
      const formNode = buildNodeForm(node);
      formNode.nextNodeId = next?.nextNodeId || "";
      formNode.condition = next?.condition ? JSON.stringify(next.condition, null, 2) : "";
      return formNode;
    });
  };

  const isComplexDefinition = (definition?: IngestionPipelineDefinition | null) => {
    if (!definition?.nodes?.length) return false;
    const outgoingCount = new Map<string, number>();
    for (const edge of definition.edges || []) {
      outgoingCount.set(edge.fromNodeId, (outgoingCount.get(edge.fromNodeId) || 0) + 1);
    }
    for (const count of outgoingCount.values()) {
      if (count > 1) return true;
    }
    return false;
  };

  const parseCondition = (raw: string) => {
    const trimmed = raw.trim();
    if (!trimmed) return null;
    if (trimmed.startsWith("{") || trimmed.startsWith("[")) {
      return JSON.parse(trimmed);
    }
    return trimmed;
  };

  const parseParserRules = (raw: string) => {
    const trimmed = raw.trim();
    if (!trimmed) return null;
    const parsed = JSON.parse(trimmed);
    if (Array.isArray(parsed)) {
      return { rules: parsed };
    }
    if (
      parsed &&
      typeof parsed === "object" &&
      Array.isArray((parsed as { rules?: unknown }).rules)
    ) {
      return { rules: (parsed as { rules?: unknown }).rules };
    }
    throw new Error("Parser rules must be an array or an object with a rules field");
  };

  const buildSettings = (node: PipelineNodeForm) => {
    switch (node.nodeType) {
      case "chunker": {
        if (!node.chunker.strategy) {
          throw new Error("分块节点需要选择 strategy");
        }
        const chunkSize = node.chunker.chunkSize.trim();
        const overlapSize = node.chunker.overlapSize.trim();
        const chunkSizeValue = chunkSize ? Number(chunkSize) : undefined;
        const overlapSizeValue = overlapSize ? Number(overlapSize) : undefined;
        if (chunkSizeValue !== undefined && Number.isNaN(chunkSizeValue)) {
          throw new Error("chunkSize must be a number");
        }
        if (overlapSizeValue !== undefined && Number.isNaN(overlapSizeValue)) {
          throw new Error("overlapSize must be a number");
        }
        return {
          strategy: node.chunker.strategy,
          chunkSize: chunkSizeValue,
          overlapSize: overlapSizeValue,
          separator: node.chunker.separator.trim() || undefined
        };
      }
      case "enhancer": {
        const tasks = node.enhancer.tasks
          .filter((task) => task.type)
          .map((task) => ({
            type: task.type,
            systemPrompt: task.systemPrompt.trim() || undefined,
            userPromptTemplate: task.userPromptTemplate.trim() || undefined
          }));
        const payload: Record<string, unknown> = {};
        if (node.enhancer.modelId.trim()) {
          payload.modelId = node.enhancer.modelId.trim();
        }
        if (tasks.length > 0) {
          payload.tasks = tasks;
        }
        return Object.keys(payload).length ? payload : undefined;
      }
      case "enricher": {
        const tasks = node.enricher.tasks
          .filter((task) => task.type)
          .map((task) => ({
            type: task.type,
            systemPrompt: task.systemPrompt.trim() || undefined,
            userPromptTemplate: task.userPromptTemplate.trim() || undefined
          }));
        const payload: Record<string, unknown> = {
          attachDocumentMetadata: node.enricher.attachDocumentMetadata
        };
        if (node.enricher.modelId.trim()) {
          payload.modelId = node.enricher.modelId.trim();
        }
        if (tasks.length > 0) {
          payload.tasks = tasks;
        }
        return payload;
      }
      case "parser": {
        if (!node.parser.rulesJson.trim()) {
          return undefined;
        }
        return parseParserRules(node.parser.rulesJson);
      }
      case "indexer": {
        const fields = node.indexer.metadataFields
          .split(",")
          .map((item) => item.trim())
          .filter(Boolean);
        const payload: Record<string, unknown> = {};
        if (node.indexer.embeddingModel.trim()) {
          payload.embeddingModel = node.indexer.embeddingModel.trim();
        }
        if (fields.length > 0) {
          payload.metadataFields = fields;
        }
        return Object.keys(payload).length ? payload : undefined;
      }
      case "fetcher":
      default:
        return undefined;
    }
  };

  const buildNodesPayload = (sourceNodes: PipelineNodeForm[]) => {
    const result: IngestionPipelinePayload["nodes"] = [];
    for (const node of sourceNodes) {
      const nodeId = node.nodeId.trim();
      if (!nodeId) {
        return { ok: false as const, message: "节点ID不能为空" };
      }
      if (!node.nodeType) {
        return { ok: false as const, message: "节点类型不能为空" };
      }
      let settings: Record<string, unknown> | undefined;
      let condition: unknown;
      try {
        settings = buildSettings(node) as Record<string, unknown> | undefined;
        condition = parseCondition(node.condition);
      } catch (error) {
        return {
          ok: false as const,
          message: error instanceof Error ? error.message : "节点配置错误"
        };
      }
      result.push({
        nodeId,
        nodeType: node.nodeType,
        settings: settings ?? null,
        condition: condition ?? null,
        nextNodeId: node.nextNodeId.trim() || null
      });
    }
    return { ok: true as const, nodes: result };
  };

  const buildDefinitionPayload = (sourceNodes: PipelineNodeForm[]) => {
    const result = buildNodesPayload(sourceNodes);
    if (!result.ok) return result;
    return {
      ok: true as const,
      definition: buildDefinitionFromLegacyNodes(result.nodes as IngestionPipelineNode[])
    };
  };

  const parseDefinitionJson = (raw: string | undefined) => {
    if (!raw || !raw.trim()) {
      return { ok: true as const, definition: null as IngestionPipelineDefinition | null };
    }
    try {
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        return { ok: false as const, message: "definition must be a JSON object" };
      }
      const definition = parsed as IngestionPipelineDefinition;
      if (!Array.isArray(definition.nodes) || !Array.isArray(definition.edges)) {
        return { ok: false as const, message: "definition must include nodes and edges arrays" };
      }
      return { ok: true as const, definition };
    } catch (error) {
      return { ok: false as const, message: "definition JSON 鏍煎紡閿欒" };
    }
  };

  const parseNodesJson = (raw: string | undefined) => {
    if (!raw || !raw.trim()) {
      return { ok: true as const, nodes: [] };
    }
    try {
      const parsed = JSON.parse(raw);
      if (!Array.isArray(parsed)) {
        return { ok: false as const, message: "节点配置必须是JSON数组" };
      }
      const nodesForm = parsed.map((item) => buildNodeForm(item as IngestionPipelineNode));
      return { ok: true as const, nodes: nodesForm };
    } catch (error) {
      return { ok: false as const, message: "节点配置JSON格式错误" };
    }
  };

  const parseGraphDefinitionJson = (raw: string | undefined) => {
    if (!raw || !raw.trim()) {
      return { ok: true as const, definition: null as IngestionPipelineDefinition | null };
    }
    try {
      const parsed = JSON.parse(raw);
      if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
        return { ok: false as const, message: "definition 必须�?JSON 对象" };
      }
      const definition = parsed as IngestionPipelineDefinition;
      if (!Array.isArray(definition.entryNodeIds)) {
        return { ok: false as const, message: "definition 必须包含 entryNodeIds 数组" };
      }
      if (!Array.isArray(definition.nodes) || !Array.isArray(definition.edges)) {
        return { ok: false as const, message: "definition 必须包含 nodes �?edges 数组" };
      }
      return { ok: true as const, definition };
    } catch (error) {
      return { ok: false as const, message: "definition JSON 格式错误" };
    }
  };

  void parseDefinitionJson;
  void parseNodesJson;

  const switchMode = (nextMode: "form" | "json") => {
    if (nextMode === nodeMode) return;
    if (nextMode === "json") {
      const result = buildDefinitionPayload(nodes);
      if (!result.ok) {
        toast.error(result.message);
        return;
      }
      form.setValue("nodesJson", JSON.stringify(result.definition, null, 2));
      setNodeMode("json");
      return;
    }
    const parsed = parseGraphDefinitionJson(form.getValues("nodesJson"));
    if (!parsed.ok) {
      toast.error(parsed.message);
      return;
    }
    setNodes(buildNodesFromDefinition(parsed.definition));
    setNodeMode("form");
  };

  useEffect(() => {
    if (open) {
      const definition = pipeline?.definition
        ? pipeline.definition
        : pipeline?.nodes?.length
          ? buildDefinitionFromLegacyNodes(pipeline.nodes)
          : null;
      form.reset({
        name: pipeline?.name || "",
        description: pipeline?.description || "",
        nodesJson: defaultNodes
      });
      setNodes(definition ? buildNodesFromDefinition(definition) : buildNodesFromPipeline(pipeline?.nodes));
      setNodeMode(isComplexDefinition(definition) ? "json" : "form");
    }
  }, [open, pipeline, defaultNodes, form]);

  const handleSubmit = async (values: PipelineFormValues) => {
    let definitionPayload: IngestionPipelinePayload["definition"] | undefined;
    let nodesPayload: IngestionPipelinePayload["nodes"] | undefined;
    if (nodeMode === "json") {
      const parsed = parseGraphDefinitionJson(values.nodesJson);
      if (!parsed.ok) {
        form.setError("nodesJson", { message: parsed.message });
        return;
      }
      definitionPayload = parsed.definition ?? undefined;
      nodesPayload = definitionPayload?.nodes;
      /*
            form.setError("nodesJson", { message: "节点配置必须是JSON数组" });
            return;
          }
            form.setError("nodesJson", { message: "每个节点必须包含 nodeId �?nodeType" });
            return;
          }
          form.setError("nodesJson", { message: "节点配置JSON格式错误" });
          return;
        }
      }
      */
    } else {
      const result = buildDefinitionPayload(nodes);
      if (!result.ok) {
        toast.error(result.message);
        return;
      }
      definitionPayload = result.definition;
      nodesPayload = result.definition.nodes;
    }

    setSaving(true);
    try {
      const payload: IngestionPipelinePayload = {
        name: values.name.trim(),
        description: values.description?.trim() || undefined,
        definition: definitionPayload,
        nodes: nodesPayload
      };
      await onSubmit(payload, mode);
    } catch (error) {
      toast.error(getErrorMessage(error, mode === "create" ? "创建失败" : "更新失败"));
      console.error(error);
    } finally {
      setSaving(false);
    }
  };

  const contractByType = new Map(contracts.map((contract) => [contract.nodeType, contract]));
  const previewResult = nodeMode === "form" ? buildDefinitionPayload(nodes) : null;
  const nodePreviews =
    nodeMode === "form" && previewResult?.ok
      ? buildNodeContractPreview(previewResult.definition, contracts)
      : new Map<string, PipelineNodeContractPreview>();

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sidebar-scroll sm:max-w-[880px]">
        <DialogHeader>
          <DialogTitle>{mode === "create" ? "新建流水�? : "编辑流水�?}</DialogTitle>
          <DialogDescription>配置节点顺序与处理逻辑</DialogDescription>
        </DialogHeader>

        <Form {...form}>
          <form className="space-y-4" onSubmit={form.handleSubmit(handleSubmit)}>
            <FormField
              control={form.control}
              name="name"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Pipeline name</FormLabel>
                  <FormControl>
                    <Input placeholder="例如：通用文档通道" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <FormField
              control={form.control}
              name="description"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>描述</FormLabel>
                  <FormControl>
                    <Textarea placeholder="说明流水线用途或流程" {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="text-sm font-medium">节点配置</div>
              <div className="flex items-center gap-2">
                <Button
                  type="button"
                  size="sm"
                  variant={nodeMode === "form" ? "default" : "outline"}
                  onClick={() => switchMode("form")}
                >
                  表单配置
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant={nodeMode === "json" ? "default" : "outline"}
                  onClick={() => switchMode("json")}
                >
                  JSON配置
                </Button>
              </div>
            </div>

            <div className="rounded-lg border bg-muted/30 p-4 space-y-3">
              <div className="flex items-center gap-2">
                <Badge variant="secondary">Node Contracts</Badge>
                <span className="text-sm text-muted-foreground">
                  Review entry eligibility and the required / produced artifacts for each node type.
                </span>
              </div>
              {contracts.length === 0 ? (
                <div className="text-sm text-muted-foreground">
                  No node contracts are available right now.
                </div>
              ) : (
                <div className="grid gap-3 md:grid-cols-2">
                  {contracts.map((contract) => (
                    <div key={contract.nodeType} className="rounded-md border bg-background p-3">
                      <div className="flex items-center justify-between gap-2">
                        <div className="space-y-1">
                          <div className="text-sm font-medium">
                            {contract.displayName || contract.nodeType}
                          </div>
                          <div className="text-xs text-muted-foreground">{contract.summary}</div>
                        </div>
                        <Badge variant={contract.supportsEntry ? "default" : "outline"}>
                          {contract.supportsEntry ? "entry ok" : "non-entry"}
                        </Badge>
                      </div>
                      <div className="mt-3 space-y-2 text-xs text-muted-foreground">
                        <div>
                          <span className="font-medium text-foreground">Requires:</span>{" "}
                          {contract.requires?.length
                            ? contract.requires.map(summarizeContractRequirement).join(" | ")
                            : "None"}
                        </div>
                        <div>
                          <span className="font-medium text-foreground">Produces:</span>{" "}
                          {contract.produces?.length ? contract.produces.join(", ") : "None"}
                        </div>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>

            {nodeMode === "json" ? (
              <FormField
                control={form.control}
                name="nodesJson"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>Pipeline graph definition (JSON)</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder='{"version":"v1","entryNodeIds":["fetch"],"nodes":[{"nodeId":"fetch","nodeType":"fetcher","settings":{}}],"edges":[{"edgeId":"fetch__to__parse","fromNodeId":"fetch","toNodeId":"parse","priority":0}]}'
                        rows={10}
                        {...field}
                      />
                    </FormControl>
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : (
              <div className="space-y-4">
                {nodes.length === 0 ? (
                  <div className="rounded-lg border border-dashed p-6 text-center text-sm text-muted-foreground">
                    暂无节点，请添加节点配置
                  </div>
                ) : null}

                {nodes.map((node, index) => (
                  <div key={node.id} className="rounded-lg border p-4 space-y-4">
                    {(() => {
                      const contract = contractByType.get(node.nodeType);
                      const preview = nodePreviews.get(node.nodeId.trim());
                      return (
                        <div className="rounded-md border bg-muted/30 p-3 space-y-2">
                          <div className="flex flex-wrap items-center gap-2">
                            <Badge variant="secondary">
                              {contract?.displayName || node.nodeType}
                            </Badge>
                            <Badge variant="outline">
                              {contract?.supportsEntry ? "supports entry" : "non-entry"}
                            </Badge>
                          </div>
                          {contract?.summary ? (
                            <div className="text-sm text-muted-foreground">{contract.summary}</div>
                          ) : null}
                          <div className="grid gap-2 text-xs md:grid-cols-2">
                            <div>
                              <span className="font-medium text-foreground">Requires:</span>{" "}
                              {contract?.requires?.length
                                ? contract.requires.map(summarizeContractRequirement).join(" | ")
                                : "None"}
                            </div>
                            <div>
                              <span className="font-medium text-foreground">Produces:</span>{" "}
                              {contract?.produces?.length
                                ? contract.produces.join(", ")
                                : "None"}
                            </div>
                          </div>
                          {preview ? (
                            <div className="grid gap-2 text-xs md:grid-cols-2">
                              <div>
                                <span className="font-medium text-foreground">
                                  Available before node:
                                </span>{" "}
                                {preview.availableArtifacts.length
                                  ? preview.availableArtifacts.join(", ")
                                  : "None"}
                              </div>
                              <div>
                                <span className="font-medium text-foreground">
                                  Missing requirements:
                                </span>{" "}
                                {preview.missingRequirements.length
                                  ? preview.missingRequirements.join(" | ")
                                  : "None"}
                              </div>
                            </div>
                          ) : null}
                          {preview?.entryError || preview?.missingRequirements.length ? (
                            <div className="flex gap-2 rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900">
                              <TriangleAlert className="mt-0.5 h-4 w-4 shrink-0" />
                              <div className="space-y-1">
                                {preview.entryError ? <div>{preview.entryError}</div> : null}
                                {preview.missingRequirements.length ? (
                                  <div>Current form graph does not provide all required artifacts.</div>
                                ) : null}
                              </div>
                            </div>
                          ) : null}
                        </div>
                      );
                    })()}
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-2">
                        <Badge variant="outline">{node.nodeType}</Badge>
                        <span className="text-sm text-muted-foreground">节点 {index + 1}</span>
                      </div>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        className="text-destructive hover:text-destructive"
                        onClick={() =>
                          setNodes((prev) => prev.filter((item) => item.id !== node.id))
                        }
                      >
                        删除
                      </Button>
                    </div>

                    <div className="grid gap-4 md:grid-cols-2">
                      <div className="space-y-2">
                        <label className="text-sm font-medium">节点ID</label>
                        <Input
                          value={node.nodeId}
                          onChange={(event) =>
                            setNodes((prev) =>
                              prev.map((item) =>
                                item.id === node.id ? { ...item, nodeId: event.target.value } : item
                              )
                            )
                          }
                          placeholder="例如：fetch"
                        />
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">节点类型</label>
                        <Select
                          value={node.nodeType}
                          onValueChange={(value) =>
                            setNodes((prev) =>
                              prev.map((item) =>
                                item.id === node.id
                                  ? { ...item, nodeType: value as PipelineNodeType }
                                  : item
                              )
                            )
                          }
                        >
                          <SelectTrigger>
                            <SelectValue placeholder="选择节点类型" />
                          </SelectTrigger>
                          <SelectContent>
                            {NODE_TYPE_OPTIONS.map((option) => (
                              <SelectItem key={option.value} value={option.value}>
                                {option.label}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      <div className="space-y-2">
                        <label className="text-sm font-medium">下一节点ID</label>
                        <Input
                          value={node.nextNodeId}
                          onChange={(event) =>
                            setNodes((prev) =>
                              prev.map((item) =>
                                item.id === node.id
                                  ? { ...item, nextNodeId: event.target.value }
                                  : item
                              )
                            )
                          }
                          placeholder="例如：parse"
                        />
                      </div>
                    </div>

                    {node.nodeType === "fetcher" ? (
                      <div className="rounded-lg bg-muted/50 p-3 text-sm text-muted-foreground">
                        Fetcher 无额外配�?                      </div>
                    ) : null}

                    {node.nodeType === "parser" ? (
                      <div className="space-y-2">
                        <label className="text-sm font-medium">Parser rules (JSON)</label>
                        <Textarea
                          rows={5}
                          value={node.parser.rulesJson}
                          onChange={(event) =>
                            setNodes((prev) =>
                              prev.map((item) =>
                                item.id === node.id
                                  ? {
                                      ...item,
                                      parser: { ...item.parser, rulesJson: event.target.value }
                                    }
                                  : item
                              )
                            )
                          }
                          placeholder='[{"mimeType":"PDF","options":{}}]'
                        />
                      </div>
                    ) : null}

                    {node.nodeType === "chunker" ? (
                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="space-y-2">
                          <label className="text-sm font-medium">分块策略</label>
                          <Select
                            value={node.chunker.strategy}
                            onValueChange={(value) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? { ...item, chunker: { ...item.chunker, strategy: value } }
                                    : item
                                )
                              )
                            }
                          >
                            <SelectTrigger>
                              <SelectValue placeholder="选择策略" />
                            </SelectTrigger>
                            <SelectContent>
                              {CHUNK_STRATEGY_OPTIONS.map((option) => (
                                <SelectItem key={option.value} value={option.value}>
                                  {option.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </div>
                        <div className="space-y-2">
                          <label className="text-sm font-medium">Chunk Size</label>
                          <Input
                            type="number"
                            value={node.chunker.chunkSize}
                            onChange={(event) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        chunker: { ...item.chunker, chunkSize: event.target.value }
                                      }
                                    : item
                                )
                              )
                            }
                            placeholder="例如�?12"
                          />
                        </div>
                        <div className="space-y-2">
                          <label className="text-sm font-medium">Overlap Size</label>
                          <Input
                            type="number"
                            value={node.chunker.overlapSize}
                            onChange={(event) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        chunker: {
                                          ...item.chunker,
                                          overlapSize: event.target.value
                                        }
                                      }
                                    : item
                                )
                              )
                            }
                            placeholder="例如�?28"
                          />
                        </div>
                        <div className="space-y-2">
                          <label className="text-sm font-medium">自定义分隔符</label>
                          <Input
                            value={node.chunker.separator}
                            onChange={(event) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        chunker: { ...item.chunker, separator: event.target.value }
                                      }
                                    : item
                                )
                              )
                            }
                            placeholder="Optional"
                          />
                        </div>
                      </div>
                    ) : null}

                    {node.nodeType === "enhancer" ? (
                      <div className="space-y-4">
                        <div className="space-y-2">
                          <label className="text-sm font-medium">模型ID</label>
                          <Input
                            value={node.enhancer.modelId}
                            onChange={(event) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        enhancer: { ...item.enhancer, modelId: event.target.value }
                                      }
                                    : item
                                )
                              )
                            }
                            placeholder="Optional"
                          />
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-sm font-medium">增强任务</span>
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        enhancer: {
                                          ...item.enhancer,
                                          tasks: [
                                            ...item.enhancer.tasks,
                                            createTask("context_enhance")
                                          ]
                                        }
                                      }
                                    : item
                                )
                              )
                            }
                          >
                            <Plus className="mr-2 h-4 w-4" />
                            添加任务
                          </Button>
                        </div>
                        {node.enhancer.tasks.length === 0 ? (
                          <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                            暂无任务
                          </div>
                        ) : (
                          <div className="space-y-3">
                            {node.enhancer.tasks.map((task, taskIndex) => (
                              <div key={task.id} className="rounded-md border p-3 space-y-3">
                                <div className="flex items-center justify-between">
                                  <span className="text-xs text-muted-foreground">
                                    任务 {taskIndex + 1}
                                  </span>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="ghost"
                                    className="text-destructive hover:text-destructive"
                                    onClick={() =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enhancer: {
                                                  ...item.enhancer,
                                                  tasks: item.enhancer.tasks.filter(
                                                    (t) => t.id !== task.id
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                  >
                                    删除
                                  </Button>
                                </div>
                                <div className="space-y-2">
                                  <label className="text-sm font-medium">任务类型</label>
                                  <Select
                                    value={task.type}
                                    onValueChange={(value) =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enhancer: {
                                                  ...item.enhancer,
                                                  tasks: item.enhancer.tasks.map((t) =>
                                                    t.id === task.id ? { ...t, type: value } : t
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                  >
                                    <SelectTrigger>
                                      <SelectValue placeholder="选择类型" />
                                    </SelectTrigger>
                                    <SelectContent>
                                      {ENHANCER_TASK_OPTIONS.map((option) => (
                                        <SelectItem key={option.value} value={option.value}>
                                          {option.label}
                                        </SelectItem>
                                      ))}
                                    </SelectContent>
                                  </Select>
                                </div>
                                <div className="space-y-2">
                                  <label className="text-sm font-medium">System Prompt</label>
                                  <Textarea
                                    rows={2}
                                    value={task.systemPrompt}
                                    onChange={(event) =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enhancer: {
                                                  ...item.enhancer,
                                                  tasks: item.enhancer.tasks.map((t) =>
                                                    t.id === task.id
                                                      ? { ...t, systemPrompt: event.target.value }
                                                      : t
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                    placeholder="Optional"
                                  />
                                </div>
                                <div className="space-y-2">
                                  <label className="text-sm font-medium">User Prompt 模板</label>
                                  <Textarea
                                    rows={2}
                                    value={task.userPromptTemplate}
                                    onChange={(event) =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enhancer: {
                                                  ...item.enhancer,
                                                  tasks: item.enhancer.tasks.map((t) =>
                                                    t.id === task.id
                                                      ? {
                                                          ...t,
                                                          userPromptTemplate: event.target.value
                                                        }
                                                      : t
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                    placeholder="Optional"
                                  />
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ) : null}

                    {node.nodeType === "enricher" ? (
                      <div className="space-y-4">
                        <div className="grid gap-4 md:grid-cols-2">
                          <div className="space-y-2">
                            <label className="text-sm font-medium">模型ID</label>
                            <Input
                              value={node.enricher.modelId}
                              onChange={(event) =>
                                setNodes((prev) =>
                                  prev.map((item) =>
                                    item.id === node.id
                                      ? {
                                          ...item,
                                          enricher: {
                                            ...item.enricher,
                                            modelId: event.target.value
                                          }
                                        }
                                      : item
                                  )
                                )
                              }
                              placeholder="Optional"
                            />
                          </div>
                          <div className="space-y-2">
                            <label className="text-sm font-medium">Attach document metadata</label>
                            <Select
                              value={node.enricher.attachDocumentMetadata ? "true" : "false"}
                              onValueChange={(value) =>
                                setNodes((prev) =>
                                  prev.map((item) =>
                                    item.id === node.id
                                      ? {
                                          ...item,
                                          enricher: {
                                            ...item.enricher,
                                            attachDocumentMetadata: value === "true"
                                          }
                                        }
                                      : item
                                  )
                                )
                              }
                            >
                              <SelectTrigger>
                                <SelectValue placeholder="选择" />
                              </SelectTrigger>
                              <SelectContent>
                                <SelectItem value="true">Yes</SelectItem>
                                <SelectItem value="false">No</SelectItem>
                              </SelectContent>
                            </Select>
                          </div>
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-sm font-medium">富集任务</span>
                          <Button
                            type="button"
                            size="sm"
                            variant="outline"
                            onClick={() =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        enricher: {
                                          ...item.enricher,
                                          tasks: [...item.enricher.tasks, createTask("keywords")]
                                        }
                                      }
                                    : item
                                )
                              )
                            }
                          >
                            <Plus className="mr-2 h-4 w-4" />
                            添加任务
                          </Button>
                        </div>
                        {node.enricher.tasks.length === 0 ? (
                          <div className="rounded-md border border-dashed p-3 text-sm text-muted-foreground">
                            暂无任务
                          </div>
                        ) : (
                          <div className="space-y-3">
                            {node.enricher.tasks.map((task, taskIndex) => (
                              <div key={task.id} className="rounded-md border p-3 space-y-3">
                                <div className="flex items-center justify-between">
                                  <span className="text-xs text-muted-foreground">
                                    任务 {taskIndex + 1}
                                  </span>
                                  <Button
                                    type="button"
                                    size="sm"
                                    variant="ghost"
                                    className="text-destructive hover:text-destructive"
                                    onClick={() =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enricher: {
                                                  ...item.enricher,
                                                  tasks: item.enricher.tasks.filter(
                                                    (t) => t.id !== task.id
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                  >
                                    删除
                                  </Button>
                                </div>
                                <div className="space-y-2">
                                  <label className="text-sm font-medium">任务类型</label>
                                  <Select
                                    value={task.type}
                                    onValueChange={(value) =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enricher: {
                                                  ...item.enricher,
                                                  tasks: item.enricher.tasks.map((t) =>
                                                    t.id === task.id ? { ...t, type: value } : t
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                  >
                                    <SelectTrigger>
                                      <SelectValue placeholder="选择类型" />
                                    </SelectTrigger>
                                    <SelectContent>
                                      {ENRICHER_TASK_OPTIONS.map((option) => (
                                        <SelectItem key={option.value} value={option.value}>
                                          {option.label}
                                        </SelectItem>
                                      ))}
                                    </SelectContent>
                                  </Select>
                                </div>
                                <div className="space-y-2">
                                  <label className="text-sm font-medium">System Prompt</label>
                                  <Textarea
                                    rows={2}
                                    value={task.systemPrompt}
                                    onChange={(event) =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enricher: {
                                                  ...item.enricher,
                                                  tasks: item.enricher.tasks.map((t) =>
                                                    t.id === task.id
                                                      ? { ...t, systemPrompt: event.target.value }
                                                      : t
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                    placeholder="Optional"
                                  />
                                </div>
                                <div className="space-y-2">
                                  <label className="text-sm font-medium">User Prompt 模板</label>
                                  <Textarea
                                    rows={2}
                                    value={task.userPromptTemplate}
                                    onChange={(event) =>
                                      setNodes((prev) =>
                                        prev.map((item) =>
                                          item.id === node.id
                                            ? {
                                                ...item,
                                                enricher: {
                                                  ...item.enricher,
                                                  tasks: item.enricher.tasks.map((t) =>
                                                    t.id === task.id
                                                      ? {
                                                          ...t,
                                                          userPromptTemplate: event.target.value
                                                        }
                                                      : t
                                                  )
                                                }
                                              }
                                            : item
                                        )
                                      )
                                    }
                                    placeholder="Optional"
                                  />
                                </div>
                              </div>
                            ))}
                          </div>
                        )}
                      </div>
                    ) : null}

                    {node.nodeType === "indexer" ? (
                      <div className="grid gap-4 md:grid-cols-2">
                        <div className="space-y-2">
                          <label className="text-sm font-medium">Embedding 模型</label>
                          <Input
                            value={node.indexer.embeddingModel}
                            onChange={(event) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        indexer: {
                                          ...item.indexer,
                                          embeddingModel: event.target.value
                                        }
                                      }
                                    : item
                                )
                              )
                            }
                            placeholder="Optional"
                          />
                        </div>
                        <div className="space-y-2">
                          <label className="text-sm font-medium">Metadata fields</label>
                          <Input
                            value={node.indexer.metadataFields}
                            onChange={(event) =>
                              setNodes((prev) =>
                                prev.map((item) =>
                                  item.id === node.id
                                    ? {
                                        ...item,
                                        indexer: {
                                          ...item.indexer,
                                          metadataFields: event.target.value
                                        }
                                      }
                                    : item
                                )
                              )
                            }
                            placeholder="用逗号分隔，如 keywords,summary"
                          />
                        </div>
                      </div>
                    ) : null}

                    <div className="space-y-2">
                      <label className="text-sm font-medium">条件（JSON / SpEL，可选）</label>
                      <Textarea
                        rows={2}
                        value={node.condition}
                        onChange={(event) =>
                          setNodes((prev) =>
                            prev.map((item) =>
                              item.id === node.id
                                ? { ...item, condition: event.target.value }
                                : item
                            )
                          )
                        }
                        placeholder='{"field":"source_type","op":"eq","value":"file"} �?#context.source.type == "file"'
                      />
                    </div>
                  </div>
                ))}

                <Button
                  type="button"
                  variant="outline"
                  onClick={() => setNodes((prev) => [...prev, createNode()])}
                >
                  <Plus className="mr-2 h-4 w-4" />
                  添加节点
                </Button>
              </div>
            )}

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={saving}
              >
                取消
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? "保存�?.." : "保存"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

interface PipelineNodesDialogProps {
  open: boolean;
  pipeline: IngestionPipeline | null;
  onOpenChange: (open: boolean) => void;
}

function PipelineNodesDialog({ open, pipeline, onOpenChange }: PipelineNodesDialogProps) {
  const nodes = pipeline?.nodes || [];
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="max-h-[90vh] overflow-y-auto sidebar-scroll sm:max-w-[720px]"
        onOpenAutoFocus={(event) => event.preventDefault()}
      >
        <DialogHeader>
          <DialogTitle>Pipeline nodes</DialogTitle>
          <DialogDescription>{pipeline?.name || ""}</DialogDescription>
        </DialogHeader>
        {nodes.length === 0 ? (
          <div className="py-6 text-center text-muted-foreground">暂无节点</div>
        ) : (
          <Table className="min-w-[640px]">
            <TableHeader>
              <TableRow>
                <TableHead className="w-[60px]">#</TableHead>
                <TableHead className="w-[160px]">节点ID</TableHead>
                <TableHead className="w-[120px]">类型</TableHead>
                <TableHead className="w-[140px]">下一节点</TableHead>
                <TableHead>配置</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {nodes.map((node, index) => (
                <TableRow key={node.id || `${node.nodeId}-${index}`}>
                  <TableCell>{index + 1}</TableCell>
                  <TableCell className="font-mono text-xs">{node.nodeId}</TableCell>
                  <TableCell>
                    <Badge variant="outline">{node.nodeType}</Badge>
                  </TableCell>
                  <TableCell>{node.nextNodeId || "-"}</TableCell>
                  <TableCell>
                    <pre className="max-w-[280px] whitespace-pre-wrap text-xs text-muted-foreground">
                      {truncateJson(node.settings || node.condition)}
                    </pre>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
      </DialogContent>
    </Dialog>
  );
}

interface TaskDialogProps {
  open: boolean;
  pipelineOptions: IngestionPipeline[];
  onOpenChange: (open: boolean) => void;
  onSubmit: (payload: IngestionTaskCreatePayload) => Promise<void>;
  onUpload: (pipelineId: string, file: File) => Promise<void>;
}

function TaskDialog({ open, pipelineOptions, onOpenChange, onSubmit, onUpload }: TaskDialogProps) {
  const [saving, setSaving] = useState(false);
  const [localFile, setLocalFile] = useState<File | null>(null);
  const [maxFileSize, setMaxFileSize] = useState<number>(50 * 1024 * 1024);
  const form = useForm<TaskFormValues>({
    resolver: zodResolver(taskSchema),
    defaultValues: {
      pipelineId: pipelineOptions[0]?.id || "",
      sourceType: "file",
      location: "",
      fileName: "",
      credentialsJson: "",
      metadataJson: ""
    }
  });

  const sourceType = form.watch("sourceType");
  const isLocalFile = sourceType === "file";

  const sourceMeta = (() => {
    switch (sourceType) {
      case "file":
        return {
          locationPlaceholder: "/path/to/file �?file://path/to/file",
          locationHint: "支持本地文件路径�?file:// 协议",
          credentialsHint: ""
        };
      case "feishu":
        return {
          locationPlaceholder: "https://open.feishu.cn/...",
          locationHint: "填写飞书文档链接",
          credentialsHint: '{"tenantAccessToken":"..."} �?{"app_id":"...","app_secret":"..."}'
        };
      case "s3":
        return {
          locationPlaceholder: "s3://bucket/key",
          locationHint: "填写 S3 路径，例�?s3://biz/file.md",
          credentialsHint: ""
        };
      case "url":
      default:
        return {
          locationPlaceholder: "https://example.com/file.pdf",
          locationHint: "支持 http/https 链接",
          credentialsHint: '{"token":"xxx"} �?{"Authorization":"Bearer xxx"}'
        };
    }
  })();

  const showCredentials = sourceType === "url" || sourceType === "feishu";

  useEffect(() => {
    if (open) {
      form.reset({
        pipelineId: pipelineOptions[0]?.id || "",
        sourceType: "file",
        location: "",
        fileName: "",
        credentialsJson: "",
        metadataJson: ""
      });
      setLocalFile(null);
      getSystemSettings()
        .then((settings) => setMaxFileSize(settings.upload.maxFileSize))
        .catch(() => {});
    }
  }, [open, pipelineOptions, form]);

  useEffect(() => {
    if (!isLocalFile) {
      setLocalFile(null);
    }
  }, [isLocalFile]);

  const parseJsonField = (value?: string) => {
    if (!value || !value.trim()) return undefined;
    try {
      return JSON.parse(value);
    } catch {
      return null;
    }
  };

  const handleSubmit = async (values: TaskFormValues) => {
    if (values.sourceType === "file") {
      if (!localFile) {
        toast.error("请选择文件");
        return;
      }
      if (localFile.size > maxFileSize) {
        const sizeMB = Math.floor(maxFileSize / 1024 / 1024);
        toast.error(`上传文件大小超过限制，最大允�?${sizeMB}MB`);
        return;
      }
      setSaving(true);
      try {
        await onUpload(values.pipelineId, localFile);
      } catch (error) {
        toast.error(getErrorMessage(error, "上传失败"));
        console.error(error);
      } finally {
        setSaving(false);
      }
      return;
    }

    const credentials = parseJsonField(values.credentialsJson);
    if (credentials === null) {
      form.setError("credentialsJson", { message: "凭证JSON格式错误" });
      return;
    }
    const metadata = parseJsonField(values.metadataJson);
    if (metadata === null) {
      form.setError("metadataJson", { message: "元数据JSON格式错误" });
      return;
    }

    setSaving(true);
    try {
      const location = values.location?.trim() || "";
      if (!location) {
        form.setError("location", { message: "请输入来源地址" });
        return;
      }
      const normalizedType = values.sourceType
        ? values.sourceType.toUpperCase()
        : values.sourceType;
      const payload: IngestionTaskCreatePayload = {
        pipelineId: values.pipelineId,
        source: {
          type: normalizedType,
          location,
          fileName: values.fileName?.trim() || undefined,
          credentials: credentials as Record<string, string> | undefined
        },
        metadata: metadata as Record<string, unknown> | undefined
      };
      await onSubmit(payload);
    } catch (error) {
      toast.error(getErrorMessage(error, "创建失败"));
      console.error(error);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sidebar-scroll sm:max-w-[720px]">
        <DialogHeader>
          <DialogTitle>新建通道任务</DialogTitle>
          <DialogDescription>
            支持 Local File / URL / Feishu / S3 来源，Local File 会直接上传文�?          </DialogDescription>
        </DialogHeader>
        <Form {...form}>
          <form className="space-y-4" onSubmit={form.handleSubmit(handleSubmit)}>
            <FormField
              control={form.control}
              name="pipelineId"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>Pipeline</FormLabel>
                  <Select value={field.value} onValueChange={field.onChange}>
                    <FormControl>
                      <SelectTrigger>
                        <SelectValue placeholder="Select a pipeline" />
                      </SelectTrigger>
                    </FormControl>
                    <SelectContent>
                      {pipelineOptions.map((pipeline) => (
                        <SelectItem key={pipeline.id} value={pipeline.id}>
                          {pipeline.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                  <FormMessage />
                </FormItem>
              )}
            />

            <div className="grid gap-4 md:grid-cols-2">
              <FormField
                control={form.control}
                name="sourceType"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>来源类型</FormLabel>
                    <Select value={field.value} onValueChange={field.onChange}>
                      <FormControl>
                        <SelectTrigger>
                          <SelectValue placeholder="选择来源" />
                        </SelectTrigger>
                      </FormControl>
                      <SelectContent>
                        {SOURCE_OPTIONS.map((option) => (
                          <SelectItem key={option.value} value={option.value}>
                            {option.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                    <FormMessage />
                  </FormItem>
                )}
              />

              {isLocalFile ? (
                <FormItem>
                  <FormLabel>本地文件</FormLabel>
                  <FormControl>
                    <Input
                      type="file"
                      onChange={(event) => setLocalFile(event.target.files?.[0] || null)}
                    />
                  </FormControl>
                </FormItem>
              ) : (
                <FormField
                  control={form.control}
                  name="fileName"
                  render={({ field }) => (
                    <FormItem>
                      <FormLabel>文件名（可选）</FormLabel>
                      <FormControl>
                        <Input placeholder="例如：doc.md" {...field} />
                      </FormControl>
                      <FormMessage />
                    </FormItem>
                  )}
                />
              )}
            </div>

            {isLocalFile ? null : (
              <FormField
                control={form.control}
                name="location"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>来源地址</FormLabel>
                    <FormControl>
                      <Input placeholder={sourceMeta.locationPlaceholder} {...field} />
                    </FormControl>
                    <FormDescription>{sourceMeta.locationHint}</FormDescription>
                    <FormMessage />
                  </FormItem>
                )}
              />
            )}

            {showCredentials ? (
              <FormField
                control={form.control}
                name="credentialsJson"
                render={({ field }) => (
                  <FormItem>
                    <FormLabel>访问凭证（JSON，可选）</FormLabel>
                    <FormControl>
                      <Textarea
                        placeholder={sourceMeta.credentialsHint || '{"token":"xxx"}'}
                        rows={4}
                        {...field}
                      />
                    </FormControl>
                    {sourceMeta.credentialsHint ? (
                      <FormDescription>示例：{sourceMeta.credentialsHint}</FormDescription>
                    ) : null}
                    <FormMessage />
                  </FormItem>
                )}
              />
            ) : null}

            <FormField
              control={form.control}
              name="metadataJson"
              render={({ field }) => (
                <FormItem>
                  <FormLabel>任务元数据（JSON，可选）</FormLabel>
                  <FormControl>
                    <Textarea placeholder='{"source":"manual"}' rows={4} {...field} />
                  </FormControl>
                  <FormMessage />
                </FormItem>
              )}
            />

            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => onOpenChange(false)}
                disabled={saving}
              >
                取消
              </Button>
              <Button type="submit" disabled={saving}>
                {saving ? "创建�?.." : "创建任务"}
              </Button>
            </DialogFooter>
          </form>
        </Form>
      </DialogContent>
    </Dialog>
  );
}

interface UploadDialogProps {
  open: boolean;
  pipelineOptions: IngestionPipeline[];
  onOpenChange: (open: boolean) => void;
  onSubmit: (pipelineId: string, file: File) => Promise<void>;
}

function UploadDialog({ open, pipelineOptions, onOpenChange, onSubmit }: UploadDialogProps) {
  const [pipelineId, setPipelineId] = useState(pipelineOptions[0]?.id || "");
  const [file, setFile] = useState<File | null>(null);
  const [saving, setSaving] = useState(false);
  const [maxFileSize, setMaxFileSize] = useState<number>(50 * 1024 * 1024);

  useEffect(() => {
    if (open) {
      setPipelineId(pipelineOptions[0]?.id || "");
      setFile(null);
      getSystemSettings()
        .then((settings) => setMaxFileSize(settings.upload.maxFileSize))
        .catch(() => {});
    }
  }, [open, pipelineOptions]);

  const handleSubmit = async () => {
    if (!pipelineId) {
      toast.error("Select a pipeline");
      return;
    }
    if (!file) {
      toast.error("请选择文件");
      return;
    }
    if (file.size > maxFileSize) {
      const sizeMB = Math.floor(maxFileSize / 1024 / 1024);
      toast.error(`上传文件大小超过限制，最大允�?${sizeMB}MB`);
      return;
    }
    setSaving(true);
    try {
      await onSubmit(pipelineId, file);
    } catch (error) {
      toast.error(getErrorMessage(error, "上传失败"));
      console.error(error);
    } finally {
      setSaving(false);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle>上传文件并进入通道</DialogTitle>
          <DialogDescription>上传文件后立即触发通道任务</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <div>
            <label className="text-sm font-medium">Pipeline</label>
            <Select value={pipelineId} onValueChange={setPipelineId}>
              <SelectTrigger className="mt-2">
                <SelectValue placeholder="Select a pipeline" />
              </SelectTrigger>
              <SelectContent>
                {pipelineOptions.map((pipeline) => (
                  <SelectItem key={pipeline.id} value={pipeline.id}>
                    {pipeline.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>
          <div>
            <label className="text-sm font-medium">文件</label>
            <Input
              type="file"
              className="mt-2"
              onChange={(event) => setFile(event.target.files?.[0] || null)}
            />
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={saving}>
            {saving ? "上传�?.." : "上传"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface TaskDetailDialogProps {
  open: boolean;
  taskId: string | null;
  onOpenChange: (open: boolean) => void;
}

function TaskDetailDialog({ open, taskId, onOpenChange }: TaskDetailDialogProps) {
  const [task, setTask] = useState<IngestionTask | null>(null);
  const [nodes, setNodes] = useState<IngestionTaskNode[]>([]);
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!open || !taskId) return;
    let active = true;
    const load = async () => {
      setLoading(true);
      try {
        const [detail, nodeLogs] = await Promise.all([
          getIngestionTask(taskId),
          getIngestionTaskNodes(taskId)
        ]);
        if (!active) return;
        setTask(detail);
        setNodes(nodeLogs || []);
      } catch (error) {
        toast.error(getErrorMessage(error, "加载任务详情失败"));
        console.error(error);
      } finally {
        if (active) setLoading(false);
      }
    };
    load();
    return () => {
      active = false;
    };
  }, [open, taskId]);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sidebar-scroll sm:max-w-[820px]">
        <DialogHeader>
          <DialogTitle>任务详情</DialogTitle>
          <DialogDescription>{taskId || ""}</DialogDescription>
        </DialogHeader>
        {loading || !task ? (
          <div className="py-6 text-center text-muted-foreground">加载�?..</div>
        ) : (
          <div className="space-y-6">
            <div className="grid gap-4 md:grid-cols-2">
              <div className="space-y-2">
                <div className="flex items-center gap-2">
                  <Badge variant={statusBadgeVariant(task.status)}>{task.status || "-"}</Badge>
                  {task.errorMessage ? <Badge variant="destructive">error</Badge> : null}
                </div>
                <div className="text-sm text-muted-foreground">Pipeline: {task.pipelineId}</div>
                <div className="text-sm text-muted-foreground">
                  Source: {task.sourceType || "-"}{" "}
                  {task.sourceFileName || task.sourceLocation || ""}
                </div>
                <div className="text-sm text-muted-foreground">
                  Chunks: {task.chunkCount ?? "-"}
                </div>
              </div>
              <div className="space-y-2 text-sm text-muted-foreground">
                <div>Created: {formatDate(task.createTime)}</div>
                <div>Started: {formatDate(task.startedAt)}</div>
                <div>Completed: {formatDate(task.completedAt)}</div>
              </div>
            </div>

            {task.errorMessage ? (
              <div className="rounded-lg border border-destructive/30 bg-destructive/5 p-3 text-sm text-destructive">
                {task.errorMessage}
              </div>
            ) : null}

            <div>
              <h3 className="text-sm font-medium">Task metadata</h3>
              <pre className="mt-2 rounded-lg bg-muted p-3 text-xs text-muted-foreground">
                {stringifyJson(task.metadata)}
              </pre>
            </div>

            <div>
              <h3 className="text-sm font-medium">节点执行日志</h3>
              {nodes.length === 0 ? (
                <div className="mt-2 text-sm text-muted-foreground">暂无节点日志</div>
              ) : (
                <Table className="min-w-[720px]">
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-[180px]">节点</TableHead>
                      <TableHead className="w-[120px]">类型</TableHead>
                      <TableHead className="w-[100px]">Status</TableHead>
                      <TableHead className="w-[110px]">耗时</TableHead>
                      <TableHead>消息</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {nodes.map((node) => (
                      <TableRow key={node.id}>
                        <TableCell className="font-mono text-xs">{node.nodeId}</TableCell>
                        <TableCell>{node.nodeType}</TableCell>
                        <TableCell>
                          <Badge variant={nodeStatusVariant(node.status)}>
                            {node.status || "-"}
                          </Badge>
                        </TableCell>
                        <TableCell>{node.durationMs ?? "-"} ms</TableCell>
                        <TableCell>
                          <div className="text-xs text-muted-foreground">
                            {node.message || node.errorMessage || "-"}
                          </div>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              )}
            </div>
          </div>
        )}
      </DialogContent>
    </Dialog>
  );
}
