# Development Notes 2026-05-15

## Chunk 中文场景优化

### 本轮目标

- 不重做 chunk 架构，先做一轮“低风险、高收益”的中文友好性优化
- 优先解决：
  - 默认 `overlap` 不生效
  - fixed-size 句边界搜索偏弱
  - 中文条款/标题边界容易被切碎

### 已完成

- `internal/app/core/chunk/fixed_size_chunker.go`
  - 调整句边界搜索范围与优先级
  - 改善句号、空行、换行附近的切块收敛
  - 增加中文条款/标题软边界识别，减少条款名被切断
- `internal/app/knowledge/service/document_process_service.go`
  - 在文档处理主链路补上默认 overlap 兜底
  - 不改底层 normalize 的 `0` 语义，避免破坏显式配置
- 测试补齐：
  - `internal/app/core/chunk/test/fixed_size_chunker_test.go`
  - `internal/app/knowledge/service/test/document_process_service_test.go`

### 验证

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/core/chunk/... -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/knowledge/service/test -run TestDocumentProcessServiceExecuteChunkUsesDefaultOverlapWhenChunkConfigMissing -count=1
```

- `internal/app/core/chunk/...` PASS
- 指定文档处理增量测试 PASS

## Chat 检索前置路由：由 Rewrite 判断是否需要 Retrieval

### 问题

- 旧流程里无论用户问什么，都会先走 retrieval
- 对“你好”“谢谢”“你是谁”这类闲聊，请求会白白消耗检索和工具资源

### 已完成

- `internal/app/rag/core/rewrite/rewrite.go`
  - `rewrite.Result` 收口为：
    - `RewrittenQuestion`
    - `SubQuestions`
    - `NeedRetrieval`
  - 增加本地兜底 `InferNeedRetrieval(...)`
- `internal/app/rag/core/rewrite/llm_rewrite_service.go`
  - rewrite prompt 追加 `need_retrieval`
  - 模型在改写问题时同步输出是否需要检索
- `internal/app/rag/service/rag_chat_service.go`
  - `prepareChat()` 根据 rewrite 结果决定是否执行 retrieve
  - 无 `KnowledgeBaseIDs` 或 `NeedRetrieval=false` 时跳过 retrieval
  - tool workflow 也跟随 retrieval 一起短路

### 结果

- 主决策由 LLM 做
- 规则只作为 fallback
- 闲聊和知识库问答的资源路径开始分离

### 验证

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/core/rewrite -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/service -count=1
```

- `internal/app/rag/core/rewrite` PASS
- `internal/app/rag/service` PASS

## SearchMode 收口清理

### 背景

- 当前产品策略已经统一使用 `hybrid`
- 继续保留 `searchMode` 作为前后端显式字段，会制造“似乎还能切模式”的误导

### 已完成

- 后端主链路统一固定 `ragretrieve.SearchModeHybrid`
- 删除 chat 路径上的 `resolveRetrieveSearchMode(...)`
- 删除 tool workflow input 里的 `SearchMode`
- rewrite 摘要不再传 `preferred_search_mode`
- 前端聊天消息不再展示“检索策略”
- trace 详情页不再展示：
  - `mode`
  - `requested`
  - `resolved`

### 影响

- chat 路径现在真正只保留一个关键决策：要不要检索
- 检索“怎么检”在当前阶段退化为实现细节，不再作为用户和维护者都能看到的伪配置项

### 验证

```powershell
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/tool -count=1
$env:GOCACHE='D:\code\GoAgent\.gocache-rag'; go test ./internal/app/rag/tool/planner -count=1
```

- `internal/app/rag/tool` PASS
- `internal/app/rag/tool/planner` PASS

## 今日结论

今天这轮工作把 RAG 主链路收得更实用了：

- chunk 对中文文档更友好了一些
- chat 不再对每条消息都强制检索
- `searchMode` 在 chat 主链路上基本完成收口

下一步更值得继续补的是：

- 给 `NeedRetrieval=false` 的路径单独准备更轻的聊天 prompt
- 补 trace 里“为什么跳过 retrieval”的显式可观测性
- 继续清掉前端 `chatStore.ts` 顶部残留的 retrieval mode helper 死代码
