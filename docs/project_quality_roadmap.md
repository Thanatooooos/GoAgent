# GoAgent 高质量项目路线图

> 目标：将 goagent 从一个"功能可用"的 MVP 演进为代码规范、流程完备、能力深厚的生产级 RAG 平台。

---

## 一、代码规范 —— 让每一行代码都经得起审视

### 1.1 零警告、零丢弃错误

**当前问题**：约 189 处 `_ =` 丢弃错误，涵盖数据库写入、SSE 发送、文件清理。

**目标状态**：
- 所有 `_ =` 仅限于两类场景：`defer Close()` 和无副作用的读操作
- 任何写操作（DB write、storage delete、SSE send）的错误必须记录日志或向上传播
- CI lint 规则将 `_ =` 设为 error 级别，逐文件解禁

**度量**：`grep -r "_ =" internal/ | wc -l` → 归零（排除 defer close 和明确标记的安全忽略）

### 1.2 统一错误处理范式

**目标状态**：
- 全项目统一使用 `fmt.Errorf("context: %w", err)` 做错误包装
- 移除 `github.com/pkg/errors` 依赖，Go 1.13+ 标准库已完全覆盖
- 业务层自定义错误类型（`ServiceException` 等）统一实现 `Unwrap()` 方法，使 `errors.Is` / `errors.As` 可穿透
- 每个 API 的错误响应格式一致，错误码有明确语义

### 1.3 `any` 替换 `interface{}`

**当前问题**：18 处 `interface{}` 残留，且分布在不同包。

**目标状态**：全项目 0 处 `interface{}`，CI 添加 `go vet` 检查。

### 1.4 消除代码重复

**当前问题**：`timePointer` ×7、`writeSuccess` ×4、`parsePositiveInt` ×3、`pageResult` ×3、context-path 路由逻辑 ×4。

**目标状态**：
- 所有公共 helper 提取到 `internal/framework/convention/` 或新建 `internal/framework/httputil/`
- 重复出现 3 次以上的模式必须提取
- Code review 阶段用工具（如 `jscpd` 或 `golangci-lint` duplicate check）自动检测

### 1.5 测试体系

**当前状态**：有单元测试和 3 个集成测试（需手动设置环境变量），无 CI。

**目标状态**：
- **单元测试覆盖率 ≥ 70%**：service 层为核心
- **集成测试自动运行**：通过 `docker compose up -d` 拉起依赖，执行测试，`down` 清理
- **失败路径测试**：每个 service 至少有 1 条 happy path + 1 条 failure path
- **测试数据隔离**：每次测试使用独立 schema 或随机前缀，互不污染

---

## 二、流程编排 —— 从手工到自动化

### 2.1 CI/CD 流水线

**当前状态**：无任何 CI/CD。

**目标状态**：

```
PR 提交
  ├── lint (golangci-lint + eslint)
  ├── test (go test ./... + vitest)
  ├── build (go build + vite build)
  ├── integration test (docker compose up → test → down)
  └── (merge 后) deploy to staging
```

**实现步骤**：
1. 添加 `.github/workflows/ci.yml`，覆盖 lint + unit test + build
2. 添加 `Makefile`，统一本地命令入口：`make lint`、`make test`、`make test-integration`
3. 接入 `docker compose` 集成测试，确保每次 PR 验证完整链路

### 2.2 数据库 Migration 规范化

**当前状态**：三种建表方式并存（SQL migration / AutoMigrate / Docker init SQL）。

