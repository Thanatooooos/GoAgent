# BM25 关键词检索替换计划

## Summary

目标是在**不引入外部搜索服务**的前提下，把当前基于 PostgreSQL `pg_trgm + word_similarity` 的 `keyword` / `metadata_title` 检索，替换为**PostgreSQL 内建的中文 lexical/BM25 风格检索方案**，并保留现有 `semantic` / `hybrid` 主链路、评估工具和通道抽象不变。

成功标准：

- `SearchByKeyword` 不再以 `word_similarity` 为主实现。
- `SearchByMetadata` 不再以 `word_similarity` 为主实现。
- `docs_markdown_samples_v2.json` 上：
  - `file_name` 不明显退化；
  - `section metadata` 明显优于当前 keyword 基线；
- `hybrid` 相比当前版本有可观测提升。
- T2Retrieval 继续保留为 retriever-only baseline，但不作为 chunk 策略结论依据。

## Current Status

截至 `2026-05-17`，以下工作已经完成并进入代码：

- 已新增 PostgreSQL lexical migration：
  - [20260517143000_add_chunk_vector_lexical_indexes.sql](/D:/goagent/internal/adapter/repository/postgres/migrations/20260517143000_add_chunk_vector_lexical_indexes.sql:1)
  - 为 `t_knowledge_chunk_vector` 增加 `content_lexemes`、`metadata_document_name_lexemes`、`metadata_source_file_name_lexemes`、`metadata_section_lexemes`
  - 为上述列增加基于 `to_tsvector('simple', ...)` 的 GIN 索引
- 已新增 lexical tokenization / normalize 辅助实现：
  - [lexical.go](/D:/goagent/internal/adapter/vectorstore/pgvector/lexical.go:1)
  - 支持中文双字切分、ASCII token 拆分、query stop phrase 清洗、短标识符 fallback 判定
- 已新增相关单元测试：
  - [lexical_test.go](/D:/goagent/internal/adapter/vectorstore/pgvector/lexical_test.go:1)
- 已新增配置项与默认值：
  - [config.go](/D:/goagent/internal/framework/config/config.go:190)
  - [application.yaml](/D:/goagent/configs/application.yaml:91)
  - 包括 `enabled-fallback-trgm`、`section-weight`、`document-name-weight`、`source-file-name-weight`
- 已将 `VectorStore` 写入链路接到 lexical 列，并兼容 migration 未执行时的旧表结构：
  - [vector_store.go](/D:/goagent/internal/adapter/vectorstore/pgvector/vector_store.go:24)
  - 新写入优先写 lexical 列
  - 若数据库尚未应用 lexical migration，则自动回退到旧 upsert 路径，避免直接报错
- 已将 `SearchByKeyword` / `SearchByMetadata` 主实现切换为 PostgreSQL lexical 查询，并保留 trigram fallback：
  - [vector_store.go](/D:/goagent/internal/adapter/vectorstore/pgvector/vector_store.go:144)
  - lexical 查询使用 `to_tsvector('simple', ...)` + `to_tsquery('simple', ...)`
  - metadata 检索使用 `section / document_name / source_file_name` 加权排序
  - lexical 查询无结果、短 ASCII 标识符 query、或 lexical 列不存在时，会回退到旧 `pg_trgm + word_similarity`

当前尚未完成：

- `cmd/lexical-rebuild` 回填/重建 CLI
- 基于 `docs_markdown_samples_v2.json` 的新一轮基准评估与诊断报告
- 对“去标题化手写 query”验收集的补充

## Implementation Changes

### 1. 检索后端改造

将当前 `corevector.Searcher` 的两条 lexical 能力保留为同名接口，但更换底层实现：

- `SearchByKeyword(ctx, query, kbIDs, topK)`
  - 改为查 **chunk 内容 lexical 索引**，排序采用 PostgreSQL 全文检索/BM25 风格分数。
- `SearchByMetadata(ctx, query, kbIDs, topK)`
  - 改为查 **`document_name` / `source_file_name` / `section` 的独立 lexical 索引**，支持字段权重。
- `word_similarity` 保留为 fallback，仅用于：
  - 文件名/短标识符极短 query 的补充兜底；
  - 索引不可用时的降级路径；
  - 不再作为默认主排序。

通道层不改接口与职责：

- `keywordChannel` 继续代表正文 lexical 检索。
- `metadataTitleChannel` 继续代表标题/文件名/section 定向检索。
- `hybrid` 仍由 lexical + vector 融合，不改 fusion 主流程。

### 2. 索引与表结构

在现有 PostgreSQL 体系内新增 lexical 索引载体，覆盖两类数据：

- chunk content
- metadata 字段：
  - `document_name`
  - `source_file_name`
  - `section`

默认实现选择：

- 为内容和 metadata 各自提供**可全文检索的索引列/表达式**；
- metadata 与 content 分开建索引，避免标题字段被正文噪声淹没；
- `section` 权重高于 `document_name` / `source_file_name`；
- metadata 查询按字段权重合成最终排序分数。

