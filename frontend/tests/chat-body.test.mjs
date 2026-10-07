import assert from "node:assert/strict";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { build } from "../node_modules/esbuild/lib/main.js";

// Exercise the real SSE reader and Zustand handlers; stub only external APIs
// and browser storage. No live server or model is needed.
const root = fileURLToPath(new URL("../", import.meta.url));
const stubs = {
  sonner: "export const toast = { error() {}, success() {} };",
  "@/utils/storage": "export const storage = { getChatKnowledgeBaseIds: () => [], setChatKnowledgeBaseIds() {}, getToken: () => null };",
  "@/services/sessionService": `
    export const listMessages = async () => globalThis.__chatBodyMessages;
    export const listSessions = async () => [];
    export const deleteSession = async () => {};
    export const renameSession = async () => {};
  `,
  "@/services/chatService": `
    export const getPendingApproval = async () => null;
    export const stopTask = async () => {};
    export const submitFeedback = async () => {};
  `
};
const bundled = await build({
  absWorkingDir: root,
  stdin: { contents: 'export { useChatStore } from "./src/stores/chatStore.ts"; export { executionPresentation } from "./src/stores/chatStateModel.ts";', resolveDir: root },
  bundle: true,
  write: false,
  format: "esm",
  platform: "node",
  alias: { "@": `${root}src` },
  define: { "import.meta.env.VITE_API_BASE_URL": '""' },
  plugins: [{
    name: "external-chat-apis",
    setup(builder) {
      builder.onResolve({ filter: /^(sonner|@\/services\/(sessionService|chatService)|@\/utils\/storage)$/ }, ({ path }) => ({ path, namespace: "stub" }));
      builder.onLoad({ filter: /.*/, namespace: "stub" }, ({ path }) => ({ contents: stubs[path], loader: "js" }));
    }
  }]
});
const { useChatStore, executionPresentation } = await import(`data:text/javascript;base64,${Buffer.from(bundled.outputFiles[0].text).toString("base64")}`);

function sse(event, payload) {
  return `event: ${event}\ndata: ${JSON.stringify(payload)}\n\n`;
}

function reset() {
  useChatStore.setState({
    sessions: [], currentSessionId: null, lastResolvedSessionId: null,
    messages: [], isStreaming: false, isCreatingNew: true,
    streamingMessageId: null, streamTaskId: null, cancelRequested: false,
    deepThinkingEnabled: false, thinkingStartAt: null
  });
}

test("finish replaces replayed or missing deltas and reload displays full original body", async () => {
  reset();
  const body = "先查资料。\n\n完整结论。";
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response([
    sse("meta", { conversationId: "conversation", taskId: "task" }),
    sse("message", { type: "think", delta: "思考过程" }),
    sse("message", { type: "response", delta: "先查资料。" }),
    sse("tool", { callId: "call", name: "知识库检索", status: "completed", summary: "tool result" }),
    // Deliberately duplicate the prefix and omit the conclusion.
    sse("message", { type: "response", delta: "先查资料。" }),
    sse("finish", { messageId: "saved", content: body, sources: [{ type: "web", url: "https://example.com" }] }),
    sse("done", {})
  ].join(""), { headers: { "Content-Type": "text/event-stream" } });
  try {
    await useChatStore.getState().sendMessage("question");
    const answer = useChatStore.getState().messages.at(-1);
    assert.equal(answer.id, "saved");
    assert.equal(answer.content, body);
    assert.equal(answer.thinking, "思考过程");
    assert.equal(answer.toolCalls.length, 1);
    assert.equal(answer.status, "done");
    assert.equal(useChatStore.getState().isStreaming, false);

    globalThis.__chatBodyMessages = [{ id: "saved", role: "assistant", content: "短摘要", rawContent: body, vote: null }];
    useChatStore.setState({ currentSessionId: null, messages: [] });
    await useChatStore.getState().selectSession("conversation");
    assert.equal(useChatStore.getState().messages[0].content, body);

    // Older messages with no original body still display their content.
    globalThis.__chatBodyMessages = [{ id: "old", role: "assistant", content: "旧正文", rawContent: "  ", vote: null }];
    useChatStore.setState({ currentSessionId: null, messages: [] });
    await useChatStore.getState().selectSession("conversation");
    assert.equal(useChatStore.getState().messages[0].content, "旧正文");
  } finally {
    globalThis.fetch = originalFetch;
    delete globalThis.__chatBodyMessages;
  }
});

