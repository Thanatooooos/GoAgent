import * as React from "react";
import { AlertCircle, Brain, CheckCircle2, ChevronDown, Database, History, Wrench, XCircle } from "lucide-react";

import { FeedbackButtons } from "@/components/chat/FeedbackButtons";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { ThinkingIndicator } from "@/components/chat/ThinkingIndicator";
import { cn } from "@/lib/utils";
import type { Message } from "@/types";

interface MessageItemProps {
  message: Message;
  isLast?: boolean;
}

export const MessageItem = React.memo(function MessageItem({ message, isLast }: MessageItemProps) {
  const isUser = message.role === "user";
  const showFeedback =
    message.role === "assistant" &&
    message.status !== "streaming" &&
    message.id &&
    !message.id.startsWith("assistant-");
  const isThinking = Boolean(message.isThinking);
  const [thinkingExpanded, setThinkingExpanded] = React.useState(false);
  const [toolsExpanded, setToolsExpanded] = React.useState(false);
  const hasThinking = Boolean(message.thinking && message.thinking.trim().length > 0);
  const hasContent = message.content.trim().length > 0;
  const isWaiting = message.status === "streaming" && !isThinking && !hasContent;
  const toolCalls = message.toolCalls ?? [];
  const memoryEvents = message.memoryEvents ?? [];
  const sessionRecallEvents = message.sessionRecallEvents ?? [];
  const agentThinks = (message.agentThinks ?? []).filter((item) => item.trim().length > 0);
  const hasAgentThinks = agentThinks.length > 0;
  const hasToolCalls = toolCalls.length > 0;
  const hasMemoryEvents = memoryEvents.length > 0;
  const hasSessionRecallEvents = sessionRecallEvents.length > 0;
  const hasFailedTools = toolCalls.some((tc) => tc.status === "failed");
  const fallbackReason = message.fallbackReason?.trim();

  if (isUser) {
    return (
      <div className="flex">
        <div className="user-message">
          <p className="whitespace-pre-wrap break-words">{message.content}</p>
        </div>
      </div>
    );
  }

  const thinkingDuration = message.thinkingDuration ? `${message.thinkingDuration}秒` : "";

  return (
    <div className="group flex">
      <div className="min-w-0 flex-1 space-y-4">
        {isThinking ? (
          <ThinkingIndicator content={message.thinking} duration={message.thinkingDuration} />
        ) : null}

        {!isThinking && hasThinking ? (
          <div className="overflow-hidden rounded-lg border border-[#BFDBFE] bg-[#DBEAFE]">
            <button
              type="button"
              onClick={() => setThinkingExpanded((prev) => !prev)}
              className="flex w-full items-center gap-2 px-4 py-3 text-left transition-colors hover:bg-[#BFDBFE]/30"
            >
              <div className="flex flex-1 items-center gap-2">
                <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-[#BFDBFE]">
                  <Brain className="h-4 w-4 text-[#2563EB]" />
                </div>
                <span className="text-sm font-medium text-[#2563EB]">深度思考</span>
                {thinkingDuration ? (
                  <span className="rounded-full bg-[#BFDBFE] px-2 py-0.5 text-xs text-[#2563EB]">
                    {thinkingDuration}
                  </span>
                ) : null}
              </div>
              <ChevronDown
                className={cn(
                  "h-4 w-4 text-[#3B82F6] transition-transform",
                  thinkingExpanded && "rotate-180"
                )}
              />
            </button>
            {thinkingExpanded ? (
              <div className="border-t border-[#BFDBFE] px-4 pb-4">
                <div className="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-[#1E40AF]">
                  {message.thinking}
                </div>
              </div>
            ) : null}
          </div>
        ) : null}

        {hasAgentThinks ? (
          <div className="overflow-hidden rounded-lg border border-sky-200 bg-sky-50">
            <div className="flex items-center gap-2 px-4 py-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-sky-100">
                <Brain className="h-4 w-4 text-sky-700" />
              </div>
              <span className="text-sm font-medium text-sky-800">Agent 推理</span>
              <span className="rounded-full bg-sky-100 px-2 py-0.5 text-xs text-sky-700">
                {agentThinks.length}
              </span>
            </div>
            <div className="border-t border-sky-200 px-4 pb-4">
              {agentThinks.map((item, idx) => (
                <p key={`${idx}-${item}`} className="mt-3 text-sm leading-6 text-sky-900">
                  {item}
                </p>
              ))}
            </div>
          </div>
        ) : null}

        {hasMemoryEvents ? (
          <div className="overflow-hidden rounded-lg border border-emerald-200 bg-emerald-50">
            <div className="flex items-center gap-2 px-4 py-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-emerald-100">
                <Database className="h-4 w-4 text-emerald-700" />
              </div>
              <span className="text-sm font-medium text-emerald-800">Long message stored</span>
              <span className="rounded-full bg-emerald-100 px-2 py-0.5 text-xs text-emerald-700">
                {memoryEvents.length}
              </span>
            </div>
            <div className="border-t border-emerald-200 px-4 pb-4">
              {memoryEvents.map((event, idx) => (
                <div
                  key={`${event.messageId}-${idx}`}
                  className="mt-3 rounded-lg border border-emerald-100 bg-white px-3 py-2.5"
                >
                  <p className="text-sm font-medium text-emerald-900">
                    Message {event.messageId} was summarized for later recall.
                  </p>
                  {event.contentSummary ? (
                    <p className="mt-1 text-xs leading-5 text-emerald-800">{event.contentSummary}</p>
                  ) : null}
                  {typeof event.rawContentLength === "number" && event.rawContentLength > 0 ? (
                    <p className="mt-1 text-xs text-emerald-700">
                      Raw length: {event.rawContentLength} chars
                    </p>
                  ) : null}
                </div>
              ))}
            </div>
          </div>
        ) : null}

        {hasSessionRecallEvents ? (
          <div className="overflow-hidden rounded-lg border border-violet-200 bg-violet-50">
            <div className="flex items-center gap-2 px-4 py-3">
              <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-violet-100">
                <History className="h-4 w-4 text-violet-700" />
              </div>
              <span className="text-sm font-medium text-violet-800">Session recall</span>
              <span className="rounded-full bg-violet-100 px-2 py-0.5 text-xs text-violet-700">
                {sessionRecallEvents.reduce((count, event) => count + (event.hitCount || 0), 0)} hits
              </span>
            </div>
            <div className="border-t border-violet-200 px-4 pb-4">
              {sessionRecallEvents.map((event, idx) => (
                <div
                  key={`${event.query || "recall"}-${idx}`}
                  className="mt-3 rounded-lg border border-violet-100 bg-white px-3 py-2.5"
                >
                  <p className="text-sm font-medium text-violet-900">
                    Recalled {event.hitCount} earlier message chunk(s)
                    {event.query ? ` for "${event.query}"` : ""}.
                  </p>
                  {typeof event.topScore === "number" && event.topScore > 0 ? (
                    <p className="mt-1 text-xs text-violet-700">Top score: {event.topScore.toFixed(2)}</p>
                  ) : null}
                  {event.hits?.length ? (
                    <div className="mt-2 space-y-2">
                      {event.hits.map((hit, hitIndex) => (
                        <div key={`${hit.messageId}-${hit.chunkIndex}-${hitIndex}`} className="rounded-md bg-violet-50 px-2.5 py-2">
                          <p className="text-xs font-medium text-violet-900">
                            {hit.messageId} / chunk {hit.chunkIndex}
                          </p>
                          {hit.summary ? (
                            <p className="mt-1 text-xs text-violet-800">{hit.summary}</p>
                          ) : null}
                          {hit.excerpt ? (
                            <p className="mt-1 line-clamp-3 whitespace-pre-wrap text-xs text-violet-700">
                              {hit.excerpt}
                            </p>
                          ) : null}
                        </div>
                      ))}
                    </div>
                  ) : null}
                </div>
              ))}
            </div>
          </div>
        ) : null}

        {hasToolCalls ? (
          <div className="overflow-hidden rounded-lg border border-amber-200 bg-amber-50">
            <button
              type="button"
              onClick={() => setToolsExpanded((prev) => !prev)}
              className="flex w-full items-center gap-2 px-4 py-3 text-left transition-colors hover:bg-amber-100/50"
            >
              <div className="flex flex-1 items-center gap-2">
                <div className="flex h-7 w-7 items-center justify-center rounded-lg bg-amber-100">
                  <Wrench className="h-4 w-4 text-amber-700" />
                </div>
                <span className="text-sm font-medium text-amber-800">
                  工具调用 ({toolCalls.length})
                </span>
                {hasFailedTools ? (
                  <span className="rounded-full bg-red-100 px-2 py-0.5 text-xs text-red-600">
                    部分失败
                  </span>
                ) : null}
              </div>
              <ChevronDown
                className={cn(
                  "h-4 w-4 text-amber-600 transition-transform",
                  toolsExpanded && "rotate-180"
                )}
              />
            </button>
            {toolsExpanded ? (
              <div className="border-t border-amber-200 px-4 pb-4">
                {toolCalls.map((tc, idx) => (
                  <div
                    key={idx}
                    className="mt-3 flex items-start gap-3 rounded-lg border border-amber-100 bg-white px-3 py-2.5"
                  >
                    {tc.status === "success" ? (
                      <CheckCircle2 className="mt-0.5 h-4 w-4 flex-shrink-0 text-green-500" />
                    ) : tc.status === "failed" ? (
                      <XCircle className="mt-0.5 h-4 w-4 flex-shrink-0 text-red-500" />
                    ) : (
                      <AlertCircle className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500" />
                    )}
                    <div className="min-w-0 flex-1">
                      <div className="flex items-center gap-2">
                        <span className="text-sm font-medium text-gray-800">{tc.name}</span>
                        {typeof tc.round === "number" ? (
                          <span className="rounded-full bg-slate-100 px-1.5 py-0.5 text-xs text-slate-600">
                            第 {tc.round} 轮
                          </span>
                        ) : null}
                        <span
                          className={cn(
                            "rounded-full px-1.5 py-0.5 text-xs font-medium",
                            tc.status === "success"
                              ? "bg-green-50 text-green-600"
                              : tc.status === "failed"
                                ? "bg-red-50 text-red-600"
                                : "bg-amber-50 text-amber-600"
                          )}
                        >
                          {tc.status}
                        </span>
                        {typeof tc.durationMs === "number" && tc.durationMs > 0 ? (
                          <span className="text-xs text-gray-400">{tc.durationMs}ms</span>
                        ) : null}
                      </div>
                      {tc.arguments && Object.keys(tc.arguments).length > 0 ? (
                        <p className="mt-1 break-words text-xs text-gray-400">
                          参数：{JSON.stringify(tc.arguments)}
                        </p>
                      ) : null}
                      {tc.summary ? (
                        <p className="mt-0.5 line-clamp-3 break-words text-xs text-gray-500">
                          {tc.summary}
                        </p>
                      ) : null}
                    </div>
                  </div>
                ))}
              </div>
            ) : null}
          </div>
        ) : null}

        <div className="space-y-2">
          {fallbackReason ? (
            <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-900">
              <span className="font-medium">已回退到通用模型：</span>
              当前知识库检索置信度较低，请注意核验回答内容。
            </div>
          ) : null}

          {isWaiting ? (
            <div className="ai-wait" aria-label="思考中">
              <span className="ai-wait-dots" aria-hidden="true">
                <span className="ai-wait-dot" />
                <span className="ai-wait-dot" />
                <span className="ai-wait-dot" />
              </span>
            </div>
          ) : null}

          {hasContent ? <MarkdownRenderer content={message.content} /> : null}

          {message.status === "error" ? (
            <p className="text-xs text-rose-500">生成已中断。</p>
          ) : null}

          {showFeedback ? (
            <FeedbackButtons
              messageId={message.id}
              feedback={message.feedback ?? null}
              content={message.content}
              alwaysVisible={Boolean(isLast)}
            />
          ) : null}
        </div>
      </div>
    </div>
  );
});
