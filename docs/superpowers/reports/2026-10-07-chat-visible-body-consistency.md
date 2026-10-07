# Chat / Work 完整正文保存与展示一致性

日期：2026-10-07。范围：成功完成的普通 Chat 与 Work，包含工具调用轮的可见正文、最终发布、恢复及历史展示。代码已修改，未提交或部署。

## 问题与结果

原实现将各模型轮次的正文流式追加到同一条页面消息，但最终助手消息只发布最后一个无工具调用轮次的正文。长消息处理还可能将 `Content` 保存为摘要、原文保存到 `RawContent`，而历史页面只展示摘要。

现在 `Runtime.Run` 在最后一个有效回答轮次结束后，按 journal sequence 汇总本次执行的 `answer_delta`。这些片段已经完成引用展开，汇总结果用于 `answer_final` 与最终发布。thinking、工具结果和其他执行事件不参与汇总；本轮模型的 assistant/tool 输入协议及取消、失败门槛保持原语义。

发布事务的 completed journal 增加 `content`，使用实际消息的完整展示正文。普通 Chat 和 Work 的前端完成处理以该正文替换当前流式正文；旧完成事件没有此字段时保留现有内容。重试发布复用已保存的完成事件和消息 ID，不重跑模型。

消息领域的 `DisplayContent` 优先原文，缺少有效原文时使用 `Content`。发布返回值、完成事件及来源提取使用该正文。普通 Chat 历史页面读取已有 `rawContent` 字段，Work 列表和单条“原始讨论依据”查询展示原文。模型历史仍读取 `Content`，继续使用摘要。没有数据库迁移或历史批量回填。

断线重连验证同时发现普通 Chat 的 URL 将已有 `?` 的 `buildQuery` 再加一次 `?`，现已修正为单个查询分隔符。

## 验证

- runtime 多轮工具调用回归：多轮正文按顺序汇总，thinking/工具结果不混入，模型协议仍保留独立 assistant/tool 消息。知识库引用在多个轮次中的流式与最终正文一致。
- 消息处理回归：摘要不带引用时，原文和来源仍完整保存。
- SSE 回归：完成正文和旧事件兼容；前端实际 SSE reader + Zustand 三个用例验证重复/缺失片段校准、原文重载、旧事件及断线重连。可独立运行 `node --test frontend/tests/chat-body.test.mjs`，需已安装前端依赖。
- 新建隔离库 `codex_work_20261007_chatbody`：普通 Chat 和 Work 的实际 PostgreSQL 持久化、注入发布失败后的恢复、重复发布、完成 SSE 及正式 HTTP 历史接口验证通过。Work turn 和消息 ID 一致，模型历史仍使用摘要。
- Work 仓储和 bootstrap 完整套件在该专用库串行通过，包含发布故障、并发恢复、真实子进程退出和取消/删除等既有回归。子进程 helper 在普通测试进程跳过，实际父用例调用子进程通过。
- runtime、conversation、runtime adapter、Chat HTTP 等相关 Go 套件通过；`go test ./cmd/... ./internal/... -run '^$'` 全仓编译通过，未宣称全仓功能套件全部执行。
- 前端 build 通过。全量 TypeScript 检查仍退出 2；使用编译器读取覆盖对比本轮相关 TypeScript 修改前后，均 42 条诊断，新增 0。比较保留了修改前已有的未提交代码，不以 Git HEAD 作为基线。

本次使用可控模型及摘要处理器配合真实数据库/HTTP，未进行外部真实模型或浏览器视觉验收。没有重启或替换应用服务。证据在 `tmp/chat-body-*.log`，持续回归以源码中的测试为准。测试依赖启动了已安装的 Docker Desktop；所有数据库测试仅使用上述专用库。

交付核对：本次承诺的成功执行正文一致性为 **aligned**。历史缺失的过程正文未批量回填；失败或取消的执行仍不会自动发布部分正文。thinking 开关和工具状态实时推送属于原定范围之外。