**目标状态**：
- 全项目统一使用 [goose](https://github.com/pressly/goose) SQL migration
- 启动时由代码自动执行未应用的 migration（`goose up`）
- 所有 AutoMigrate 调用移除
- Docker init SQL 与 migration 文件由同一源生成，保证一致

### 2.3 配置管理分层

**当前状态**：单一 `application.yaml` + `.env` 覆盖，配置项平铺。

**目标状态**：
```
configs/
  application.yaml           # 默认值（可提交）
  application-dev.yaml       # 开发环境覆盖
  application-staging.yaml   # 预发布覆盖
  application-prod.yaml      # 生产覆盖（敏感值通过 env/KMS）
```

- 配置结构按模块分组，消除平铺
- 生产敏感值（API key、DB 密码）不出现在任何 YAML 中
- 增加配置校验：启动时检查必填项，缺失则明确报错而非运行时 panic

### 2.4 优雅关闭与健康检查

**当前状态**：`r.Run(addr)` 永久阻塞，无信号处理。

**目标状态**：
```
启动 → 健康检查就绪 → 接收流量
收到 SIGTERM
  → 停止接收新请求
  → 等待现有请求完成（最长 30s）
  → ingestion executor 等待 workflow 完成
  → schedule loop 释放分布式锁
  → 关闭 DB 连接池
  → 退出
```

- `/health` 返回就绪状态（DB 连通、AI 服务可达）
- `/health/live` 返回存活状态（轻量级）
- Kubernetes 可基于健康检查做滚动更新

---

## 三、RAG 功能深度 —— 从"能跑"到"好用"

### 3.1 检索质量闭环

**当前状态**：有 embedding + vector search + 可选 rerank，但无质量度量。

**目标状态**：

```
检索质量度量体系：
  ├── Recall@K     — 召回率
  ├── MRR          — 平均倒数排名
  ├── NDCG@K       — 归一化折损累计增益
  └── 人工标注测试集（至少 50 组 query / relevant-docs）

每次检索策略变更 → 跑评估 → 对比基线报告
```

**实现路径**：
1. 新建 `internal/app/rag/evaluation/` 评估模块
2. 定义测试集格式（JSON/YAML），包含 query、expected doc IDs、min recall
3. 评估 runner 对比不同 retriever 配置（topK、rerank、chunk size）的效果
4. CI 中加入评估步骤，回归检测

### 3.2 多轮对话上下文管理

**当前状态**：有 memory service（history + summary），但实现简单。

**目标方向**：
- **上下文窗口预算管理**：不是简单截断历史，而是按 token 预算动态决定携带多少轮历史
- **摘要压缩策略**：当历史超过 N 轮时自动触发摘要压缩，摘要 + 最近 K 轮原文
- **对话分支**：支持用户从某条消息重新生成，创建对话分支
- **上下文注入标记**：区分 system prompt / history / retrieved chunks / user query，模型可感知各段来源

### 3.3 检索策略增强

**当前状态**：embedding search + 可选 rerank。

**目标方向**：
- **混合检索**：BM25（关键词）+ Dense（语义）混合，支持权重调节
- **Query 扩展**：对短 query 做 HyDE（假设文档嵌入）或 LLM query 扩展
- **分块策略自适应**：根据不同文档类型（PDF 论文、Markdown 文档、代码文件）自动选择最优 chunker
- **多路召回融合**：从不同 knowledge base 检索，按相关性合并排序

### 3.4 可观测对话质量

**当前状态**：有 trace run/node 记录，有 vote feedback。

**目标方向**：
- **端到端链路追踪**：一次 chat 请求的完整 trace（rewrite → retrieval → rerank → prompt assembly → LLM call → post-process），每步记录耗时和 token
- **质量仪表盘**：反馈分布（👍/👎 比例）、平均检索延迟、平均首 token 时间
- **Bad Case 自动采样**：👎 反馈自动关联完整 trace，便于人工分析

---

## 四、Ingestion 管道 —— 从能跑到可靠

### 4.1 幂等与重试

**当前状态**：`indexer` 已写入 knowledge 下游，但幂等策略不完整。

**目标状态**：
- 每个 node 支持**至少一次**执行语义：node 重跑时不会产生重复 chunk/vector
- indexer 写入前先检查：该文档的 chunk 是否已有同 content-hash 的记录，有则跳过
- 失败自动重试：node 级别失败后，最多重试 3 次，指数退避

### 4.2 管道可恢复性

**当前状态**：task 失败后状态落 `failed`，无从断点恢复。

**目标方向**：
- Task 支持 **从失败 node 重试**，而非重跑整个 pipeline
- Pipeline 定义中声明每个 node 的幂等性和可重试性
- 支持人工审批节点：关键步骤（如发布到生产 KB）需要人工确认

### 4.3 管道可观测性

**当前状态**：有 task/task_node 记录和 knowledge 页面联动，缺少系统指标。

**目标状态**（对齐 project_progress_context.md P0 计划）：
- task 总耗时分布（P50/P95/P99）
- 各 node 耗时分布（识别瓶颈在 fetcher/parser/chunker/indexer）
- 运行中并发数（用于调优 `max-concurrent`）
- 失败率趋势（按 source type、document type 分组）
- 下游延迟（embedding API、vector store、storage）

### 4.4 Feishu 来源支持

**当前状态**：`feishu` 停留在接口层，无真实拉取实现（对齐 project_progress_context.md 已知问题）。

**目标**：实现飞书文档拉取 → Tika 解析 → pipeline 处理完整链路。

---

## 五、安全基线 —— 纵深防御

### 5.1 密钥管理

**当前状态**：`.env` 中硬编码真实 API key。

**目标状态**：
- 开发环境：`.env` 仅含占位符，真实密钥从本地密钥链或 1Password CLI 注入
- 生产环境：通过云 KMS 或 Kubernetes Secrets + External Secrets Operator 注入
- CI/CD：密钥通过 GitHub Secrets 或 Vault 注入，不出现在日志中
- **立即轮换已泄露的 Bailian 和 Siliconflow API key**

### 5.2 输入边界防护

**目标状态**：
- 全局请求体大小限制（`MaxBytesReader`）
- 文件上传校验：MIME type + 文件魔数 + 大小限制
- Path 参数格式校验（ID 格式、长度范围）
- URL fetcher 的 SSRF 防护（IP 白名单，拒绝内网地址）
- 文件路径访问限制到白名单基础目录

### 5.3 认证与授权加固

**当前状态**：X-Login-Id 头可直接指定登录用户。

**目标状态**：
- 去除 `X-Login-Id` 提取逻辑（或仅 `demo-mode=true` 时启用）
- Debug 路由添加认证中间件
- 会话 Token 支持过期/刷新机制
- 密码策略升级：≥8 位，含大小写 + 数字 + 特殊字符

### 5.4 运行时防护

**目标状态**：
- CORS 中间件，配置白名单来源
- 安全响应头中间件（CSP、HSTS、X-Frame-Options、X-Content-Type-Options）
- 请求频率限制（rate limit），防止 API 滥用
- PostgreSQL SSL 连接，生产环境强制加密

---

## 六、可观测性 —— 从日志到全链路

### 6.1 结构化日志

**当前状态**：使用 zap，但多处使用 `log.Println`（标准库，非结构化）。

**目标状态**：
- 全局统一使用 zap，移除 `log.Println`
- 每条日志携带：`requestId`、`userId`、`traceId`
- 关键操作日志携带业务维度：`knowledgeBaseId`、`documentId`、`taskId`

### 6.2 Metrics 指标

**当前状态**：无。

**目标状态**：
- 接入 Prometheus + Grafana
- 基础指标：HTTP 请求量/延迟/错误率（RED metrics）
- 业务指标：chat 请求量、检索延迟、token 消耗、ingestion task 吞吐
- 基础设施指标：goroutine 数、内存使用、DB 连接池使用率

### 6.3 告警规则

**目标状态**：
- 错误率 > 1% 告警
- P95 延迟 > 5s 告警
- AI API 调用失败率 > 5% 告警
- DB 连接池耗尽告警
- Ingestion task 失败堆积告警

---

## 七、文档体系 —— 让新人一天上手

### 7.1 必须文档

| 文档 | 内容 | 状态 |
|------|------|------|
| `README.md` | 项目简介、快速开始、架构概览 | 缺失 |
| `CONTRIBUTING.md` | 开发规范、提交流程、代码审查清单 | 缺失 |
| `docs/architecture.md` | 领域模型、模块边界、数据流 | 部分（设计文档分散） |
| `docs/api.md` | 完整 API 文档（可基于 OpenAPI 生成） | 缺失 |
| `docs/deployment.md` | 部署步骤、环境变量、配置说明 | 缺失 |

### 7.2 Code Tour

**目标**：新开发者读完 `README.md` + 一个 `docs/architecture.md` 后，能在 1 天内完成第一个小改动。

---

## 八、分阶段实施路线

### Phase 1：止血（1-2 周）

- [ ] 轮换已泄露的 API key
- [ ] Debug AI 路由加认证
- [ ] 修复路径穿越漏洞
- [ ] SSRF 防护
- [ ] 所有 goroutine 加 `recover()`
- [ ] 修复 context.WithTimeout 泄露
- [ ] `.env` 从磁盘移除真实密钥

### Phase 2：夯实基础（2-4 周）

- [ ] 接入 CI（lint + test + build）
- [ ] 添加 Makefile
- [ ] 数据库 migration 统一为 goose
- [ ] 优雅关闭
- [ ] CORS 中间件
- [ ] 统一 `any` 替换 `interface{}`
- [ ] 提取公共 helper，消除代码重复
- [ ] 废弃依赖清理

### Phase 3：质量提升（4-8 周）

- [ ] 测试覆盖率提升到 70%
- [ ] 集成测试接入 CI
- [ ] 失败路径测试补齐
- [ ] 结构化日志统一
- [ ] 错误处理范式统一
- [ ] Ingestion 事务补齐
- [ ] 无分页查询增加默认上限

### Phase 4：RAG 深度（8-12 周）

- [ ] 检索质量评估框架 + 测试集
- [ ] 混合检索（BM25 + Dense）
- [ ] 上下文窗口预算管理
- [ ] 摘要压缩策略
- [ ] 检索策略 A/B 对比

### Phase 5：生产就绪（12-16 周）

- [ ] Prometheus + Grafana 接入
- [ ] 告警规则配置
- [ ] 安全响应头
- [ ] 请求频率限制
- [ ] 完整文档体系
- [ ] 性能压测与调优
- [ ] Feishu 来源支持
- [ ] EINO 编排层接入
