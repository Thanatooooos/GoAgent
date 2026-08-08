# Daily Brief 分层选题树（大卡片 / 小卡片）

> **目标：** 平台维护分层 topic catalog，用户在 `/brief` 通过「大卡片 → 小卡片」多选订阅；**只有叶子 topic** 进入生成与分栏。不要求用户自定义 RSS。

**与 MVP 关系：** 保留现有抓取 / 调度 / 生成链路；扩展 `topic_catalog`、source 绑定、订阅 UI 与 `GET topic-catalog` API。

---

## 1. Key 命名约定

| 规则 | 示例 |
|------|------|
| 大卡片（L1） | 单段小写英文：`tech`, `art`, `music`, `politics` |
| 小卡片（L2 叶子） | `L1.slug`：`tech.ai`, `tech.cs`, `art.design` |
| 可选 L3（后期） | `tech.ai.models`, `tech.cs.systems` |
| 字符集 | `[a-z0-9]+` 段，段间 `.` 分隔 |
| 订阅存储 | **仅叶子 key** 写入 `subscription.topics[]` |
| 展示名 | 中文 `displayName`，不进 key |

**禁止：** 用户订阅 L1（如只选 `tech`）；L1 只做导航与 UI 分组。

---

## 2. 八个大卡片（L1）

| Key | 展示名 | 副标题（卡片描述） |
|-----|--------|-------------------|
| `tech` | 科技 | AI、计算机、工程与产业创新 |
| `science` | 科学 | 数学、物理、生命与基础科研 |
| `art` | 艺术 | 视觉、设计、电影与当代艺术 |
| `music` | 音乐 | 创作、产业、技术与现场文化 |
| `politics` | 时政 | 国内外政策、地缘与公共议题 |
| `business` | 商业财经 | 公司、市场、消费与宏观经济 |
| `culture` | 文化 | 出版、历史、社会与思想 |
| `games` | 游戏 | 主机、独立游戏、电竞与互动娱乐 |

首期 UI 可只开放 **科技 / 艺术 / 音乐 / 时政** 四个大卡片，其余显示「即将上线」。

---

## 3. 小卡片（L2 叶子）— 首期重点四域

### 3.1 科技 `tech`（8 个）

| Key | 展示名 | 说明 |
|-----|--------|------|
| `tech.ai` | AI 与大模型 | 模型发布、对齐、应用 |
| `tech.cs` | 计算机科学 | 系统、算法、软件工程 |
| `tech.dev` | 开发者与开源 | 社区、工具链、热榜 |
| `tech.security` | 安全 | 漏洞、隐私、攻防 |
| `tech.hardware` | 芯片与硬件 | 半导体、算力、设备 |
| `tech.startups` | 创业与产业 | 融资、产品、行业动态 |
| `tech.product` | 产品与互联网 | 消费级应用、平台 |
| `tech.robotics` | 机器人与自动化 | 具身智能、工业自动化 |

### 3.2 艺术 `art`（6 个）

| Key | 展示名 | 说明 |
|-----|--------|------|
| `art.contemporary` | 当代艺术 | 展览、艺术家、艺术市场 |
| `art.design` | 设计与创意 | 平面、工业、UX |
| `art.film` | 电影与影像 | 院线、流媒体、制作 |
| `art.photography` | 摄影 | 纪实、商业、器材 |
| `art.architecture` | 建筑与空间 | 城市、地标、室内设计 |
| `art.digital` | 数字艺术 | NFT、生成艺术、AI 创作 |

### 3.3 音乐 `music`（6 个）

| Key | 展示名 | 说明 |
|-----|--------|------|
| `music.industry` | 音乐产业 | 版权、流媒体、商业 |
| `music.electronic` | 电子音乐 | 制作、巡演、厂牌 |
| `music.rock-pop` | 流行与摇滚 | 专辑、艺人、现场 |
| `music.classical` | 古典与实验 | 古典、先锋、剧场 |
| `music.tech` | 音乐科技 | 制作工具、AI 音乐、硬件 |
| `music.live` | 现场与音乐节 | 演出、节庆、票务 |

### 3.4 时政 `politics`（6 个）

| Key | 展示名 | 说明 |
|-----|--------|------|
| `politics.china` | 国内时政 | 政策、治理、社会议题 |
| `politics.global` | 国际局势 | 地缘、外交、冲突 |
| `politics.economy-policy` | 经济政策 | 财政、货币、监管 |
| `politics.tech-policy` | 科技政策 | AI 监管、数据、反垄断 |
| `politics.energy` | 能源与气候政治 | 碳排、能源安全 |
| `politics.elections` | 选举与政党 | 重要选举、民调 |

### 3.5 其余 L1 的 L2（二期展开，先占位）

