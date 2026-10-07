import * as React from "react";
import { Virtuoso, type VirtuosoHandle } from "react-virtuoso";

import { MessageItem } from "@/components/chat/MessageItem";
import { WelcomeScreen } from "@/components/chat/WelcomeScreen";
import { cn } from "@/lib/utils";
import type { Message } from "@/types";

interface MessageListProps {
  messages: Message[];
  isLoading: boolean;
  isStreaming: boolean;
  sessionKey?: string | null;
}

export function MessageList({ messages, isLoading, isStreaming, sessionKey }: MessageListProps) {
  const virtuosoRef = React.useRef<VirtuosoHandle | null>(null);
  const scrollerRef = React.useRef<HTMLElement | null>(null);
  const followLatestRef = React.useRef(true);
  const scrollerCleanupRef = React.useRef<(() => void) | null>(null);
  const lastSessionRef = React.useRef<string | null>(null);
  const pendingScrollRef = React.useRef(true);
  const settleTimerRef = React.useRef<number | null>(null);
  const heightScrollRafRef = React.useRef<number | null>(null);
  const prevStreamingRef = React.useRef(false);
  const initialTopMostItemIndex = React.useMemo(
    () => ({ index: "LAST" as const, align: "end" as const }),
    []
  );

  const scrollToBottom = React.useCallback(() => {
    if (!followLatestRef.current) return;
    virtuosoRef.current?.scrollToIndex({ index: "LAST", align: "end", behavior: "auto" });
    const scroller = scrollerRef.current;
    if (scroller) {
      scroller.scrollTop = scroller.scrollHeight;
    }
  }, []);

  const stickToBottom = React.useCallback(() => {
    if (!followLatestRef.current) return;
    const scroller = scrollerRef.current;
    if (!scroller) return;
    scroller.scrollTop = scroller.scrollHeight;
  }, []);

  React.useEffect(() => {
    const nextKey = sessionKey ?? "empty";
    if (lastSessionRef.current !== nextKey) {
      lastSessionRef.current = nextKey;
      followLatestRef.current = true;
      pendingScrollRef.current = true;
      if (settleTimerRef.current) {
        window.clearTimeout(settleTimerRef.current);
        settleTimerRef.current = null;
      }
    }
  }, [sessionKey]);

  React.useEffect(() => {
    const wasStreaming = prevStreamingRef.current;
    prevStreamingRef.current = isStreaming;
    if (!wasStreaming && isStreaming) {
      followLatestRef.current = true;
      stickToBottom();
      const timer = window.setTimeout(stickToBottom, 120);
      return () => window.clearTimeout(timer);
    }
    if (wasStreaming && !isStreaming) {
      if (!followLatestRef.current) return;
      scrollToBottom();
      const finishScroll = () => { if (followLatestRef.current) scrollToBottom(); };
      const timer = window.setTimeout(finishScroll, 120);
      const lateTimer = window.setTimeout(finishScroll, 360);
      return () => {
        window.clearTimeout(timer);
        window.clearTimeout(lateTimer);
      };
    }
    return;
  }, [isStreaming, stickToBottom, scrollToBottom]);

  React.useLayoutEffect(() => {
    if (!pendingScrollRef.current || isStreaming || isLoading || messages.length === 0) {
      return;
    }
    let attempts = 0;
    let rafId = 0;
    let active = true;
    const run = () => {
      scrollToBottom();
      attempts += 1;
      if (attempts < 3) {
        rafId = window.requestAnimationFrame(run);
      }
    };
    run();
    const timer = window.setTimeout(scrollToBottom, 240);
    const lateTimer = window.setTimeout(scrollToBottom, 900);
    const handleLoad = () => {
      if (active) {
        scrollToBottom();
      }
    };
    if (document.readyState === "complete") {
      handleLoad();
    } else {
      window.addEventListener("load", handleLoad, { once: true });
    }
    if (document.fonts?.ready) {
      document.fonts.ready.then(() => {
      if (active) {
        scrollToBottom();
      }
    });
  }
    if (settleTimerRef.current) {
      window.clearTimeout(settleTimerRef.current);
    }
    settleTimerRef.current = window.setTimeout(() => {
      pendingScrollRef.current = false;
      settleTimerRef.current = null;
    }, 1500);
    return () => {
      active = false;
      window.cancelAnimationFrame(rafId);
      window.clearTimeout(timer);
      window.clearTimeout(lateTimer);
      if (settleTimerRef.current) {
        window.clearTimeout(settleTimerRef.current);
        settleTimerRef.current = null;
      }
      window.removeEventListener("load", handleLoad);
    };
  }, [messages.length, isStreaming, isLoading, sessionKey]);

  React.useEffect(() => {
    return () => {
      scrollerCleanupRef.current?.();
      if (heightScrollRafRef.current) {
        window.cancelAnimationFrame(heightScrollRafRef.current);
        heightScrollRafRef.current = null;
      }
      if (settleTimerRef.current) {
        window.clearTimeout(settleTimerRef.current);
        settleTimerRef.current = null;
      }
    };
  }, []);

  const handleTotalListHeightChanged = React.useCallback(() => {
    if (isLoading) {
      return;
    }
    const shouldStick = (isStreaming && followLatestRef.current) || pendingScrollRef.current;
    if (!shouldStick) return;
    if (heightScrollRafRef.current) {
      return;
    }
    heightScrollRafRef.current = window.requestAnimationFrame(() => {
      heightScrollRafRef.current = null;
      if (isStreaming) {
        stickToBottom();
      } else {
        if (followLatestRef.current) scrollToBottom();
      }
    });
  }, [isStreaming, isLoading, scrollToBottom, stickToBottom]);

  const List = React.useMemo(() => {
    const Comp = React.forwardRef<HTMLDivElement, React.HTMLAttributes<HTMLDivElement>>(
      ({ className, ...props }, ref) => (
        <div
          ref={ref}
          className={cn("chat-message-list mx-auto w-full max-w-[820px] space-y-10 px-5 pb-6 pt-8 md:px-8", className)}
          {...props}
        />
      )
    );
    Comp.displayName = "MessageList";
    return Comp;
  }, []);

  const Footer = React.useMemo(() => {
    const Comp = () => <div aria-hidden="true" className="chat-message-list-footer h-12" />;
    Comp.displayName = "MessageListFooter";
    return Comp;
  }, []);

  if (messages.length === 0) {
    if (isLoading) {
      return <div className="h-full" />;
    }
    return <WelcomeScreen />;
  }

  return (
    <Virtuoso
      key={sessionKey ?? "empty"}
      ref={virtuosoRef}
      data={messages}
      initialTopMostItemIndex={initialTopMostItemIndex}
      followOutput={(atBottom) => {
        if (isStreaming) return false;
        return atBottom ? "auto" : false;
      }}
      scrollerRef={(node) => {
        scrollerCleanupRef.current?.();
        scrollerRef.current = node as HTMLElement | null;
        if (!(node instanceof HTMLElement)) return;
        const handleScroll = () => {
          const atBottom = node.scrollHeight - node.scrollTop - node.clientHeight < 48;
          if (atBottom) followLatestRef.current = true;
        };
        const pauseFollowing = () => {
          followLatestRef.current = false;
          pendingScrollRef.current = false;
        };
        const handleKey = (event: KeyboardEvent) => {
          if (["ArrowUp", "PageUp", "Home"].includes(event.key)) pauseFollowing();
        };
        const handleWheel = (event: WheelEvent) => {
          if (event.deltaY < 0) pauseFollowing();
        };
        node.addEventListener("scroll", handleScroll, { passive: true });
        node.addEventListener("wheel", handleWheel, { passive: true });
        node.addEventListener("touchstart", pauseFollowing, { passive: true });
        node.addEventListener("pointerdown", pauseFollowing);
        node.addEventListener("keydown", handleKey);
        scrollerCleanupRef.current = () => {
          node.removeEventListener("scroll", handleScroll);
          node.removeEventListener("wheel", handleWheel);
          node.removeEventListener("touchstart", pauseFollowing);
          node.removeEventListener("pointerdown", pauseFollowing);
          node.removeEventListener("keydown", handleKey);
        };
      }}
      totalListHeightChanged={handleTotalListHeightChanged}
      className="h-full"
      components={{ List, Footer }}
      itemContent={(index, message) => (
        <div>
          <MessageItem message={message} isLast={index === messages.length - 1} />
        </div>
      )}
    />
  );
}
