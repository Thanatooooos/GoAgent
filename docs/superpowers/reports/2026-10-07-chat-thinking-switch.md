# 普通 Chat 深度思考开关接入

日期：2026-10-07。用户要求：启用前端深度思考开关时，后端相应启用模型 thinking。代码已修改，未提交或部署。

## 行为与范围

普通 Chat 的 POST 请求现在接收布尔 `deepThinking`，GET 兼容入口也支持布尔查询参数。缺省或 false 表示关闭，非法值拒绝。开关通过 `ChatInput`、`RunRequest` 传到每轮正常回答模型请求，包括工具调用之后的轮次。已 admission 的输入不能再改变该开关。

`ModelRequest.Thinking` 是可选的每次调用设置；普通 Chat 明确设置 true/false，模型适配器在本地变量中解析它，不修改共享实例。没有覆盖值的摘要、意图或定时任务调用继续使用原适配器默认值。硅基流动 native 请求显式发送 `enable_thinking` 布尔值，开启时解析 `reasoning_content` 并经既有 journal / SSE / 前端 thinking 通路展示；关闭时解析器忽略 reasoning。

本次不增加 thinking 历史消息回填或最终消息字段持久化，也不为 Work 新增开关。完整正文发布仍独立于 thinking；没有数据库迁移。

## 验证

- HTTP 参数测试：POST 开启/关闭/省略及非法类型，GET 开启/关闭及非法值。
- Chat admission 测试：设置从 admission 传到 RunRequest，admission 后改值被拒绝。
- runtime 测试：同一次执行的每轮模型调用均收到正确设置。
- 模型适配器并发测试：同一共享实例处理 20 个交替开启/关闭的请求，设置互不污染；false 可覆盖 true 默认值。
- 正式 Chat HTTP → 实际 native 模型客户端 → 本地模拟 OpenAI 风格模型服务 → 两轮工具循环 → SSE：在专用库 `codex_work_20261007_chatbody` 分别验证开启、关闭和省略。开启时独立发送并保存两段 thinking journal，关闭时即使模拟服务返回 reasoning 也不展示；每轮模型请求具有正确的显式布尔设置，完整答案和完成正文一致。
- 相关 runtime、模型适配器、Chat HTTP、infra-ai chat 套件通过。专用库正文发布、恢复、故障、并发和子进程退出回归通过。全仓 `go test ./cmd/... ./internal/... -run '^$'` 编译通过。
- 前端真实 SSE reader / Zustand 测试新增开关请求和独立 thinking 展示用例，全部 4 个用例通过。前端产品代码无需修改，未新增依赖。

上述自动化验证使用本地模拟模型服务及真实隔离数据库。随后按用户授权完成一次外部真实模型及浏览器联调，详情见下节；未重启原应用服务。证据见 `tmp/chat-thinking-*.log`，稳定测试见对应源码及 `frontend/tests/chat-body.test.mjs`。

## 真实模型与浏览器联调

2026-10-07 19:25（Asia/Shanghai），从当前代码编译并启动隔离后端 9091 和 Vite 前端 5175，连接 `codex_work_20261007_chatbody`，启用数据库身份校验，关闭定时任务及画像观察。专用普通用户 `thinking-live-1007` 从正式 Chat 页面勾选深度思考，发送一次无需工具的概率题。运行模型为正式 bootstrap 固定的 SiliconFlow `deepseek-ai/DeepSeek-V4-Flash`，使用现有真实模型配置；未替换模型客户端或返回内容。

- 前端代理观察到一次 POST `/api/ragent/rag/v3/chat`，`deepThinking=true`。只记录该字段和会话标识，不记录请求凭据。
- 后端完成一个模型轮次，`thinkingBytes=407`、`contentBytes=296`、`toolCalls=0`、`finishReason=stop`。本次没有追加关闭模式的真实模型调用，关闭和缺省行为由前述模拟协议测试覆盖。
- 122 段 thinking journal 与独立 `type=think` SSE 拼接内容一致；110 段 answer journal 与 `type=response` SSE 拼接内容一致。
- 浏览器确认完成后思考面板默认折叠，点击可展开实际模型 reasoning，正文独立给出 `2/5`。展开截图为 `tmp/chat-thinking-live-thinking.jpg`。
- SSE 正文、finish 正文、数据库展示原文以及刷新后页面“复制内容”的正文逐字一致；runtime session 为 completed。检查结果为 `tmp/chat-thinking-live-verification.json`，SSE 为 `tmp/chat-thinking-live-stream.sse`，脱敏轮次证据为 `tmp/chat-thinking-live-model-audit.jsonl`。首次实时页面复制的立即读取遇到异步剪贴板尚未更新，该空文件不作为验收证据；刷新后等待复制完成的采集有效。
- thinking 不混入最终消息正文；刷新后没有思考卡片，因为本次没有新增最终消息 thinking 字段回填。刷新截图为 `tmp/chat-thinking-live-reloaded.jpg`。

本次页面还出现既有 sample-questions / approval-pending 路由 404，以及普通用户读取知识库列表 403，未阻止此次请求或历史正文加载；这些入口问题未在本次修改范围内修复。联调结束后仅停止本次启动的预览进程，保留隔离库及证据，未部署原环境。

本次开关接通承诺核对为 **aligned**。thinking 在重新加载历史会话后恢复展示及 Work thinking UI 仍是独立后续范围。
