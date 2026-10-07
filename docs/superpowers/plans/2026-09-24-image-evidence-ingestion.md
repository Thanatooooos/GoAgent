# 图片证据入库与视觉描述实施计划

**依据：** [图片证据入库与视觉描述规格](../specs/2026-09-24-image-evidence-ingestion-design.md)  
**目标：** 文件内嵌图片和独立图片分别完成 OCR 与视觉描述，形成可检索、可回溯原图的独立证据；允许部分失败并可靠恢复。  
**范围：** Go 后端、PostgreSQL 迁移、对象存储、检索与引用、React 管理页和聊天引用、联调与运维配置。  
**首版不做：** 人工修正、外部链接图片下载、多模态聊天回答、依据 AI 图片描述生成摘要或预测问题。

## 实施约束与基线

- 只修改本计划涉及的文件；工作区可能包含其他未提交改动，实施前记录目标文件状态，不能用全仓重置覆盖已有工作。
- 保留 `cmd/server → knowledge runtime → DocumentProcessService.ExecuteChunk` 主入口。现有进程内 chunk 队列继续承担主任务；图片处理另用持久化任务与 worker。不能把图片重试只放在内存 goroutine 中。
- 现有文本解析、父子分块、摘要和问题向量行为须通过回归测试；OCR 文字可进入文档摘要输入，但不能在普通正文和图片证据中重复建同一份向量。AI 图片描述不进入现有摘要/预测问题生成。
- 图片向量复用 `t_knowledge_chunk_vector` 与当前检索通道。该表的 `chunk_id` 已加宽到 `VARCHAR(64)`；图片证据版本 ID 必须不超过该长度，并与普通文本块 ID 空间不冲突。
- 所有新增状态和证据写入必须可幂等重放。数据库提交、对象存储和模型调用之间没有跨系统事务；通过版本栅栏、临时对象登记与补偿清理避免半成品被检索。
- 不把 API Key、原图 base64、完整敏感提示词或签名 URL 写入日志、chunk 元数据及前端错误信息。

## 阶段 0：确认运行基线与迁移落点

**主要文件：** `internal/adapter/repository/postgres/migrations/`、`internal/bootstrap/knowledge/runtime.go`、`internal/app/core/parser/`、`internal/app/knowledge/service/process/`、`frontend/src/components/chat/CitationChip.tsx`。

- [ ] 记录当前解析器选择、DocReader→Tika 回退、OCR 测试和主分块流程的测试结果；确认 `DOCREADER_LIVE=1` 样本在本地容器可复现。
- [ ] 检查迁移执行顺序、`t_knowledge_document`/chunk/vector 的当前列与索引、`VARCHAR(64)` 证据 ID 约束，以及 Postgres 向量写入是否参与现有事务。
- [ ] 确认当前管理路由的鉴权入口、对象存储键规范、知识库删除与远程文档刷新路径。把发现的路径记录在本计划的实现 PR 描述中，避免另建并行入口。

**完成标准：** 基线测试可重跑，新的数据与接口落点已定位；没有改变运行行为。

## 阶段 1：数据模型、迁移与状态机

**主要文件：** 新迁移 `internal/adapter/repository/postgres/migrations/20260924*_image_evidence.sql`；`internal/app/knowledge/domain/`、`internal/app/knowledge/port/`、`internal/adapter/repository/postgres/knowledge/` 及其测试。

- [ ] 为文档增加活动版本指针和 `partial` 状态；定义 `pending/running/success/partial/failed/deleting` 的合法转换。重试 `partial` 时可进入 `running`，但保留上一发布版本的检索可见性。分块日志、更新条件、删除条件、远程刷新辅助类同步支持 `partial`。
- [ ] 持久化 `document_revision`、`image_occurrence`、不可变 `image_evidence_version`、`image_processing_task`。字段覆盖源文件版本、出现序号、DocReader 引用、原图对象键/MIME/哈希、OCR 与描述状态/文本/错误、模型与提示词版本、任务重试次数/下一次执行时间/租约、活动证据版本及清理标记。
- [ ] 约束与索引：同一文档版本内出现序号唯一；同一版本/图片/处理项只有一个活动任务；任务扫描按到期时间和租约索引；证据版本按文档及出现 ID 可查。图片证据版本 ID 采用短且不可重用的 ID，历史 ID 不被新内容占用。
- [ ] 仓储提供条件更新：只有文档未删除、文档版本匹配、任务租约仍有效时才能写回。聚合状态由任务及有效检索内容推导，不允许客户端直接设为 `success`。
- [ ] 保留 `chunk_count` 表示当前活动 `t_knowledge_chunk` 行数的契约；新图片证据数量单独返回。OCR 从普通文本块转到图片证据后，重处理文档的 `chunk_count` 可能改变，界面不得将其解释为图片数量。迁移须可在已有文档上运行，历史文档不强制重新解析；给出向前部署与回退步骤，回退不能直接抹掉已创建的图片证据数据。

