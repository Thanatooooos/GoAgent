// Development-only visual fixture. Uses the shipped SSE reader/store/components
// with synthetic local events; it never contacts a model or a backend.
import * as React from "react";
import { createRoot } from "react-dom/client";
import { Toaster } from "sonner";
import { BrowserRouter } from "react-router-dom";
import { MessageList } from "@/components/chat/MessageList";
import { useChatStore } from "@/stores/chatStore";
import { useThemeStore } from "@/stores/themeStore";
import "@/styles/globals.css";

const encode = (event: string, data: unknown) => new TextEncoder().encode(`event: ${event}\ndata: ${JSON.stringify(data)}\n\n`);
let stream: ReadableStreamDefaultController<Uint8Array> | undefined;
let phase = 0;
const phases: Array<Array<[string, unknown]>> = [
  [["message", { type: "think", delta: "先梳理当前消息结构与事件顺序。\n需要保留原有正文、来源与审批流程，优先修改渲染层。" }]],
  [["message", { type: "response", delta: "我先读取现有实现，确认流式事件的边界。\n\n" }], ["tool_start", { callId: "read", name: "Read file", arguments: { path: "frontend/src/components/chat/MessageItem.tsx" }, status: "running" }]],
  [["tool_result", { callId: "read", name: "Read file", status: "completed", summary: "已读取 MessageItem.tsx，当前 thinking 与工具调用按类型聚合。", durationMs: 230, data: { path: "MessageItem.tsx", lines: 420 } }], ["message", { type: "think", delta: "根据文件内容，现有 SSE 可以保留到达顺序。\n将执行记录拆成轻量 segment，保持后端接口与消息正文不变。" }]],
  [["tool", { callId: "check", name: "Run checks", status: "running", arguments: { command: "node --test frontend/tests/chat-body.test.mjs" } }]],
  [["tool", { callId: "check", name: "Run checks", status: "completed", summary: "检查通过：多段思考、工具结果顺序、正文校准与旧消息兼容。", durationMs: 2100, data: { output: Array.from({length:120}, (_,i)=>`[${i+1}] assertion passed · execution timeline`).join("\n") } }], ["message", { type: "think", delta: "最后核对浅色、深色与窄屏。\n完成阶段自动折叠，完整输出仍然可展开查看。" }]],
  [["message", { type: "response", delta: "已完成对话区域优化。\n\n- 思考、工具和结果按实际发生顺序展示。\n- 细节按需展开，最终回答保持清晰。\n- 流式更新与正文保存逻辑保持兼容。" }], ["finish", { messageId: "preview-answer", content: "我先读取现有实现，确认流式事件的边界。\n\n已完成对话区域优化。\n\n- 思考、工具和结果按实际发生顺序展示。\n- 细节按需展开，最终回答保持清晰。\n- 流式更新与正文保存逻辑保持兼容。" }], ["done", {}]]
];
window.fetch = async () => new Response(new ReadableStream({ start(controller) { stream = controller; } }), { headers: { "Content-Type": "text/event-stream" } });
const history = Array.from({length:12}, (_,i)=>({id:`history-${i}`, role:i%2 ? "assistant" as const : "user" as const, content:i%2 ? "这是一条历史消息，用于检查向上阅读时页面是否保持位置。\n\n执行细节应按需展开，正文仍然最易阅读。" : `历史任务 ${i+1}`, status:"done" as const}));
useChatStore.setState({ messages: history, currentSessionId:null, isCreatingNew:true });

function Preview() {
  const {messages, isStreaming} = useChatStore();
  const [stage, setStage] = React.useState(-1);
  function start() {
    if (isStreaming) return;
    useChatStore.setState({messages:history, currentSessionId:null, isCreatingNew:true});
    phase=0;
    void useChatStore.getState().sendMessage("请优化 Agent 对话区域，保留现有业务功能。");
    setStage(0);
  }
  function next() {
    if (!stream || phase >= phases.length) return;
    for (const [event,data] of phases[phase++]) stream.enqueue(encode(event,data));
    if (phase === phases.length) {stream.close(); stream=undefined;}
    setStage(phase);
  }
  function stress() {
    if (isStreaming) return;
    const id="stress";
    useChatStore.setState({messages:[{id,role:"assistant",content:"",status:"streaming"}], streamingMessageId:id});
    for(let i=0;i<15;i++) {
      if(i%2===0) useChatStore.getState().appendThinkingContent(`第 ${i/2+1} 段思考。\n`+"检查边界与执行条件。\n".repeat(60));
      useChatStore.getState().appendToolCall({callId:`stress-${i}`,name:i===8?"Verify (failed)":`Read file ${i+1}`,status:"running",arguments:{path:`src/module-${i}.go`}});
      useChatStore.getState().appendToolCall({callId:`stress-${i}`,name:i===8?"Verify (failed)":`Read file ${i+1}`,status:i===8?"failed":"completed",summary:i===8?"验证失败：展示失败状态，不伪装成功。":"已完成读取。",data:{output:"日志行\n".repeat(200)}});
    }
    useChatStore.getState().appendStreamContent("长执行已完成。详细记录可展开，正文仍然是主要阅读内容。");
    useChatStore.setState(state=>({messages:state.messages.map(message=>({...message,status:"done" as const})),streamingMessageId:null}));
    setStage(-1);
  }
  return <BrowserRouter><div className="chat-page-content flex h-screen flex-col">
    <header className="flex flex-wrap items-center gap-3 border-b px-5 py-3 text-xs dark:text-gray-200 dark:border-gray-700">
      <strong>Agent execution · 视觉验收</strong>
      <button onClick={start} disabled={isStreaming}>开始流式演示</button>
      <button onClick={next} disabled={!isStreaming}>下一步 ({stage}/6)</button>
      <button onClick={stress} disabled={isStreaming}>长执行场景</button>
      <button onClick={()=>useThemeStore.getState().toggleTheme()}>切换明暗</button>
    </header>
    <main className="min-h-0 flex-1"><MessageList messages={messages} isLoading={false} isStreaming={isStreaming} sessionKey="visual-preview" /></main>
    <footer className="border-t px-5 py-3 text-xs dark:text-gray-300 dark:border-gray-700">合成事件 · 正式 SSE reader / Zustand / MessageList · 不调用外部模型</footer>
    <Toaster />
  </div></BrowserRouter>;
}
createRoot(document.getElementById("root")!).render(<Preview />);
