# 联调阻塞修复 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task with review checkpoints.

**Goal:** 修复 embedding 模型不匹配、远程 URL 上传校验错误和旧后端运行版本问题，并用浏览器完成端到端回归验证。

**Architecture:** 保持按知识库模型选择 embedding 候选的现有架构，只将联调知识库切换到当前已配置模型。URL 上传在服务层按来源类型分别校验，本地文件继续要求文件名和文件体，URL 由远程抓取器推导文件名。后端从当前 HEAD 构建并替换 9090 旧进程。

**Tech Stack:** Go、Gin、PostgreSQL、React/Vite、PowerShell、浏览器联调。

---

### Task 1: 为 embedding 模型解析补回归测试

**Files:**
- Test: `internal/infra-ai/embedding/routing_embedding_service_test.go`（若不存在则创建）

- [ ] **Step 1: 写测试，断言未知模型返回明确错误**

测试构造只包含 `qwen-emb-8b` 的 selector，调用 `EmbedBatchWithModel(..., "text-embedding-3-large")`，断言错误包含 `embedding model unavailable`。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/infra-ai/embedding -run TestRoutingEmbeddingServiceRejectsUnknownModel -count=1 -v`

Expected: 测试能够执行；若当前实现已返回该错误则记录为行为已覆盖，否则应因错误为空或错误内容不符而失败。

- [ ] **Step 3: 只在必要处补充错误上下文**

保持 `resolveTarget` 的模型选择逻辑不变；如果测试暴露错误被路由层吞掉，则让错误保留模型 ID，不改变 fallback 语义。

- [ ] **Step 4: 运行 embedding 测试确认 GREEN**

Run: `go test ./internal/infra-ai/embedding -count=1`

Expected: PASS。

### Task 2: 修复 URL 上传的来源分支校验

**Files:**
- Modify: `internal/app/knowledge/service/document/knowledge_document_command_service.go:36-44`
- Test: `internal/app/knowledge/service/document/knowledge_document_service_test.go`

- [ ] **Step 1: 写 URL 上传缺少 fileName 仍可继续的测试**

测试传入 `SourceType=url`、有效 `SourceLocation`、空 `FileName` 和空 `Body`，使用远程抓取 stub 返回文件信息，断言上传成功且文档名称来自远程结果。

- [ ] **Step 2: 运行测试确认 RED**

Run: `go test ./internal/app/knowledge/service/document -run TestKnowledgeDocumentServiceUploadURLWithoutFileName -count=1 -v`

Expected: FAIL，错误为 `file name is required` 或 `file body is required`。

- [ ] **Step 3: 将校验移动到本地文件分支**

在 `Upload` 中先规范化来源类型；`file` 继续校验 `FileName`、`Body`、`Size`，`url` 只校验 `SourceLocation`，再调用已有的 `buildRemoteKnowledgeDocument`。

- [ ] **Step 4: 增加本地文件校验回归测试并运行包测试**

保留本地文件缺少文件名/文件体的失败断言，运行：

`go test ./internal/app/knowledge/service/document -count=1`

Expected: PASS。

### Task 3: 对齐联调知识库并重建后端

**Files:**
- Runtime data: PostgreSQL `t_knowledge_base` row `kb_demo_01`
- Build output: `tmp/goagent-server-latest.exe`

- [ ] **Step 1: 只读确认当前值**

Run: `select id, embedding_model from t_knowledge_base where id = 'kb_demo_01';`

Expected: `text-embedding-3-large`。

- [ ] **Step 2: 更新联调知识库模型**

Run the approved read/write database operation to set only `kb_demo_01.embedding_model` to `qwen-emb-8b`, then query the row again.

- [ ] **Step 3: 构建当前 HEAD**

Run: `go build -o tmp/goagent-server-latest.exe ./cmd/server`

Expected: exit code 0。

- [ ] **Step 4: 停止旧 9090 进程并启动新二进制**

确认监听 PID 后停止该 PID，在隐藏窗口启动 `tmp/goagent-server-latest.exe`，将 stdout/stderr 写入 `tmp/goagent-server-latest.out.log` 和 `.err.log`。

- [ ] **Step 5: 验证运行版本和健康状态**

确认 9090 重新监听，进程构建版本包含当前 HEAD revision，并检查 `/health` 或 `/ready` 返回成功。

### Task 4: 浏览器端到端回归

**Files:**
- No source changes.

- [ ] **Step 1: 在已登录管理后台重新分块 Markdown 文档**

确认 `2026-08-09-wiki-p4-design.md` 状态为 `success` 且分块数大于 0，分块详情不再出现 embedding failure。

- [ ] **Step 2: 发起 Wiki 生成**

在聊天页请求基于成功文档生成 Wiki，等待流结束，确认页面标题和写入结果返回。

- [ ] **Step 3: 检查 Wiki 页面和图谱**

进入 `kb_demo_01` Wiki 页面，确认页面列表出现新页面；切换图谱确认节点/边数据与页面链接一致。

- [ ] **Step 4: 回归远程 URL 上传**

使用公开 Markdown URL 上传，确认不再出现 `file name is required`，并记录后续抓取/分块状态。

### Task 5: 汇总验证

- [ ] **Step 1: 运行定向 Go 测试**

Run: `go test ./internal/infra-ai/embedding ./internal/app/knowledge/service/document ./internal/adapter/http/knowledge/... -count=1`

- [ ] **Step 2: 检查工作区差异**

Run: `git diff --check` and `git status --short`。

- [ ] **Step 3: 汇报运行时状态**

明确区分代码修复、数据库联调数据变更、后端进程重启和浏览器验证结果；不覆盖原有 Vite 缓存改动。