**验证：** 迁移 Up/回退方案在临时库检查；状态流转覆盖 OCR/描述成功、正常无内容、单项失败、全无内容、重试和删除；重复任务写入不能创建重复活动证据。

## 阶段 2：结构化解析与图片资产

**主要文件：** `internal/app/core/parser/parser_result.go`、`docreader_document_parser.go`、`selector.go`、相关测试；`internal/app/knowledge/service/process/document_process_pipeline.go`；图片存储 port 与适配器。

- [ ] 扩展解析结果，返回正文、图片出现清单、占位位置/可用页码、解析方式和 `image_inventory_confirmed`。DocReader 的每个图片帧都要与 Markdown 引用一一校验；缺失帧或重复引用不能被悄悄忽略。支持 inline `image_data`，如镜像返回 `storage_key`，先验证可读路径再明确适配。
- [ ] 将 DocReader 调用限制在结构解析阶段；OCR 与视觉描述从解析器内移到后续图片处理任务。保留单纯 Markdown/TXT 与 HTML/Tika 路径的原有正文行为，提取 DOCX 后继续执行现有文本清理。
- [ ] 为 PDF、Office、EPUB 和独立图片提取图片出现；同一二进制图片重复出现仍生成不同出现 ID。原图保存到私有对象存储，按文档版本管理；处理副本单独生成并约束解码像素、字节数、格式与尺寸。原图不可由未授权公网 URL 访问。
- [ ] 检测 Markdown/HTML 中的外部图片引用，记录未处理原因，不发起网络请求。HTML 即使由 Tika 提正文，也需从原始 HTML 识别图片引用；防止它们被误当作已解析图片。
- [ ] DocReader 失败而 Tika 有正文时，返回 `image_inventory_unknown`；正文可发布为 `partial` 并留下 DocReader 重试任务。Tika 无正文则按失败收口。不能对 Tika 文本结果标记图片清单已确认。
- [ ] 取消当前超过 100 张图片就整份失败的逻辑：完整登记图片后分批处理。单图无法生成处理副本时记录该图错误；后续图片仍继续。图片登记与对象写入失败时执行幂等补偿。

**验证：** 现有 PDF/DOCX/XLSX/HTML/OCR 样本回归；新增同图双出现、图帧缺失、图片超过单批数量、超大图片、Markdown/HTML 外部图片和 Tika 回退场景。断言来源/位置不丢失且没有静默跳过。

## 阶段 3：独立 OCR 与硅基流动视觉模型适配器

**主要文件：** `internal/app/core/parser/ocr_client.go` 的调用侧；新建视觉描述 port/适配器；`internal/framework/config/`、`configs/application.yaml`、相关测试。

- [ ] 定义独立结果：OCR=`success/no_text/error`，描述=`described/no_content/error`；正常无内容不重试。失败原因区分超时、限流、鉴权、模型不可用、图片处理与响应格式；日志只记类别及受控长度的信息。
- [ ] 复用现有 Tesseract OCR 服务，但逐图任务不再受整个 DocReader gRPC 解析超时约束。OCR 的文字按原始结果保存；不得让视觉模型输出覆盖 OCR 字段。
- [ ] 单独调用硅基流动 `/v1/chat/completions`，固定模型 ID `Qwen/Qwen3.8-27B`，用图片处理副本的带 MIME base64 data URL 和文本提示构造多模态内容。先做真实账号探针验证模型、图像输入、响应形状及限制；账号不可用时保留明确错误，不换模型。
- [ ] 视觉提示只要求可见对象、文字、结构、关系及不确定点；邻近正文作为单独上下文字段，不能要求模型把正文推断写成图中事实。解析输出并做空白/套话/长度校验；响应无效归为 `error`，真正无可检索内容归为 `no_content`。
- [ ] 配置独立 API 地址、密钥环境变量、超时、并发、图像尺寸、输出 token 上限与提示词版本。明确允许的唯一外部视觉模型，不接入聊天模型的自动回退。记录有限的用量和延迟指标，用于成本评估。

