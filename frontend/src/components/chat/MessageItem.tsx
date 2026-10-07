import * as React from "react";
import { Database, FileText, Globe, History } from "lucide-react";

import { ApprovalPendingCard } from "@/components/chat/ApprovalPendingCard";
import { CitationNumberProvider } from "@/components/chat/citationContext";
import { FeedbackButtons } from "@/components/chat/FeedbackButtons";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { draftsFromToolCalls, ScheduledTaskDraftCard } from "@/components/chat/ScheduledTaskDraftCard";
import { ExecutionTimeline } from "@/components/chat/ExecutionTimeline";
import { executionPresentation } from "@/stores/chatStateModel";
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
    message.status !== "awaiting_approval" &&
    message.id &&
    !message.id.startsWith("assistant-") &&
    !message.approvalPending;
  const isThinking = Boolean(message.isThinking);
  const { segments, answer } = executionPresentation(message);
  const hasContent = answer.trim().length > 0;
  const hasApprovalPending = Boolean(message.approvalPending?.required);
  const isWaiting = message.status === "streaming" && !isThinking && !hasContent && segments.length === 0;
  const toolCalls = message.toolCalls ?? [];
  const taskDrafts = draftsFromToolCalls(toolCalls);
  const memoryEvents = message.memoryEvents ?? [];
  const sessionRecallEvents = message.sessionRecallEvents ?? [];
  const hasMemoryEvents = memoryEvents.length > 0;
  const hasSessionRecallEvents = sessionRecallEvents.length > 0;
  const fallbackReason = message.fallbackReason?.trim();

  if (isUser) {
    return (
      <div className="chat-user-row flex">
        <div className="user-message">
          <p className="whitespace-pre-wrap break-words">{message.content}</p>
        </div>
      </div>
    );
  }

  return (
    <div className="chat-assistant-row group flex">
      <div className="chat-assistant-content min-w-0 flex-1 space-y-4">
        <ExecutionTimeline segments={segments} status={message.status} content={message.content} />

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

        <div className={segments.length && hasContent ? "chat-final-answer space-y-2" : "space-y-2"}>
          {hasApprovalPending ? <ApprovalPendingCard approval={message.approvalPending!} /> : null}

          {taskDrafts.map((draft) => (
            <ScheduledTaskDraftCard key={draft.id} draft={draft} />
          ))}

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

          {hasContent ? (
            <>
              {segments.length ? <div className="execution-answer-label">回答</div> : null}
              <CitationNumberProvider content={message.content}>
                <MarkdownRenderer content={answer} />
              </CitationNumberProvider>
            </>
          ) : null}

          {message.sources?.length ? (
            <div className="rounded-lg border border-gray-200 px-3 py-2 text-xs text-gray-600 dark:border-gray-700 dark:text-gray-300">
              <div className="mb-1 font-medium">回答来源</div>
              <ul className="space-y-1">
                {message.sources.map((source, index) => (
                  <li key={`${source.type}-${source.chunkId || source.url || index}`} className="flex items-start gap-1.5">
                    {source.type === "web" ? <Globe className="mt-0.5 h-3 w-3 shrink-0" /> : <FileText className="mt-0.5 h-3 w-3 shrink-0" />}
                    {source.type === "web" && /^https?:\/\//i.test(source.url || "") ? (
                      <a href={source.url} target="_blank" rel="noreferrer" className="break-all text-blue-600 hover:underline dark:text-blue-400">
                        {source.title || source.url}
                      </a>
                    ) : (
                      <span>{source.title || "来源文档"}</span>
                    )}
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

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