迁移要求：

- 新增 migration，不覆盖现有 `pg_trgm` migration；
- migration 只做增量添加，不破坏现有数据；
- 保留旧 trigram 索引，等新方案稳定后再决定是否删除。

### 3. 写入与回填链路

所有 lexical 索引数据必须和 chunk 生命周期同步：

- 新文档导入时建立 lexical 索引；
- 文档重新 chunk 时重建该文档所有 lexical 数据；
- chunk 更新时同步更新 lexical 数据；
- chunk 删除/禁用时同步删除或失效 lexical 数据。

实现策略：

- 以当前文档处理与 chunk 持久化链路为唯一写入源；
- 不额外引入异步最终一致写入；
- 先保证“同事务或同流程内同步一致”，再谈后续优化。

需要新增一个**索引回填/重建 CLI**：

- 面向已有 KB 全量重建 lexical 索引；
- 支持按 KB 重建；
- 支持 dry-run/统计输出；
- 作为 migration 后的运维工具，而不是临时脚本。

### 4. 查询策略细化

为避免实现时再做产品决策，这里直接锁默认行为：

- `SearchByKeyword`
  - 主查正文内容；
  - query 直接走 lexical 检索，不做 LLM rewrite 前置依赖；
  - 允许后续加轻量 query normalize，但第一版不依赖 LLM。
- `SearchByMetadata`
  - 主查 `section`、`document_name`、`source_file_name`；
  - 以 `section` 为最高权重；
  - 对完全相同或高重叠标题应优先命中深层 section，而不是总被顶层标题吃掉。
- `hybrid`
  - 继续沿用现有 channel expand topK + fusion + dedup + rerank；
  - 第一版不改 fusion 算法，只替换 lexical 候选质量。

## Public Interfaces / APIs

对外接口保持稳定，不新增新的调用参数；改动集中在实现与运维面。

需要明确的新增/变更点：

- `corevector.Searcher` 方法签名不变。
- `retrieve-eval` / `retrieve-inspect` CLI 参数不变。
- 新增一个 lexical rebuild CLI，建议命名为：
  - `cmd/lexical-rebuild`
- 新增默认配置项：
  - `rag.search.channels.keyword.enabled_fallback_trgm = true`
  - `rag.search.channels.metadata_title.enabled_fallback_trgm = true`
  - `rag.search.channels.metadata_title.section_weight`
  - `rag.search.channels.metadata_title.document_name_weight`
  - `rag.search.channels.metadata_title.source_file_name_weight`

第一版要求这些配置都有默认值，未配置时行为稳定。

## Test Plan

必须覆盖四层验证。

### 1. 单元测试

- `SearchByKeyword`
  - 中文正文 query 命中相关 chunk；
  - KB 过滤生效；
  - 空 query 返回空结果；
  - fallback 路径可触发。
- `SearchByMetadata`
  - 文件名 query 命中正确文档；
  - section query 优先命中深层 section；
  - 顶层同前缀标题不会稳定压制目标 section。
- channel 层
  - `keyword` / `metadata_title` / `hybrid` 的启停行为不变；
  - 新实现不改变 `search_mode` 路由语义。

### 2. 集成测试

- 新文档导入后 lexical 可立即检索；
- 文档重切 chunk 后旧 lexical 数据不会残留；
- chunk 删除/禁用后 lexical 结果同步消失；
- lexical rebuild CLI 可重建已有 KB 数据。

### 3. 基准评估

使用现有资产做回归：

- `testdata/docs_markdown_samples_v2.json`
- `testdata/docs_markdown_results_keyword_v2.json`
- `testdata/docs_markdown_results_semantic_v2.json`
- `testdata/docs_markdown_results_hybrid_v2.json`

验收关注点：

- `file_name` 任务不明显退化；
- `section_metadata` 的 `Hit@1 / Hit@10 / MRR` 明显提升；
- `hybrid` 至少在 markdown 语料上优于当前版本；
- inspect 之前的典型 miss case，应有若干被 lexical 单独修复。

### 4. 诊断样例回放

固定回放这类样本：

- 长 section 标题 query
- 文件名 query
- 自然语言但不直接复制标题的 query
- 之前 `keyword miss / hybrid hit` 的代表样本

要求输出一页对比总结，说明“修复了哪些 miss，没修复哪些 miss”。

## Assumptions / Defaults

- 采用 **PostgreSQL 内建方案**，本期不引入 Elasticsearch/OpenSearch。
- 本期目标是**替换主 lexical 方案**，不是重构整个检索架构。
- `word_similarity` 不立即删除，只降级为 fallback。
- 第一版优先解决：
  - 中文 section/title 检索弱
  - 长标题被公共前缀淹没
  - metadata_title 对 markdown 结构利用不足
- 当前 `docs_markdown_samples_v2.json` 可作为第一轮验收集，但后续仍应补一批**去标题化手写 query** 做更真实的 keyword 验收。