**验证：** 适配器使用 HTTP stub 覆盖请求体、鉴权、无内容、无效响应、限流和超时；真实联调使用文字扫描、图表、流程图、照片和空白图做人工核对。真实 API Key 只在运行环境注入，不写入测试或文档。

## 阶段 4：持久化图片任务、调度与重试

**主要文件：** `internal/app/knowledge/service/process/`、新的任务仓储和 worker、`internal/bootstrap/knowledge/runtime.go`、相关测试。

- [ ] 主 chunk 任务完成结构解析与正文发布后，为每张图创建 OCR/描述任务；图片描述不阻塞已完成正文和 OCR 的检索。worker 从数据库领取到期任务，设置租约、续租/过期恢复、并发上限与关闭处理。启动时恢复遗留 `running` 任务。
- [ ] 以文档版本、出现 ID 和处理项作幂等键。任务完成前再次校验文档未删除、版本仍有效、租约仍归当前 worker。迟到结果不得写入新版本；临时对象按补偿记录清理。
- [ ] 对可重试错误设置有限次数、退避与抖动；永久错误直接终止并记录原因。`no_text/no_content` 正常终止。手动重试只重新排入失败项，成功项复用；模型/提示词/图片副本参数改变时失效相应缓存。
- [ ] 按“有可检索内容 + 剩余错误/未处理项”推导 `success/partial/failed`；`running` 期间已发布内容可检索。重试失败恢复上一发布状态，不清空旧向量。任务监控显示待处理、失败、重试耗尽与年龄。
- [ ] DocReader 回退的“图片清单未知”另有持久化重试任务；完成后用完整图片清单创建图片任务。若重试仍失败且已有 Tika 正文，保持 `partial`。

**验证：** worker 重启、租约过期、双 worker 竞争、重复提交、限流/超时、人工重试、文档刷新/删除后迟到结果、100+ 图片分批处理。断言每个图片处理项最多一个有效结果与正确的文档收口状态。

## 阶段 5：阶段性发布与检索一致性

**主要文件：** `internal/app/knowledge/service/process/document_process_persistence.go`、`internal/adapter/vectorstore/pgvector/vector_store.go`、`internal/app/rag/core/retrieve/service.go`、相关事务与检索测试。

- [ ] 正文版本发布与图片证据发布分离：新文档版本先发布正文，后续逐图发布 OCR/描述证据；重分块时保持上一已发布版本可查，直到新版本正文已准备好并原子切换。新旧正文块须使用互不冲突、长度不超过 64 字符的版本化 ID，否则当前 `docID-index` ID 无法并存。切换后旧正文向量不再检索；旧文本引用至少不能被同一 ID 的新内容误指。不能继续使用无条件 `DeleteByDocumentID` 清空所有已发布图片向量。
- [ ] 每张图以一条活动图片证据向量参与现有向量检索；`record_type=image`，向量 ID 使用不可变证据版本 ID。OCR/描述新版本的嵌入成功后才切换活动版本，移除旧活动向量；数据库版本指针与向量索引在同一事务或可恢复的发布协议中收口。
- [ ] 图片证据检索文本含 OCR、AI 描述和少量相邻正文，但保留来源标记。回答上下文显式标注 AI 描述；检索结果不可把描述标成文件原文。文本父子分块与问题向量仍按原路径工作；图片不生成摘要或预测问题。OCR 进入文档摘要输入时不得产生重复内容向量。
- [ ] 检索按当前发布文档版本和活动证据版本过滤，历史证据快照不得误入新查询。已有 `question` 命中回链与 `parent_content` 展开不能吞掉或改写 `image` 命中。切换失败时保留旧发布版本并记录待补偿步骤。
- [ ] 普通文本证据的现有 ID 与引用行为保持兼容；图片引用必须使用证据版本 ID，不由普通 chunk 序号推导。文档 `chunk_count` 保留原义，增加图片证据计数及状态字段。

**验证：** 正文/OCR先可检索、图片描述增量发布、图证据独立召回、重复图不互相覆盖、重试过程中旧版本可查、失败不清空向量、历史证据不出现在新检索中；现有父子块和问题向量检索回归。

## 阶段 6：引用、受保护原图与删除

**主要文件：** `internal/app/rag/core/citation/`、`internal/adapter/http/knowledge/`、知识文档删除服务、对象存储适配器、相关测试。

