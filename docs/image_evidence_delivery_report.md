# 图片证据入库实施与验收记录

更新：2026-09-26

## 已接入的运行路径

- `cmd/server` 装配 DocReader 结构化解析、OCR、硅基流动 `Qwen/Qwen3.8-27B`、图片任务 worker、鉴权读取接口和管理接口。
- 正文发布后，图片原图及处理副本写入私有对象存储；每张图独立执行 OCR 和描述，结果以不可复用的证据版本 ID 写入向量表。
- 图片描述、OCR 原文和有限长度的相邻正文分别保存并标记来源。聊天图片引用通过证据 ID 读取原图 Blob；管理页支持查看原图与重试失败的处理项。
- DocReader 回退到 Tika 时保留 `partial` 与持久化图片清单重试。重处理使用版本标记，旧任务不能覆盖新文档版本。正文块也使用每次发布的新 ID，避免历史引用误指新内容。
- 删除文档后立即撤销证据读取，后台清理原图和处理副本。模型与 OCR 返回错误按受控类别记录，不保存服务返回正文或密钥。

## 已通过的验证

- Go 全量编译：`go test ./cmd/... ./internal/... -run '^$'`。
- 真实 PostgreSQL + DocReader + OCR 主入口：`IMAGE_EVIDENCE_LIVE=1 DOCREADER_LIVE=1 go test ./internal/app/knowledge/service/imageevidence ./internal/app/knowledge/service/process -run 'TestImageEvidencePostgresLive|TestImageInventoryRetryLive|TestImageEvidenceMainEntrypointLive' -v`。覆盖图片任务、OCR 先发布、向量切换、历史证据、删除撤权、回退清单恢复、普通 PDF 正文块持久化和旧版本写回栅栏；夹具事务回滚。
- 真实硅基流动调用：`VISION_LIVE=1 DOCREADER_LIVE=1 go test ./internal/app/core/vision -run 'TestSiliconFlowVisionLive|TestSiliconFlowExtractedPDFImageLive' -v`。文字图片得到描述，空白图得到 `no_content`，混合 PDF 提取出的 JPEG 得到描述；测试未输出密钥或图片内容。
- 真实 RustFS 上传、读取、删除：`RAG_INTEGRATION_RUSTFS=1 go test ./internal/adapter/storage/s3/test -run TestFileStorageUploadOpenDeleteIntegration -v`。
- 前端 `npx vite build` 通过。结构化解析 101 张图片、重复引用、缺失图片帧、引用注册表防伪造等单测通过。`npx tsc --noEmit` 在当前多项目 TypeScript 配置下不会检查应用；实际 `npx tsc -b` 仍报出多处现存类型错误（聊天、日报、trace 等），不能据此宣称全量类型检查通过。

## 浏览器全链路验收（2026-09-26）

- 使用本机 Edge 的 Playwright 自动化登录 `admin`，在独立测试知识库 `image-e2e-20260926` 上传 `scanned.pdf`，从页面启动分块，等待图片任务完成。文档最终为 `success`，图片 `1/1` 完成、失败 `0`；OCR 为 `success`，描述为 `described`，模型记录为 `Qwen/Qwen3.8-27B`。
- 管理页“图片处理详情”能分别显示 OCR 和图片描述状态，“查看原图”成功加载原图（浏览器 `naturalWidth=1500`）。图片向量含 OCR 原文 `SCANNED INVOICE 2026` 与显式标记的 AI 生成描述。
- 在聊天中选择该知识库，实际请求携带其 ID，运行时调用 `retrieve_knowledge`。回答出现图片引用；点击引用后，浏览器直接显示同一原图（宽 1500 像素）、OCR 原文与“AI 生成的图片描述”，来源可回溯。模型回答把标题中的空格漏掉，引用弹层中的 OCR 原文准确，说明最终生成措辞仍需以原图和 OCR 核验。
- 验收中发现文档页残留数据通道变量导致运行时白屏；已移除不再存在的数据通道表单残留并恢复页面。图片向量新增文件名、文档版本、图片出现 ID 及 OCR/描述状态元数据，使后续生成的引用显示实际文件名并满足规格的溯源字段。前端 Vite 构建及相关 Go 测试通过。
- 首次 `mixed.pdf` 测试在正文 embedding 阶段等待外部模型时，隔离后端测试进程到达默认 10 分钟超时；该测试文档已清理。它未形成完整的混合正文加图片浏览器验收，需在模型服务稳定后复测。扫描版 PDF 的图片独立链路已完整验收。

## 尚需上线前人工核对

- 在模型服务稳定后，补做混合正文与图片 PDF 的浏览器联调；本次已走通扫描版 PDF 的上传、图片 OCR 与描述、检索回答、引用和原图展示。
- 用业务代表性的图表、流程图和照片人工评估描述质量；现有真实模型探针证明接口可用，但不代表这些类型的描述质量已验收。
- 服务重启、双 worker 竞争和对象存储故障恢复已有租约、版本栅栏和补偿代码；仍建议在预发布环境做故障注入演练。

## 规格交付审计

