import * as React from "react";
import { ChevronRight, Sparkles } from "lucide-react";

interface ThinkingIndicatorProps {
  content?: string;
  duration?: number;
  active?: boolean;
  interrupted?: boolean;
}

export const ThinkingIndicator = React.memo(function ThinkingIndicator({
  content = "", duration, active = true, interrupted = false
}: ThinkingIndicatorProps) {
  const [expanded, setExpanded] = React.useState(active);
  const bodyId = React.useId();
  const bodyRef = React.useRef<HTMLDivElement>(null);
  const followRef = React.useRef(true);
  React.useEffect(() => { setExpanded(active); }, [active]);
  React.useLayoutEffect(() => {
    if (active && expanded && followRef.current && bodyRef.current) {
      bodyRef.current.scrollTop = bodyRef.current.scrollHeight;
    }
  }, [content, active, expanded]);
  return (
    <div className="execution-thinking" data-active={active}>
      <button type="button" className="execution-thinking-header"
        aria-expanded={expanded} aria-controls={bodyId}
        onClick={() => setExpanded((value) => !value)}>
        <Sparkles className="h-3.5 w-3.5 shrink-0" aria-hidden="true" />
        <span className="font-medium">Thinking</span>
        <span className="execution-status">{active ? "working…" : interrupted ? "stopped" : "completed"}</span>
        {!active && typeof duration === "number" && duration > 0 ? <span className="execution-duration">{Math.max(1, Math.round(duration))}s</span> : null}
        {active ? <span className="execution-live-dot" aria-hidden="true" /> : null}
        <ChevronRight className={`ml-auto h-3.5 w-3.5 shrink-0 ${expanded ? "rotate-90" : ""}`} aria-hidden="true" />
      </button>
      {expanded ? (
        <div id={bodyId} ref={bodyRef} className="execution-thinking-body" tabIndex={0} aria-label="Thinking 内容"
          onScroll={(event) => {
            const node = event.currentTarget;
            followRef.current = node.scrollHeight - node.scrollTop - node.clientHeight < 24;
          }}>
          {content || (active ? "正在分析任务…" : "暂无思考内容")}
        </div>
      ) : null}
    </div>
  );
});
