import * as React from "react";
import { Check, ChevronRight, CirclePause, Loader2, Wrench, X } from "lucide-react";

import { CitationNumberProvider } from "@/components/chat/citationContext";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { ThinkingIndicator } from "@/components/chat/ThinkingIndicator";
import { isToolSettled } from "@/stores/chatStateModel";
import type { ExecutionSegment, MessageStatus, ToolCallPayload } from "@/types";

function ToolSegment({ tool, result, status }: { tool: ToolCallPayload; result: boolean; status?: MessageStatus }) {
  const [expanded, setExpanded] = React.useState(false);
  const bodyId = React.useId();
  const failed = ["failed", "error", "rejected"].includes(tool.status);
  const stopped = ["cancelled", "interrupted"].includes(tool.status);
  const success = ["success", "completed"].includes(tool.status);
  const working = !isToolSettled(tool.status) && status === "streaming";
  const label = result
    ? failed ? "Failed" : stopped ? "Stopped" : "Completed"
    : working ? tool.status === "pending" ? "queued" : "running" : isToolSettled(tool.status) ? "" : status === "awaiting_approval" ? "awaiting approval" : "stopped";
  const details = result ? tool.data : tool.arguments;
  const hasDetails = Boolean(details && Object.keys(details).length > 0);
  const canExpand = hasDetails || Boolean(result && tool.summary);
  const Icon = result ? failed ? X : stopped ? CirclePause : Check : working ? Loader2 : Wrench;

  return (
    <div className={`execution-tool ${result ? "execution-result" : "execution-call"}`} data-failed={failed}>
      <button type="button" className="execution-tool-header"
        aria-label={`${result ? "工具结果" : "工具调用"} · ${tool.name} ${label}`}
        aria-expanded={expanded} aria-controls={canExpand ? bodyId : undefined}
        disabled={!canExpand} onClick={() => setExpanded((value) => !value)}>
        <Icon className={`h-3.5 w-3.5 shrink-0 ${working ? "execution-spinner" : ""} ${result && success ? "execution-success" : ""}`} aria-hidden="true" />
        <span className={result ? "execution-result-label" : "execution-tool-name"}>{result ? label : tool.name || "Tool"}</span>
        {!result && typeof tool.round === "number" ? <span className="execution-status">第 {tool.round} 轮</span> : null}
        {result ? <span className="execution-result-name">{tool.name}</span> : label ? <span className="execution-status">{label}</span> : null}
        {typeof tool.durationMs === "number" && tool.durationMs > 0 ? <span className="execution-duration">{(tool.durationMs / 1000).toFixed(1)}s</span> : null}
        {canExpand ? <ChevronRight className={`ml-auto h-3.5 w-3.5 shrink-0 ${expanded ? "rotate-90" : ""}`} aria-hidden="true" /> : null}
      </button>
      {result && tool.summary && !expanded ? <p className="execution-result-preview">{tool.summary.slice(0, 240)}</p> : null}
      {!result && hasDetails && !expanded ? <p className="execution-arguments-preview">{JSON.stringify(details).slice(0, 180)}</p> : null}
      {!result && tool.originalName && tool.originalName !== tool.name ? <p className="execution-arguments-preview">{tool.originalName}</p> : null}
      {expanded && canExpand ? (
        <div id={bodyId} className="execution-tool-details" tabIndex={0} aria-label={result ? "完整工具结果" : "完整工具参数"}>
          {result && tool.summary ? <pre>{tool.summary}</pre> : null}
          {hasDetails ? <pre>{JSON.stringify(details, null, 2)}</pre> : null}
        </div>
      ) : null}
    </div>
  );
}

export function ExecutionTimeline({ segments, status, content }: {
  segments: ExecutionSegment[];
  status?: MessageStatus;
  content: string;
}) {
  const [showAll, setShowAll] = React.useState(false);
  const compact = status !== "streaming" && segments.length > 16 && !showAll;
  const visible = compact ? segments.filter((segment, index) => index < 2 || index >= segments.length - 4 ||
    ((segment.kind === "tool_call" || segment.kind === "tool_result") && ["failed", "error", "rejected"].includes(segment.tool.status))) : segments;
  const hiddenCount = segments.length - visible.length;
  const thinkingCount = segments.filter((segment) => segment.kind === "thinking").length;
  const toolCount = segments.filter((segment) => segment.kind === "tool_call").length;
  if (!segments.length) return null;

  return (
    <section className="chat-execution" aria-label="Agent 执行过程">
      <div className="execution-caption">执行过程 <span>{thinkingCount} 次思考 · {toolCount} 次工具调用</span></div>
      <CitationNumberProvider content={content}>
        <ol className="execution-timeline">
          {visible.map((segment, index) => (
            <React.Fragment key={segment.id}>
              {compact && hiddenCount > 0 && index === 2 ? (
                <li className="execution-step execution-more">
                  <button type="button" onClick={() => setShowAll(true)}>查看其余 {hiddenCount} 项过程 <ChevronRight className="h-3 w-3" /></button>
                </li>
              ) : null}
              <li className={`execution-step execution-${segment.kind}`}>
                {segment.kind === "thinking" ? (
                  <ThinkingIndicator content={segment.content}
                    active={status === "streaming" && segment.status === "working"}
                    interrupted={segment.status === "stopped" || (segment.status === "working" && status !== "streaming" && status !== "done")}
                    duration={segment.durationMs ? segment.durationMs / 1000 : undefined} />
                ) : segment.kind === "text" ? (
                  <div className="execution-narration"><MarkdownRenderer content={segment.content} /></div>
                ) : (
                  <ToolSegment tool={segment.tool} result={segment.kind === "tool_result"} status={status} />
                )}
              </li>
            </React.Fragment>
          ))}
        </ol>
      </CitationNumberProvider>
      {showAll && status !== "streaming" && segments.length > 16 ? <button className="execution-collapse" type="button" onClick={() => setShowAll(false)}>收起较早的执行记录</button> : null}
    </section>
  );
}
