# DocReader 文件解析接入

复杂格式（PDF、DOC、DOCX、XLS、XLSX、EPUB）及常见图片优先调用 WeKnora DocReader 的 gRPC `ReadStream`。DocReader 不可用或解析报错时，若配置了 Tika，解析器回退到 Tika。Markdown 和 TXT 直接在 Go 中读取；HTML 等其他格式由 Tika 处理，未配置 Tika 时明确报错。当前已测试的 DocReader 镜像不支持 HTML 文件解析。

## 启动

在本地开发环境运行 `docker compose --profile docreader up -d docreader ocr`。DocReader 绑定本机 `127.0.0.1:50051`；Tesseract OCR 服务绑定 `127.0.0.1:8082`，内置简体中文与英文语言包。生产环境设置 `WEKNORA_DOCREADER_IMAGE` 为经过验证的固定镜像版本或摘要，不使用默认的 `latest`。

本次联调使用本机缓存的 `wechatopenai/weknora-docreader:latest`（镜像 ID `7f09b7581150`）。该镜像的内置引擎未提供 HTML，尽管本地 `D:\WeKnora` 源码中有 HTML 解析器；部署其他镜像版本时应重新运行下方联调测试。

配置 `parser.docreader.address`（环境变量 `PARSER_DOCREADER_ADDRESS`）为服务地址，`parser.docreader.timeout-ms` 为单次解析超时。`parser.ocr.url`（`PARSER_OCR_URL`）及 `parser.ocr.timeout-ms` 控制 OCR 服务。默认地址分别为 `localhost:50051` 和 `http://localhost:8082`。未启动 DocReader 时会尝试回退 Tika；OCR 调用失败会使本次解析失败。

## 旧解析入口的边界

- DocReader 返回的图片帧逐张送入 OCR，识别文字在原图片位置写回 Markdown，然后交给现有分块链路。OCR 的 `success` 返回文字；`no_text` 返回空文字，图片链接不会成为检索内容。纯图片文件没有可识别文字时，文档会以“无可检索文本”失败。图片描述能力尚未接入。
- OCR 一次最多处理 100 张图，每张图限制 10 MiB。OCR 服务或某张图片识别报错时本次解析失败，避免遗漏扫描页后仍显示入库成功。
- `no_text` 仅表示 OCR 未检测到可识别文字，不表示图片经过了语义分类。中文识别也可能有错字；联调样本中“扫描发票金额四十二元”曾识别为“扫质发票金额四十二元”。检索质量要求高的场景需要后续增加识别质量评估或人工校对。
- 单次 gRPC 消息上限为 50 MiB。文件读取仍沿用现有的内存读取方式；大文件输入限制和流式对象存储读取是后续工作。
- 本接入复制了 WeKnora 的 `docreader.proto` 生成的 Go 协议代码，来源见 `internal/app/core/parser/docreaderpb/NOTICE.md`。

## 验证

运行 `go test ./internal/app/core/parser/... ./internal/app/knowledge/service/process ./internal/framework/config`。启动 DocReader、OCR 和 Tika 后，设置 `DOCREADER_LIVE=1` 再运行同一命令，会使用仓库内的 `internal/app/core/parser/test/fixtures/docreader` 样本验证 PDF、DOCX、XLSX、HTML、扫描 PDF、混合 PDF、中文文字图片及空白图片；还会通过旧 `ExecuteChunk` 测试夹具检查同步 OCR 文字进入 chunk 与向量。

## 图片证据入库（2026-09-24）

主服务现使用 `ParseStructured` 读取 DocReader 正文与图片出现清单。图片原图和处理副本写入私有对象存储；OCR 与硅基流动 `Qwen/Qwen3.8-27B` 的图片描述分别由 PostgreSQL 持久任务处理。正文块和问题向量按每次发布生成新 ID，避免旧聊天引用误指重处理后的内容。旧 `Parse` 仍用于兼容测试与直接解析调用，其同步 OCR 行为不代表主入库路径。

主服务优先读取 `PARSER_VISION_API_KEY`，未配置时复用 `.env` 中的 `AI_PROVIDERS_SILICONFLOW_API_KEY`。`parser.vision.url` 默认为 `https://api.siliconflow.cn`，超时和输出上限由 `parser.vision.timeout-ms`、`parser.vision.max-tokens` 控制。不要在配置文件或日志中写入 API Key。没有 Key 时描述任务记录错误，OCR 仍独立处理，文档可能显示 `partial`。

图片证据向量的 `record_type` 为 `image`，内容分别标为“图片 OCR 原文”和“AI 生成的图片描述”，并带有限长度的相邻正文；三个来源保存在独立字段中。聊天 `<kb kind="image">` 引用指向不可复用的证据版本 ID；点击角标通过登录态读取证据和原图 Blob。历史引用保留原证据快照，删除文档后原图接口立即拒绝读取，后台清理原图与处理副本。管理页展示图片总数、已处理数、失败数和逐图状态，可预览原图并重试失败的 OCR/描述任务，包括尚未生成证据版本的任务。描述证据还记录模型、提示词版本、输入/输出 token 数与耗时；失败记录使用受控错误码，不保存服务端响应正文。

DocReader 失败而 Tika 有正文时，文档保持 `partial`，同时写入图片清单重试任务。重试会核对源文件哈希；DocReader 恢复后建立新版本并为每张图片创建 OCR/描述任务。有限次数后仍失败则保留 `partial`，避免把未知图片清单误标为成功。

部署顺序：先运行 SQL 迁移，再部署后端，再配置并启动图片 worker，最后部署前端。回退时先停止 worker；迁移的 Down 有意保留证据表和对象键，避免历史引用丢失。数据库与对象存储之间没有跨系统事务，失败对象由补偿清理任务处理。

联调命令：`DOCREADER_LIVE=1 go test ./internal/app/core/parser/test -run TestDocReaderLiveStructuredImages -v`；`IMAGE_EVIDENCE_LIVE=1 DOCREADER_LIVE=1 go test ./internal/app/knowledge/service/imageevidence -run 'TestImageEvidencePostgresLive|TestImageInventoryRetryLive' -v`；`VISION_LIVE=1 go test ./internal/app/core/vision -run TestSiliconFlowVisionLive -v`。测试夹具验证任务、向量、历史版本、删除撤权和清单恢复；真实硅基流动请求已用文字图片与空白图片验证，运行时仍需有效 Key。