test("older finish events retain streamed content", async () => {
  reset();
  const originalFetch = globalThis.fetch;
  globalThis.fetch = async () => new Response([
    sse("message", { type: "response", delta: "旧事件的完整正文" }),
    sse("finish", { messageId: "old" }),
    sse("done", {})
  ].join(""));
  try {
    await useChatStore.getState().sendMessage("question");
    assert.equal(useChatStore.getState().messages.at(-1).content, "旧事件的完整正文");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("reconnect applies the persisted body after an incomplete initial stream", async () => {
  reset();
  const originalFetch = globalThis.fetch;
  const urls = [];
  globalThis.fetch = async (url) => {
    urls.push(url);
    if (urls.length === 1) return new Response(
      sse("meta", { conversationId: "conversation", taskId: "task" }) +
      sse("message", { type: "response", delta: "部分正文" })
    );
    return new Response(
      sse("finish", { messageId: "saved", content: "部分正文及完整结论" }) + sse("done", {})
    );
  };
  try {
    await useChatStore.getState().sendMessage("question");
    assert.equal(urls.length, 2);
    assert.match(urls[1], /\/chat\/continue\?taskId=task&offset=2/);
    assert.equal(useChatStore.getState().messages.at(-1).content, "部分正文及完整结论");
    assert.equal(useChatStore.getState().messages.at(-1).status, "done");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("deep thinking switch is sent per chat request and reasoning stays separate", async () => {
  const originalFetch = globalThis.fetch;
  const requests = [];
  globalThis.fetch = async (_url, options) => {
    const request = JSON.parse(options.body);
    requests.push(request);
    return new Response(
      (request.deepThinking ? sse("message", { type: "think", delta: "思考内容" }) : "") +
      sse("message", { type: "response", delta: "答案" }) +
      sse("finish", { messageId: "saved", content: "答案" }) + sse("done", {})
    );
  };
  try {
    for (const enabled of [true, false]) {
      reset();
      useChatStore.getState().setDeepThinkingEnabled(enabled);
      await useChatStore.getState().sendMessage("question");
      const answer = useChatStore.getState().messages.at(-1);
      assert.equal(answer.content, "答案");
      assert.equal(Boolean(answer.thinking), enabled);
      assert.equal(answer.isThinking, false);
    }
    assert.equal(requests[0].deepThinking, true);
    assert.equal(requests[1].deepThinking, undefined);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("execution follows multiple thinking rounds and concurrent tool result arrival", async () => {
  reset();
  const originalFetch = globalThis.fetch;
  const observed = [];
  const unsubscribe = useChatStore.subscribe((state) => {
    const message = state.messages.at(-1);
    if (message?.executionSegments?.length) observed.push(message);
  });
  globalThis.fetch = async () => new Response([
    sse("message", { type: "think", delta: "分析" }),
    sse("message", { type: "think", delta: "任务" }),
    sse("message", { type: "response", delta: "先检查文件。" }),
    sse("tool", { callId: "a", name: "Read file", status: "pending", arguments: { path: "app.go" } }),
    sse("tool_start", { callId: "a", name: "Read file", status: "running" }),
    sse("tool_start", { callId: "b", name: "Search", status: "running" }),
    sse("tool_result", { callId: "b", name: "Search", status: "completed", summary: "search result" }),
    sse("tool_result", { callId: "a", name: "Read file", status: "completed", summary: "file result" }),
    // Repeated updates do not create another call/result or lose arguments.
    sse("tool_result", { callId: "a", name: "Read file", status: "completed", summary: "file result" }),
    sse("message", { type: "think", delta: "根据结果检查边界。" }),
    sse("tool", { callId: "c", name: "Verify", status: "failed", summary: "test failed", data: { log: "raw output" } }),
    sse("message", { type: "think", delta: "最后总结。" }),
    sse("message", { type: "response", delta: "最终结论。" }),
    sse("finish", { messageId: "saved", content: "先检查文件。最终结论。" }),
    sse("done", {})
  ].join(""));
  try {
    await useChatStore.getState().sendMessage("question");
    const message = useChatStore.getState().messages.at(-1);
    const segments = message.executionSegments;
    assert.deepEqual(segments.map((part) => part.kind), ["thinking", "text", "tool_call", "tool_call", "tool_result", "tool_result", "thinking", "tool_call", "tool_result", "thinking", "text"]);
    assert.deepEqual(segments.filter((part) => part.kind === "thinking").map((part) => part.content), ["分析任务", "根据结果检查边界。", "最后总结。"]);
    assert.ok(segments.filter((part) => part.kind === "thinking").every((part) => part.status === "done"));
    assert.deepEqual(segments.filter((part) => part.kind === "tool_result").map((part) => part.tool.callId), ["b", "a", "c"]);
    assert.deepEqual(segments.find((part) => part.kind === "tool_call" && part.tool.callId === "a").tool.arguments, { path: "app.go" });
    assert.ok(observed.some((item) => item.executionSegments[0].status === "working"));
    const presentation = executionPresentation(message);
    assert.equal(presentation.answer, "最终结论。");
    assert.equal(presentation.segments.filter((part) => part.kind === "text").map((part) => part.content).join("") + presentation.answer, message.content);
    assert.equal(message.toolCalls.length, 3);
  } finally { unsubscribe(); globalThis.fetch = originalFetch; }
});

test("finish correction is authoritative without repeating streamed narration", () => {
  const result = executionPresentation({ id: "saved", role: "assistant", content: "正确正文", status: "done", executionSegments: [
    { id: "text-0", kind: "text", content: "重复前缀" },
    { id: "tool", kind: "tool_call", tool: { name: "Search", status: "completed" } },
    { id: "text-1", kind: "text", content: "错误正文" }
  ] });
  assert.equal(result.answer, "正确正文");
  assert.equal(result.segments.some((part) => part.kind === "text"), false);
  const legacy = executionPresentation({ id: "old", role: "assistant", content: "旧答案", thinking: "旧思考", status: "done" });
  assert.equal(legacy.segments[0].status, "done");
  assert.equal(legacy.answer, "旧答案");
});

test("error, cancellation and bare done end the active thinking segment", async () => {
  const originalFetch = globalThis.fetch;
  try {
    for (const [event, expected] of [["error", "stopped"], ["cancel", "stopped"], ["done", "done"]]) {
      reset();
      globalThis.fetch = async () => new Response(sse("message", { type: "think", delta: "正在检查" }) + sse(event, event === "error" ? { error: "interrupted" } : {}) + (event === "done" ? "" : sse("done", {})));
      await useChatStore.getState().sendMessage("question");
      const message = useChatStore.getState().messages.at(-1);
      assert.equal(message.executionSegments[0].status, expected);
      assert.equal(message.isThinking, false);
      assert.equal(useChatStore.getState().isStreaming, false);
    }
  } finally { globalThis.fetch = originalFetch; }
});

test("legacy tool start/result without IDs updates one call in the execution view", () => {
  reset();
  useChatStore.setState({ messages: [{id:"legacy",role:"assistant",content:"",status:"streaming"}], streamingMessageId:"legacy" });
  useChatStore.getState().appendThinkingContent("检查旧事件。");
  useChatStore.getState().appendToolCall({name:"Search",status:"running",arguments:{query:"q"}});
  useChatStore.getState().appendToolCall({name:"Search",status:"completed",summary:"found"});
  const segments=useChatStore.getState().messages[0].executionSegments;
  assert.deepEqual(segments.map(part=>part.kind), ["thinking","tool_call","tool_result"]);
  assert.equal(segments[0].status,"done");
  assert.deepEqual(segments[1].tool.arguments,{query:"q"});
});
