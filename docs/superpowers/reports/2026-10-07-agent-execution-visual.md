# Agent 对话执行过程视觉优化

日期：2026-10-07。范围：普通 Chat 的 Thinking / Tool Call / Tool Result / 正文展示及滚动体验。按用户粘贴要求直接改造现有前端，未修改后端、数据库或模型调用，未新增依赖，未提交或部署。

## 实现与设计决策

- `Message.executionSegments` 是浏览器内的轻量执行投影，按实际 SSE 到达顺序保留 thinking、tool_call、tool_result、text；原 content / thinking / toolCalls 与既有审批、草稿、反馈、来源处理保持兼容。连续 thinking delta 合并，工具或正文事件结束当前思考；下一次 thinking 创建独立段。工具用 callId 原位更新，结果在到达时插入，支持并发工具结果逆序到达和重复状态更新。
- 用细时间线替换按类型聚合的大型蓝色、黄色卡片。Thinking header 使用 Sparkles 与中性背景，执行时默认展开、完成后自动折叠，用户仍可自行展开/收起；中断思考显示 stopped。文本不进行逐 token 动画，已移除整个末尾 assistant 消息的淡入，工具新行只有短淡入并支持 reduced-motion。
- 工具名称突出，参数弱化；结果缩进且只预览两行，完整参数 / JSON / 日志可以展开。thinking 内容最多 176px，结果最多 240px，超过后内部滚动。8 次 thinking / 15 次工具的已完成执行默认收起较早记录，可展开全部；失败步骤始终保留可见。
- 中间正文保留在发生位置，最后正文在细分隔线后展示，复制仍使用完整 content。完成事件校准后的 content 具有最高优先级；无法与流式前缀匹配时，不展示重复中间正文，直接显示校准后的完整正文。引用编号继续依据完整 content。
- 复用项目 CSS 变量与既有 Work 暗色中性色，明暗与 390px 窄屏均验收。样式作用于 Chat 阅读区域，不引入新的全局配色系统。
- 页面跟随流式增长时使用 rAF；滚轮向上、触摸、键盘向上阅读及指针交互会暂停跟随，回到底部恢复。完成收尾也尊重用户阅读位置，初次加载的延迟滚动同样受跟随状态约束。

## 验证

`frontend/tests/chat-body.test.mjs` 的 8 项测试通过，使用实际 SSE reader 与 Zustand store，覆盖多段 thinking、并发结果到达顺序、工具更新不重复、参数保留、完整正文校准与重连、旧消息兼容、开关传参、错误 / 停止 / bare done 收敛和无 ID 旧工具事件。证据：`tmp/chat-execution-tests.log`。

Vite production build 通过（`tmp/chat-execution-build.log`）。TypeScript 全量仍失败，42 项诊断与先前 `tmp/chat-body-typecheck.log` 基线数量相同，按文件、代码及主诊断比较无新增；既有审批映射 TS2322 的类型展开文本随新增字段变化，其根因仍为 status 宽化。比较脚本：`tmp/chat-execution-typecheck-compare.cjs`。

浏览器使用 `frontend/tests/fixtures/agent-execution-preview.html` / `.tsx`，导入正式 MessageList、MessageItem、SSE reader 与 Zustand，事件由本地可控 fixture 提供，不请求后端或外部模型。逐阶段确认执行中展开、3 段 thinking 在工具之间独立排列、完成自动收起、点击查看完整工具结果。工具长输出测得 clientHeight=240 / scrollHeight=919；长 thinking 为 176 / 1317。长执行全部展开有 38 段，默认收起 30 段且保留失败步骤。390px viewport 没有横向溢出。

流式上滚验收：从底部向上阅读后 scrollTop=57.33；新增工具、结果、thinking 与最终完成后仍为 57.33。未主动上滚时自动跟随距底部约 0px。观察流式追加未发现整块消息反复动画或强制跳回底部；阶段完成折叠和显式展开导致的高度变化属于预期行为，不承诺布局完全不变。

截图：`tmp/chat-execution-light.jpg`、`tmp/chat-execution-dark.jpg`、`tmp/chat-execution-mobile.jpg`、`tmp/chat-execution-long-dark.jpg`、`tmp/chat-execution-mobile-dark.jpg`。均为真实组件的合成事件视觉验收，不作为新增真实模型调用证据。此前真实模型的开关验证仍见深度思考报告。

## 文件与交付边界

产品改动：

- `frontend/src/types/index.ts`
- `frontend/src/stores/chatStateModel.ts`
- `frontend/src/stores/chatStore.ts`
- `frontend/src/components/chat/ExecutionTimeline.tsx`
- `frontend/src/components/chat/ThinkingIndicator.tsx`
- `frontend/src/components/chat/MessageItem.tsx`
- `frontend/src/components/chat/MessageList.tsx`
- `frontend/src/styles/globals.css`

验证改动：`frontend/tests/chat-body.test.mjs`、`frontend/tests/fixtures/agent-execution-preview.html`、`frontend/tests/fixtures/agent-execution-preview.tsx`。文档：本报告、项目进度、文档索引。

本次视觉承诺核对为 aligned：正式 Chat 组件已接入，无独立产品演示替代正式入口。executionSegments 暂未持久化；重载后的历史只有后端已有正文与可选 thinking 字段，无法重建旧执行顺序，不伪造历史阶段。Work 的独立聊天渲染、真实工具参数协议补齐、历史 journal 回填仍在本次范围之外。验收后停止本次启动的 Vite 服务。
