# Wiki P4：前端 wiki 浏览器 + 图谱

日期：2026-08-09
状态：待审

## 背景与范围

P0-P3 已建成 wiki 后端（生成/链接/检索/Agent）。**P4 前端可视化**：浏览 KB 的 wiki 页面、阅读页面内容、看链接图谱。

**P4 交付**：
1. 后端图谱 API：`GET /knowledge-base/:kb-id/wiki/graph` → `{nodes, edges}`。
2. 前端 `wikiService.ts`（listPages / getPage / getGraph）。
3. Wiki 浏览器页（挂在既有 KB 上下文 `/admin/knowledge/:kbId/wiki`）：页面列表 + Markdown 渲染（`[[slug|标题]]` → 内部链接）。
4. 图谱视图：SVG 简单布局展示 nodes/edges，节点可跳转。
5. AdminLayout 菜单：知识库详情页内加 "Wiki" 入口（Tab 或链接）。

**明确不做**：WeKnora 式完整力模拟、页面编辑/修订 UI、多 KB 图谱总览、wiki 页搜索。

## 一、后端图谱 API

`internal/app/knowledge/domain/wiki_graph.go`：

```go
type WikiGraphNode struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	InLinks  int    `json:"inLinks"`
	OutLinks int    `json:"outLinks"`
}

type WikiGraphEdge struct {
	From   string `json:"from"`
	To     string `json:"to"`
	Anchor string `json:"anchor"`
}

type WikiGraph struct {
	Nodes []WikiGraphNode `json:"nodes"`
	Edges []WikiGraphEdge `json:"edges"`
}
```

`WikiPageService.GetGraph(ctx, kbID) (domain.WikiGraph, error)`：
- `pageRepo.ListByKB` 全量页 → nodes（含 in/out_links 已持久化）。
- `linkRepo.ListByKB` → edges（from/to 为页面 ID）。
- 空页 → 空 graph（非错）。

`wiki_page_handler.go` 加路由 + handler：

```go
	r.GET("/knowledge-base/:kb-id/wiki/graph", handler.Graph)
```

`Graph` handler 调 `service.GetGraph` 返回 `writeSuccess(c, graph)`。`WikiPageService` 接口加 `GetGraph(ctx, kbID) (domain.WikiGraph, error)`。

## 二、前端

### wikiService.ts

```ts
import { api } from "@/services/api";

export interface WikiPageItem {
  id: string;
  kbId: string;
  slug: string;
  title: string;
  pageType: string;
  status: string;
  summary: string;
  content?: string;
  inLinks?: number;
  outLinks?: number;
}

export interface WikiGraphData {
  nodes: { id: string; slug: string; title: string; inLinks: number; outLinks: number }[];
  edges: { from: string; to: string; anchor: string }[];
}

export const listWikiPages = async (kbId: string, current = 1, size = 100): Promise<WikiPageItem[]> => {
  const page = await api.get<{ records: WikiPageItem[] }, { records: WikiPageItem[] }>(
    `/knowledge-base/${kbId}/wiki/pages?current=${current}&size=${size}`
  );
  return page.records || [];
};

export const getWikiPage = async (kbId: string, slug: string): Promise<WikiPageItem> => {
  return api.get<WikiPageItem, WikiPageItem>(`/knowledge-base/${kbId}/wiki/pages/${encodeURIComponent(slug)}`);
};

export const getWikiGraph = async (kbId: string): Promise<WikiGraphData> => {
  return api.get<WikiGraphData, WikiGraphData>(`/knowledge-base/${kbId}/wiki/graph`);
};
```

### Wiki 浏览器页 `frontend/src/pages/admin/knowledge/WikiBrowserPage.tsx`

- 路由：`/admin/knowledge/:kbId/wiki`。
- 布局：左侧页面列表（title + summary + in/out 徽标），右侧详情（选中页内容 Markdown）。
- Markdown 渲染：复用 `MarkdownRenderer`；`[[slug|标题]]` 转换：预处理内容，把 `[[slug|标题]]` 替换为 `[标题](/admin/knowledge/:kbId/wiki?slug=...)` markdown 链接（或用一个自定义 remark 插件/预处理函数 `convertWikiLinks(content, kbId)`）。
- 图谱按钮：切到图谱视图（同页 Tab 或子路由）。
- 状态：选中页 slug 存 query param `?slug=`，支持直达。

### 图谱视图（同页 Tab）

- `getWikiGraph(kbId)` → SVG 渲染 nodes（圆 + title）与 edges（连线）。
- 布局：简单径向/力导向的近似——用 `useState` 迭代若干次简单斥力+弹簧，或直接圆形布局（节点按 outLinks 排序摆放圆周）。为 P4 简单：**圆形布局**（所有节点均匀放在圆周，edges 画弦），节点点击跳转页面。
- 图例：inLinks/outLinks 徽标。

### AdminLayout / 知识库详情页入口

- `KnowledgeDocumentsPage`（`/admin/knowledge/:kbId/docs`）的 tab 或按钮区加 "Wiki" 入口，链接到 `/admin/knowledge/:kbId/wiki`。读 `KnowledgeDocumentsPage.tsx` 的导航结构，在合适位置加入口。

## 三、测试策略

- 后端：`WikiPageService.GetGraph` 单测（stub pages/links → nodes/edges）；handler 测试（Graph 路由返回结构）。
- 前端：`npm run build` + `npx tsc --noEmit`（lint 为仓库既有损坏，跳过）。
- 回归：`go test ./internal/app/knowledge/... ./internal/adapter/http/knowledge/... -count=1`。

## 四、明确不做（YAGNI）

- 完整力模拟图谱、页面编辑/修订 UI、多 KB 总览、wiki 搜索、移动端适配。

## 回归约束

- 后端只新增（graph 域类型 + service 方法 + handler 路由）。
- 前端只新增页面/服务/入口，不改既有聊天渲染。