- [ ] 扩展服务端引用登记，使 `record_type=image` 的 `<kb/>` 指向不可变证据版本；模型仍只输出服务端注册过的 `<ref/>`，不能自造图片 URL。历史聊天中已有图片引用保留当时证据版本 ID；非图片老引用保持兼容。
- [ ] 新增图片证据详情与原图读取接口，以及文档图片失败项/任务状态查询和手动重试接口。每次读取都根据当前文档与知识库权限校验，不能仅凭历史证据 ID 或对象键授权。原图通过鉴权 Blob 响应或短时签名地址访问，不提供永久公开 URL。
- [ ] 引用详情返回原图、OCR、AI 描述、相邻正文、处理状态及“AI 生成”标签；没有页码时只展示图片顺序。证据版本已删除或用户无权限时，返回可区分的不可用状态，前端显示“来源已删除或无权访问”。
- [ ] 文档删除时先撤销图片读取和任务写回，再清除活动向量；持久化对象清理清单，幂等删除当前/历史图片副本与临时对象。远程文档刷新建立新文档版本，旧快照只用于既有引用，不进入当前检索。

**验证：** 伪造证据 ID、越权读取、删除后旧引用、权限撤销、过期签名地址、删除与迟到任务竞争、原图清理失败后的补偿重试；现有普通 chunk 引用测试继续通过。

## 阶段 7：前端状态与聊天图片引用

**主要文件：** `frontend/src/services/knowledgeService.ts`、`frontend/src/services/chatService.ts`、`frontend/src/types/index.ts`、`frontend/src/pages/admin/knowledge/KnowledgeDocumentsPage.tsx`、`frontend/src/components/chat/CitationChip.tsx` 及相关测试。

- [ ] 管理页展示 `running/partial/success/failed`、图片总数/完成数/失败数、失败处理项与原因；为失败项提供手动重试。OCR/描述的 `no_text/no_content` 显示为正常无内容，不当作失败。
- [ ] 图片引用沿用现有角标。点击后加载对应证据版本，在弹层中分开展示原图缩略图、放大图、OCR、AI 描述和相邻正文；不要求模型在回答正文里拼接 Markdown 图片。
- [ ] 使用现有带认证头的 API 客户端请求图片 Blob 并创建临时 object URL，或使用后端短时签名地址；关闭弹层时释放 object URL。处理加载失败、无权限、已删除、无描述及过大图片的呈现。
- [ ] 扩展 API 类型而不破坏现有文本 chunk 的展示。检索答案仍可只引用文字；图片预览只在图片证据引用出现时加载。

**验证：** 前端生产构建，组件/服务测试覆盖文本引用兼容、图片证据展示、失败态、无权态和 Blob 释放；浏览器手动检查混合 PDF 的图片引用弹层。

## 阶段 8：真实联调、迁移演练与交付门槛

- [ ] `go test ./cmd/... ./internal/... -run '^$'` 全量编译；运行 parser、process、知识文档服务、Postgres adapter、向量检索、引用与 HTTP handler 的相关测试；前端运行 `npm run build`。
- [ ] 启动 DocReader、OCR、PostgreSQL、对象存储与应用。对混合 PDF、扫描页、图表、照片、空白图、100+ 图片及外部图片引用，走真实上传→分块→持久化→检索→聊天引用；记录文档状态、证据版本、向量和原图展示证据。
- [ ] 使用硅基流动账号真实调用 `Qwen/Qwen3.8-27B`；验证图片请求、模型输出、超时/限流、用量与人工描述质量。若账号不可用，标记该验收门槛未通过，不能宣称视觉描述链路已完成。
- [ ] 演练服务重启、模型故障与恢复、Tika 回退、手动重试、刷新并发、删除与对象清理。确认 `partial` 可见、旧版本保持可检索、旧引用不漂移、迟到结果不污染新版本。
- [ ] 在临时数据库运行迁移与回退演练，确认旧文档/旧引用/无图文档仍可用。上线采用先迁移、再部署支持新结构的后端、再启用图片 worker 与前端的顺序；启用开关关闭时，旧文档行为保持可用。回滚须先停 worker，再切回旧读取路径；已发布图片数据不做破坏性删除，待恢复后继续处理。
- [ ] 更新 `docs/docreader_integration.md` 和部署说明，记录模型配置、状态含义、重试诊断、对象清理与联调命令。交付报告区分真实端到端通过的路径与仅有 stub 的路径。

**交付标准：** 规格中的 9 组验收场景均有测试或真实联调证据；主入口、默认配置、数据库迁移、worker 启停、对象存储、向量检索、引用详情及前端展示完整接通。任何未通过的真实模型或持久化门槛必须明确列为未完成，不以模块测试通过代替整体交付。