结论：**部分对齐**。扫描版 PDF 的正式入口从上传、分块、OCR/描述任务、图片向量、聊天检索到鉴权原图展示已实际走通；迁移、对象存储和默认模型配置也在运行路径中。阶段 8 要求的混合 PDF、图表/照片、100+ 图片等样本的真实浏览器链路及故障演练仍未全部完成，实施计划中的对应勾选项应保持未完成。当前正文 embedding 在图片登记之前执行，外部 embedding 长时间阻塞时，混合文档的图片任务也无法先发布；本次 `mixed.pdf` 测试暴露了这一风险。上线验收需解决或明确这一时序与规格“正文和 OCR 可先检索”的关系，并补齐混合文件的真实联调。

工作区已有大量其他未提交改动；本次实施没有重置或提交它们。

## 续验收记录（2026-09-26）

- 修正状态收口：图片 OCR/描述任务仍在处理时，文档保持 `running`；任务终结后按已发布内容和错误收口。远程刷新时调度器只会在文档仍为 `running` 且图片数量为零的同一条条件更新中标记 `success`；已登记图片的最终状态由图片 worker 决定，避免并发下提前收口。
- 真实 PostgreSQL、DocReader、OCR 主入口联测通过：`text.png` 入队后为 `running`，两项任务完成后为 `success`；`mixed.pdf` 在正文向量已发布、4 项图片任务待处理时为 `running`，处理后同时有正文和图片向量并为 `success`；`blank.png` 的 OCR 无文字且描述为 `NO_CONTENT` 时无向量，最终为 `failed`。该联测的正文和图片 embedding、图片描述使用受控替身，事务结束回滚测试数据。
- PostgreSQL 图片任务联测通过：OCR 已发布而描述未完成时为 `running`；描述失败时为 `partial`，OCR 向量保留；仅重试描述后恢复 `success`。这项联测的 OCR、描述和 embedding 使用受控替身。
- `go test ./internal/app/knowledge/... ./internal/app/core/parser/... ./internal/app/core/vision/... -count=1`、`go test ./cmd/... ./internal/... -run '^$' -count=1` 和前端 `npm run build` 通过；真实数据库与 DocReader/OCR 联测及解析样本检查通过。前端构建首次受沙箱 `spawn EPERM` 阻断，在授权环境重跑通过。真实硅基流动探针沿用此前已通过的记录，本次组合测试未设置 `VISION_LIVE=1`，因此没有重复调用平台。
- 阶段 8 仍未全部通过：浏览器端混合 PDF 检索及引用、图表/流程图/照片描述质量、100+ 图片的真实全链路、故障注入与迁移回退演练仍需补齐。混合文档的正文 embedding 先于图片登记；外部 embedding 阻塞时会延迟图片任务入队。现有 OCR 不再进入文档摘要输入，规格要求的这项能力仍待实现和验收。结论继续保持**部分对齐**。
- 本次尝试重新启动本地后端和 Vite 前端以补做混合 PDF 浏览器验证；两项服务正常启动，但 computer-use 浏览器清单返回 `nodeRepl.fetch request failed`，未能产生新的浏览器证据。启动的服务随后已停止。

## 修复与复验（2026-09-27）

- 混合文档在结构化解析后立即登记待发布图片版本并创建 OCR/描述任务。正文 embedding 可以与图片任务并行；正文及文本向量成功持久化后，才在同一数据库事务内切换活动版本。新版本失败时旧正文和旧图片向量不被清除。已在真实 PostgreSQL + DocReader + OCR 联测中用阻塞 embedding 和受控 embedding 错误验证。
- OCR 在待发布版本完成时先保存独立证据，不提前暴露图片向量；版本切换后排入图片索引任务。切换后立即结算文档状态，避免图片任务在切换前已完成而文档一直停留在 `running`。
- OCR 全部结束且存在识别文字时，单独排入文档摘要任务。摘要只使用正文和 OCR 原文，不使用 AI 图片描述；结果写回文档摘要及正文向量的摘要元数据，不生成或改写问题向量。摘要任务按 OCR 内容指纹去重，并在写回时校验当前活动版本与指纹；失败只标记摘要降级。
- `TestImageEvidenceMainEntrypointLive`、`TestOCRSummaryLive`、`TestImageEvidencePostgresLive` 和 `TestImageInventoryRetryLive` 分包顺序运行通过。摘要联测覆盖混合 PDF 与纯图片文档，核对真实 OCR 输入、排除描述文本、正文向量元数据和问题向量数量。上述联测使用真实数据库、DocReader、OCR，图片描述、embedding 和摘要模型使用受控替身；本轮没有重复浏览器全链路或真实视觉模型调用。
- 修复了此前记录的两处代码差距。阶段 8 仍为**部分对齐**：混合 PDF 的浏览器检索与引用、图表/流程图/照片质量、100+ 图片真实全链路及预发布故障演练尚待完成。
- 用户启动全部进程后，`http://localhost:5173/` 返回 `200`，后端受保护知识库接口返回预期 `401`，确认前后端可访问。Codex 浏览器操控和 Windows Computer Use 的 Node 运行时均在初始化时返回“系统找不到指定的路径 (os error 3)”；重置后复试仍失败。本次未执行浏览器上传、检索引用与原图展示，待工具运行时恢复后继续。
