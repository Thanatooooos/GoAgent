# Ingestion Target Direction

更新时间：2026-05-09

## 文档目的

这份文档用于单独记录 `ingestion` 模块的目标方向、阶段定位和中长期改进项。

当前项目已经完成 ingestion 主链和一轮最小收口，但**短期内不再把 ingestion 作为主工作模块**。因此这里更强调：

- ingestion 最终应该达到什么状态
- 当前已经到了哪里
- 后面什么时候值得再回来继续做

## 目标定义

理想中的 ingestion，不只是“能把文档塞进去”，而是一个：

- 结果可信
- 失败可恢复
- 问题可诊断
- 异常可见
- 状态可收口

的文档入库系统。

## 目标拆解

### 1. 结果可信

希望最终做到：

- task 成功就是真成功
- task 失败就能明确失败在哪
- running 代表真实在运行，而不是卡住
- `task / task_node / document / chunk_log` 之间状态不互相打架

### 2. 失败可恢复

希望最终做到：

- 临时性错误可自动重试
- 半成功写入可自动补偿清理
- 回写漂移可自动 reconcile
- 扫描机制能发现未完全收口的状态
- 自动修不了的异常可以明确暴露出来

### 3. 问题可诊断

希望最终做到：

- 快速定位失败 node
- 快速判断是 source、parser、chunker、indexer 还是回写问题
- 能看到 retry、duration、lastError、errorCategory 等证据
- 能知道系统是否已尝试 reconcile，以及 reconcile 的结果

### 4. 异常可见

希望最终做到：

- 能看到多少 task 成功、失败、卡住
- 能看到多少 document 被 reconcile 修复过
- 能知道自动修复成功了什么、失败了什么
- 能明确哪些异常需要人工关注

### 5. 架构可扩展

希望最终做到：

- 在不破坏稳定性的前提下扩展新的 source type
- 扩展 parser、chunker、indexer 策略
- 后续再考虑更复杂的 workflow 编排

## 当前状态判断

截至 2026-05-09，ingestion 已经具备：

- `pipeline -> task -> task_node -> knowledge 回写` 主链
- `fetcher / parser / chunker / indexer`
- 节点级 retry + backoff
- `Indexer` 失败补偿清理
- `document` 活动 task 保护
- task-scoped chunk log 回写保护
- 即时 reconcile
- 后台 reconcile scan
- reconcile 结果接入 ingestion metrics
- service 目录结构已按职责分组整理

当前仍未完全收口的部分：

- pending/running 超时治理
- reconcile 结果沉淀与修复审计
- 更完整的不一致状态规则矩阵
- 更系统的恢复策略
- 更清晰的异常统计与告警暴露

## 短期策略

### 当前决定

短期内不再把 `ingestion` 作为主工作模块。

原因：

- 主链已经具备，当前继续深挖 ingestion 的边际收益，不如优先投入到：
  - `RAG retrieve`
  - `diagnose`
  - `tool / trace / fallback`
- ingestion 已完成一版最小收口，适合先“冻结主要方向”，仅保留必要修复

### 短期只做什么

- 必要 bugfix
- 被动配套改动
- 与主链强相关的兼容性修正

### 短期不做什么

- 不继续扩大 ingestion 设计范围
- 不继续做较重的恢复治理工程
- 不以 ingestion 作为近期 roadmap 的主推进项

## 中期待办

后续若重新回到 ingestion，建议按下面顺序推进：

### P0

- pending/running 超时治理
- `document / chunk_log / task` 不一致规则矩阵收口
- reconcile 结果沉淀与失败可见性增强

### P1

- 更系统的恢复策略
- 更细粒度异常统计
- 更完整的排障建议和管理端暴露

### P2

- 更复杂的 workflow 扩展
- 更强的运营视角与人工干预入口

## 重新启动 ingestion 优先级的条件

建议只有在下面情况之一成立时，再把 ingestion 升回主工作模块：

1. 真实运行中频繁暴露出状态漂移或卡住问题
2. RAG / diagnose / trace 主线已经完成一轮关键收口
3. 需要支撑新的 source / workflow 能力，现有 ingestion 结构开始成为阻碍