每个 L1 预留 4–6 个叶子 key，**无 source 前不展示**。例如：

- `science.math`, `science.physics`, `science.biology`, `science.space`
- `business.markets`, `business.startups`, `business.retail`, `business.macro`
- `culture.books`, `culture.history`, `culture.society`
- `games.pcg`, `games.console`, `games.indie`, `games.esports`

---

## 4. MVP topic 对照（实现时直接删除旧 key）

| 旧 key（将删除） | 新叶子 key |
|------------------|------------|
| `ai-models` / `ai-research` / `ai-tools` | `tech.ai` |
| `developer-trends` | `tech.dev` |
| `startups-and-industry` | `tech.startups` |

**说明：** 无老用户，不做 legacy 别名；直接改用新叶子 key。本地测试数据可清库或一次性 SQL 替换。

---

## 5. 现有 Source → 新叶子绑定（首期可上线）

| Source key | 新叶子 topic | 状态 |
|------------|--------------|------|
| `openai-blog` | `tech.ai` | 已有 |
| `anthropic-blog` | `tech.ai` | 已有 |
| `google-deepmind-blog` | `tech.ai` | 已有 |
| `meta-ai-blog` | `tech.ai` | 已有 |
| `arxiv-cs-ai` | `tech.ai` | 已有 |
| `arxiv-cs-cl` | `tech.ai` | 已有 |
| `arxiv-cs-lg` | `tech.ai` | 已有 |
| `papers-with-code` | `tech.ai` | 已有 |
| `hacker-news` | `tech.dev` | 已有 |
| `github-trending` | `tech.dev` | 已有 |
| `the-decoder` | `tech.startups` | 已有 |
| `venturebeat-ai` | `tech.startups` | 已有 |
| `techcrunch-ai` | `tech.startups` | 已有 |

**艺术 / 音乐 / 时政：** 二期按叶子逐个补 RSS（见 §7）。上线规则：**叶子无 source 则 UI 灰显或隐藏**。

---

## 6. 后端结构建议

> **两层是首期 UI / 运营粒度，不是技术上限。** Key 用点分路径可任意加深（§1）；订阅、source 绑定、生成分栏**只认叶子**。Catalog 按**通用树**建模，避免把「L1 + L2」写死在类型里。

### 6.1 Domain（可扩展树）

```go
type TopicNode struct {
    Key         string
    ParentKey   string // 根节点为空
    DisplayName string
    Description string // 卡片副标题
    SortOrder   int
    Selectable  bool   // true = 叶子，可订阅、可绑 source、可进简报分栏
    Enabled     bool   // 无 source 或未开放时可 false（UI 灰显）
    Children    []TopicNode // 仅内存/API 组装用；持久化 catalog 可为 flat list + ParentKey
}
```

**规则（固定，不随层级数变化）：**

| 规则 | 说明 |
|------|------|
| 仅叶子可订阅 | `Selectable == true` 的 key 才能写入 `subscription.topics[]` |
| 仅叶子绑 source | `source_feed_catalog` 的 `topicKey` 必须指向叶子 |
| 中间节点 | `Selectable == false`，只做导航分组；可有任意深度子节点 |
| Key 深度 | 由点分段数决定：`tech`（1）→ `tech.ai`（2）→ `tech.ai.models`（3） |

**辅助函数：**

- `TopicCatalogFlat() []TopicNode` — 全量 flat list（含 ParentKey）
- `TopicCatalogTree() []TopicNode` — 组装为带 `Children` 的树
- `LeafTopicKeys() []string` — 所有 `Selectable == true` 的 key
- `IsTopicKeySupported(key)` — 等价于「key 是已启用叶子」
- `TopicDisplayName(key)` / `TopicBreadcrumb(key)` — 展示名；后者用于多层级 UI（如「科技 · AI · 大模型」）

**不要：** 用 `TopicLevelDomain=1 / TopicLevelLeaf=2` 这类固定层级 enum；首期 UI 只展示两层，但 catalog 结构应能直接加 L3。

### 6.2 何时加第三层（L3）

多数栏目两层足够。仅在以下情况考虑把某 L2 **拆成中间节点**并挂 L3 叶子：

- 该栏目 RSS 过多，单栏简报过于混杂
- 运营需要更细的分栏（例：`tech.ai` → `tech.ai.models` + `tech.ai.agents` + `tech.ai.policy`）
- LLM 分栏 prompt 在同一 key 下难以稳定归类

**拆分流程（无 legacy）：** 将原叶子改为 `Selectable: false` 或删除 → 新增 L3 叶子 → source 改绑 → 测试订阅校验。本地数据清库或一次性 SQL 即可。

### 6.3 API

| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/daily-brief/topic-catalog` | 返回**递归**树 JSON；节点含 `children[]`，叶子含 `selectable: true` |
| PUT | `/daily-brief/subscription` | 仍传 `topics: string[]`（仅叶子 key） |

**topic-catalog 响应示例（首期两层；结构已支持更深）：**

```json
{
  "nodes": [
    {
      "key": "tech",
      "displayName": "科技",
      "description": "AI、计算机、工程与产业创新",
      "selectable": false,
      "enabled": true,
      "children": [
        {
          "key": "tech.ai",
          "displayName": "AI 与大模型",
          "description": "模型发布、对齐、应用",
          "selectable": true,
          "enabled": true,
          "hasSources": true,
          "children": []
        }
      ]
    }
  ]
}
```

**L3 扩展示例（后期，仅说明结构）：** `tech.ai` 改为 `selectable: false`，其 `children` 下挂 `tech.ai.models`、`tech.ai.agents` 等叶子；前端对该节点再展开一层即可，API 形状不变。

### 6.4 生成

- `FilterCandidatesByTopics`：不变，按叶子过滤。
- `prompt_builder`：注入用户选中叶子的 `displayName` + 父域名称。
- `AlignBriefArtifactWithCandidates`：不变。
- 多叶子时：`max-items` 建议改为 **每叶子最多 2 条，全局上限 8**（配置项 `max-items-per-topic`）。

---

## 7. 二期 Source 候选（艺术 / 音乐 / 时政）

> 实施前需用 `curl -I` 验证 RSS 可用，并加 `testdata` fixture。

| 叶子 | 候选 RSS（待评审） |
|------|-------------------|
| `art.design` | Creative Review / Dezeen 等 |
| `art.film` | 院线或行业媒体 RSS |
| `music.industry` | Music Business Worldwide 等 |
| `music.tech` | 与 AI 音乐交叉源 |
| `politics.global` | Reuters World / BBC 国际等 |
| `politics.tech-policy` | 可复用部分 `techcrunch-ai` + 政策类源 |

---

## 8. 前端交互（`/brief` 订阅区）

```
┌─────────────────────────────────────┐
│ 选择你关心的领域（大卡片，可多选）    │
│ [科技] [艺术] [音乐] [时政] ...      │
└─────────────────────────────────────┘
         ↓ 展开已选领域
┌─────────────────────────────────────┐
│ 科技 · 细分类别（小卡片，可多选）     │
│ [AI与大模型] [开发者] [创业] ...     │
└─────────────────────────────────────┘
┌─────────────────────────────────────┐
│ 已选：科技·AI、科技·创业 | 音乐·产业  │
└─────────────────────────────────────┘
```

- 大卡片多选 → 展示多个领域下的小卡片区。
- 节点 `selectable: false` 且有 `children` → **再展开一层**（首期可只遇到 L1，逻辑先写好）。
- 叶子 `selectable: true` 且 `hasSources: false` → 禁用 + tooltip「即将上线」。
- 保存时只提交叶子 key 数组；`TopicBreadcrumb` 用于已选 chips（如「科技 · AI · 大模型」）。

---

## 9. 实施顺序

| 阶段 | 任务 | 可交付 |
|------|------|--------|
| **P0** | `TopicNode` catalog + source 改绑新叶子 | 后端测试通过 |
| **P1** | `GET /topic-catalog` + 前端大/小卡片 | UI 可选科技下 8 个小卡片 |
| **P2** | 艺术/音乐/时政各补 2–3 个 RSS + fixture | 四域大卡片全部可订阅 |
| **P3** | `max-items-per-topic`、prompt 多栏优化 | 多选体验稳定 |
| **P4** | 开放 science / business / culture / games | 八域齐全 |

---

## 10. 配置建议（多叶子后）

```yaml
daily-brief:
  generation:
    max-candidates: 16
    max-items: 8
    max-items-per-topic: 2
    prompt-version: v4   # 多域分栏 prompt
  schedule:
    run-timeout-ms: 600000
```

---

## 11. 验收标准

- [ ] 用户只能订阅叶子 topic（`selectable: true`）；中间节点不能写入 subscription。
- [ ] Catalog 为通用树模型，无固定「仅两层」类型约束。
- [ ] 科技域 8 叶子 + 现有 13 source 全覆盖无孤儿源。
- [ ] UI 大卡片 → 小卡片流程与 `GET topic-catalog` 一致。
- [ ] 选 3+ 叶子时简报分栏正确、条数受 per-topic 限制。

---

**下一步（实现）：** 待确认后从 P0 开始改 `topic_catalog.go`、`source_feed_catalog.go` 与 `constants_test.go`，再接 P1 API 与前端卡片。**当前仅方案，不落代码。**
