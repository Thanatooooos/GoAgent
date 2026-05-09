# Development Notes - 2026-05-09

## 今日主题

今天的工作集中在 `ingestion` 的最小收口和文档整理，目标不是继续扩大 ingestion 能力边界，而是先把：

1. reconcile 修复行为做成可见信息
2. `internal/app/ingestion/service` 目录结构收拾清楚
3. 在项目层面明确“短期不再以 ingestion 为主工作模块”

## 本次改动

### 1. 做了一版 ingestion 最小收口

本轮没有继续扩展 ingestion 主执行链，而是在现有主链之上补了一层“修复行为留痕”。

改动点：

- `internal/app/knowledge/service/knowledge_document_ingestion_reconcile.go`
  - 将 reconcile 的执行结果收敛为结构化结果
  - 区分：
    - `skipped`
    - `documentUpdated`
    - `chunkLogUpdated`
    - `chunkLogCreated`
    - `errorMessage`

- `internal/app/knowledge/service/knowledge_document_service.go`
  - 新增 `IngestionReconcileRecorder`
  - `KnowledgeDocumentService` 可注入 reconcile recorder

- `internal/bootstrap/knowledge/ingestion_bridge.go`
  - 新增 knowledge -> ingestion metrics 的 recorder adapter

- `internal/app/ingestion/service/observer_metrics.go`
  - ingestion metrics 新增 `reconcile` 维度
  - 增加：
    - `attempts`
    - `skipped`
    - `documentUpdated`
    - `chunkLogUpdated`
    - `chunkLogCreated`
    - `failures`
    - `lastFailure`

- `cmd/server/main.go`
  - runtime 装配时将 reconcile recorder 接入 ingestion metrics

结果：

- `GET /ingestion/metrics` 不再只展示 task/node 运行态，还能看到 reconcile 修复是否发生、是否失败、最近一次失败是什么

### 2. 整理了 `internal/app/ingestion/service` 的文件结构

当前目录已经统一为按职责分组的命名方式：

- `service_*`
- `executor_* / workflow_*`
- `runner_*`
- `observer_*`

同时新增：

- `internal/app/ingestion/service/doc.go`

用于说明这一层文件的分组语义。

这次整理只做“低风险可读性收敛”：

- 不拆 package
- 不改变对外 API
- 不调整 runtime 装配方式
- 不改变主执行逻辑

### 3. 更新了项目进度文档

已更新：

- `docs/project_progress_context.md`

更新内容包括：

- 新增 2026-05-09 进展
- 写明 ingestion 已阶段性收口
- 明确短期不再以 ingestion 为主工作模块
- 将近期待办重心切回：
  - `RAG retrieve`
  - `diagnose`
  - `trace / tool / fallback`

### 4. 新增 ingestion 目标方向文档目录

新建目录：

- `docs/ingestion/`

并补充 ingestion 的目标方向文档，方便后续中期再回到该模块时快速对齐。

## 验证

已通过：

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/app/knowledge/service ./internal/adapter/http/ingestion -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-agent'; go test ./internal/app/ingestion/service ./internal/adapter/http/ingestion ./internal/bootstrap/ingestion ./internal/app/knowledge/service -count=1
```

结果：

- `internal/app/ingestion/service` PASS
- `internal/app/knowledge/service` PASS
- `internal/adapter/http/ingestion` PASS
- `internal/bootstrap/ingestion` 可正常编译

## 当前判断

今天这轮之后，`ingestion` 的状态更适合被定义为：

- 主链已具备
- 一致性已有基础收口
- 修复行为开始可见
- 目录结构可读性已提升
- 但仍未达到“继续深挖最划算”的阶段

因此短期策略调整为：

- `ingestion` 暂停作为主推进模块
- 后续仅做必要修复和被动配套
- 主研发重心切回 `RAG / diagnose / tool / trace`

## 后续保留事项

虽然短期不主攻 ingestion，但以下事项保留为中期待办：

- pending/running 超时治理
- reconcile 结果沉淀与修复审计
- 更完整的 `document / chunk_log / task` 不一致规则矩阵
- 更系统的恢复策略与告警暴露
