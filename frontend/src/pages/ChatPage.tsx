import * as React from "react";
import { useNavigate, useParams } from "react-router-dom";

import { ChatInput } from "@/components/chat/ChatInput";
import { MessageList } from "@/components/chat/MessageList";
import { draftsFromToolCalls, ScheduledTaskDraftCard } from "@/components/chat/ScheduledTaskDraftCard";
import { MainLayout } from "@/components/layout/MainLayout";
import { useChatStore } from "@/stores/chatStore";
import { listPendingTaskDrafts, markScheduledConversationRead, type TaskDraft } from "@/services/scheduledTaskService";

export function ChatPage() {
  const navigate = useNavigate();
  const { sessionId } = useParams<{ sessionId: string }>();
  const {
    messages,
    isLoading,
    isStreaming,
    currentSessionId,
    sessions,
    isCreatingNew,
    fetchSessions,
    selectSession,
    createSession
  } = useChatStore();
  const showWelcome = messages.length === 0 && !isLoading;
  const [sessionsReady, setSessionsReady] = React.useState(false);
  const [pendingDrafts, setPendingDrafts] = React.useState<TaskDraft[]>([]);
  const sessionExists = React.useMemo(() => {
    if (!sessionId) return false;
    return sessions.some((session) => session.id === sessionId);
  }, [sessionId, sessions]);
  const latestSessionId = React.useMemo(() => sessions[0]?.id || null, [sessions]);

  React.useEffect(() => {
    let active = true;
    fetchSessions()
      .catch(() => null)
      .finally(() => {
        if (active) {
          setSessionsReady(true);
        }
      });
    return () => {
      active = false;
    };
  }, [fetchSessions]);

  React.useEffect(() => {
    if (sessionId) {
      if (sessionsReady && !sessionExists) {
        createSession().catch(() => null);
        navigate("/chat", { replace: true });
        return;
      }
      selectSession(sessionId).catch(() => null);
      return;
    }
    if (!sessionsReady) {
      return;
    }
    if (isCreatingNew) {
      return;
    }
    if (currentSessionId) {
      return;
    }
    if (latestSessionId) {
      selectSession(latestSessionId).catch(() => null);
      navigate(`/chat/${latestSessionId}`, { replace: true });
      return;
    }
    createSession().catch(() => null);
  }, [
    sessionId,
    sessionsReady,
    sessionExists,
    isCreatingNew,
    currentSessionId,
    latestSessionId,
    selectSession,
    createSession,
    navigate
  ]);

  React.useEffect(() => {
    // An explicit /chat/:id navigation selects that conversation asynchronously.
    // Do not navigate back to the previously selected ID while it is loading.
    if (!sessionId && currentSessionId) {
      navigate(`/chat/${currentSessionId}`, { replace: true });
    }
  }, [currentSessionId, sessionId, navigate]);

  React.useEffect(() => {
    if (sessionId && sessionExists) {
      markScheduledConversationRead(sessionId).catch(() => null);
    }
  }, [sessionId, sessionExists, messages.length]);

  // A reload drops the tool events that carried the confirmation cards, so the
  // pending drafts of the conversation are asked for separately.
  React.useEffect(() => {
    setPendingDrafts([]);
    if (!sessionId || !sessionExists) {
      return;
    }
    let active = true;
    listPendingTaskDrafts(sessionId)
      .then((drafts) => {
        if (active) {
          setPendingDrafts(drafts);
        }
      })
      .catch(() => null);
    return () => {
      active = false;
    };
  }, [sessionId, sessionExists]);

  // Cards already rendered from a tool event stay the only copy of that draft.
  const restoredDrafts = React.useMemo(() => {
    const rendered = new Set<string>();
    for (const message of messages) {
      for (const draft of draftsFromToolCalls(message.toolCalls ?? [])) {
        rendered.add(draft.id);
      }
    }
    return pendingDrafts.filter((draft) => !rendered.has(draft.id));
  }, [pendingDrafts, messages]);

  return (
    <MainLayout variant="chat">
      <div className="chat-page-content flex h-full min-w-0 flex-col bg-white">
        <div className="flex-1 min-h-0">
          <MessageList
            messages={messages}
            isLoading={isLoading}
            isStreaming={isStreaming}
            sessionKey={currentSessionId}
          />
        </div>
        {restoredDrafts.length > 0 ? (
          <div className="mx-auto w-full max-w-[820px] space-y-2 px-5 pb-3 md:px-8">
            {restoredDrafts.map((draft) => (
              <ScheduledTaskDraftCard key={draft.id} draft={draft} />
            ))}
          </div>
        ) : null}
        {showWelcome ? null : (
          <div className="chat-input-dock relative z-20 bg-white">
            <div className="mx-auto max-w-[820px] px-6 pb-4 pt-1">
              <ChatInput />
            </div>
          </div>
        )}
      </div>
    </MainLayout>
  );
}
