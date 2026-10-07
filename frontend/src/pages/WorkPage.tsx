import { useCallback, useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { createPortal } from "react-dom";
import {
  ArrowLeft,
  Archive,
  BriefcaseBusiness,
  Menu,
  MessageSquare,
  PanelRightClose,
  PanelRightOpen,
  Plus,
  Moon,
  Sun,
  X
} from "lucide-react";
import { toast } from "sonner";
import { MarkdownRenderer } from "@/components/chat/MarkdownRenderer";
import { WorkProgressPanel } from "@/components/work/WorkProgressPanel";
import { WorkDocumentPanel, type WorkDocumentHandle } from "@/components/work/WorkDocumentPanel";
import { WorkMaterialsPanel } from "@/components/work/WorkMaterialsPanel";
import { WorkCitationChip } from "@/components/work/WorkCitationChip";
import { WorkDocumentEditor } from "@/components/work/WorkDocumentEditor";
import { WorkDocumentSuggestions } from "@/components/work/WorkDocumentSuggestions";
import { useAuthStore } from "@/stores/authStore";
import { useThemeStore } from "@/stores/themeStore";
import { storage } from "@/utils/storage";
import {
  workService,
  workRequestId,
  type WorkTopic,
  type WorkItem,
  type WorkConversation,
  type WorkArtifact,
  type WorkState,
  type WorkProposal,
  type WorkMessage,
  type WorkAction,
  type WorkReference,
  type WorkTurn,
  type WorkDocument,
  type WorkNode,
  type WorkChatInput
} from "@/services/workService";
import "@/styles/work.css";

function documentText(node: WorkNode): string {
  if (node.type === "text") return node.text || "";
  return (node.content || [])
    .map(documentText)
    .join(["paragraph", "heading"].includes(node.type) ? "" : "\n");
}

function WorkNavigation({ topics, close }: { topics: WorkTopic[]; close: () => void }) {
  const user = useAuthStore((s) => s.user);
  const logout = useAuthStore((s) => s.logout);
  const theme = useThemeStore((s) => s.theme);
  const toggleTheme = useThemeStore((s) => s.toggleTheme);
  return (
    <>
      <Link className="work-brand" to="/work" onClick={close}>
        <BriefcaseBusiness size={20} /> Work
      </Link>
      <nav className="work-nav">
        <Link to="/chat" onClick={close}>
          聊天
        </Link>
        <Link to="/brief" onClick={close}>
          每日简报
        </Link>
        <Link to="/scheduled-tasks" onClick={close}>
          定时任务
        </Link>
        <Link className="selected" to="/work" onClick={close}>
          Work 专区
        </Link>
      </nav>
      <div className="work-sidebar-label">我的专题</div>
      <div className="work-topic-links">
        {topics.map((t) => (
          <Link key={t.id} to={`/work/${t.id}`} onClick={close}>
            {t.name}
          </Link>
        ))}
      </div>
      <div id="work-conversation-navigation" />
      <div className="work-user">
        {user?.role === "admin" && <Link to="/admin">管理后台</Link>}
        <span>{user?.username}</span>
        <button
          aria-label={theme === "dark" ? "切换浅色模式" : "切换深色模式"}
          onClick={toggleTheme}
        >
          {theme === "dark" ? <Sun size={14} /> : <Moon size={14} />}
        </button>
        <button onClick={() => logout()}>退出登录</button>
      </div>
    </>
  );
}

export function WorkPage() {
  const { topicId } = useParams();
  const navigate = useNavigate();
  const [topics, setTopics] = useState<WorkTopic[]>([]);
  const [status, setStatus] = useState("active");
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [busy, setBusy] = useState(false);
  const [navOpen, setNavOpen] = useState(false);
  const fetchTopics = useCallback(async () => {
    setTopics(await workService.topics(status));
  }, [status]);
  useEffect(() => {
    let valid = true;
    workService
      .topics(status)
      .then((t) => {
        if (valid) setTopics(t);
      })
      .catch(() => null);
    return () => {
      valid = false;
    };
  }, [status]);
  const create = async () => {
    if (!name.trim() || busy) return;
    setBusy(true);
    try {
      const t = await workService.createTopic(name.trim(), description.trim());
      setCreating(false);
      setName("");
      setDescription("");
      await fetchTopics();
      navigate(`/work/${t.id}`);
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="work-shell">
      <aside className={`work-sidebar ${navOpen ? "mobile-open" : ""}`}>
        <button
          className="work-mobile-close"
          aria-label="关闭导航"
          onClick={() => setNavOpen(false)}
        >
          <X size={20} />
        </button>
        <WorkNavigation topics={topics} close={() => setNavOpen(false)} />
      </aside>
      {navOpen && (
        <button
          className="work-nav-backdrop"
          aria-label="关闭导航"
          onClick={() => setNavOpen(false)}
        />
      )}
      {topicId ? (
        <WorkTopicRoom
          key={topicId}
          topicId={topicId}
          refreshTopics={fetchTopics}
          openNavigation={() => setNavOpen(true)}
          closeNavigation={() => setNavOpen(false)}
        />
      ) : (
        <main className="work-home">
          <header>
            <button
              className="work-mobile-menu"
              aria-label="打开导航"
              onClick={() => setNavOpen(true)}
            >
              <Menu size={20} />
            </button>
            <span className="work-eyebrow">你的长期协作空间</span>
            <h1>把一件事，持续做好。</h1>
            <p>
              学习一个领域、完成技术选型，或推进日常工作。每个专题都保留讨论、进展、文档与资料。
            </p>
            <button className="primary" onClick={() => setCreating(true)}>
              <Plus size={16} /> 新建专题
            </button>
          </header>
          <div className="work-home-filter">
            <button
              className={status === "active" ? "selected" : ""}
              onClick={() => setStatus("active")}
            >
              进行中的专题
            </button>
            <button
              className={status === "archived" ? "selected" : ""}
              onClick={() => setStatus("archived")}
            >
              已归档
            </button>
          </div>
          <div className="work-topic-grid">
            {topics.map((t) => (
              <Link className="work-topic-card" key={t.id} to={`/work/${t.id}`}>
                <BriefcaseBusiness size={22} />
                <h2>{t.name}</h2>
                <p>{t.description || "从下一次讨论继续推进"}</p>
                <small>
                  最近活动：
                  {new Date(t.updatedAt).toLocaleString("zh-CN", {
                    month: "numeric",
                    day: "numeric",
                    hour: "2-digit",
                    minute: "2-digit"
                  })}
                </small>
                <small>{t.status === "archived" ? "已归档 · 可以查看和恢复" : "继续协作"} →</small>
              </Link>
            ))}
          </div>
          {!topics.length && (
            <div className="work-empty">
              <h2>{status === "archived" ? "还没有归档专题" : "从一个想持续推进的主题开始"}</h2>
              {status === "active" && <p>例如：学习数据分析、选择消息队列、处理本周工作。</p>}
            </div>
          )}
          {topics.length > 0 && topics.length % 30 === 0 && (
            <button
              onClick={async () =>
                setTopics([...topics, ...(await workService.topics(status, topics.length))])
              }
            >
              加载更多专题
            </button>
          )}
        </main>
      )}
      {creating && (
        <div className="work-modal-backdrop">
          <section
            role="dialog"
            aria-modal="true"
            aria-labelledby="work-create-title"
            className="work-modal"
          >
            <h2 id="work-create-title">新建专题</h2>
            <label>
              名称
              <input
                autoFocus
                maxLength={128}
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="例如：学习数据分析"
              />
            </label>
            <label>
              一句描述
              <textarea
                value={description}
                onChange={(e) => setDescription(e.target.value)}
                placeholder="你想持续推进什么？"
              />
            </label>
            <p className="work-muted">目标与下一步可以在协作中逐渐形成。</p>
            <div className="work-panel-actions">
              <button disabled={busy} onClick={() => setCreating(false)}>
                取消
              </button>
              <button className="primary" disabled={busy || !name.trim()} onClick={create}>
                {busy ? "创建中…" : "创建专题"}
              </button>
            </div>
          </section>
        </div>
      )}
    </div>
  );
}

function WorkTopicRoom({
  topicId,
  refreshTopics,
  openNavigation,
  closeNavigation
}: {
  topicId: string;
  refreshTopics: () => Promise<void>;
  openNavigation: () => void;
  closeNavigation: () => void;
}) {
  const userId = storage.getUser()?.userId || "";
  const viewKey = `work-view:${userId}:${topicId}`;
  const [initialView] = useState(() => {
    try {
      return JSON.parse(sessionStorage.getItem(viewKey) || "{}");
    } catch {
      return {};
    }
  });
  const [topic, setTopic] = useState<WorkTopic>();
  const [items, setItems] = useState<WorkItem[]>([]);
  const [conversations, setConversations] = useState<WorkConversation[]>([]);
  const [artifacts, setArtifacts] = useState<WorkArtifact[]>([]);
  const [state, setState] = useState<WorkState>({ revision: 1, entries: [] });
  const [proposals, setProposals] = useState<WorkProposal[]>([]);
  const [conversationId, setConversationId] = useState<string>(initialView.conversationId || "");
  const [itemId, setItemId] = useState<string>(initialView.itemId || "");
  const [continueFrom, setContinueFrom] = useState("");
  const [artifactId, setArtifactId] = useState<string>(initialView.artifactId || "");
  const [tab, setTab] = useState<string>(initialView.tab || "progress");
  const [panel, setPanel] = useState(() => localStorage.getItem("work-panel-open") !== "false");
  const [mobilePanel, setMobilePanel] = useState(false);
  const [messages, setMessages] = useState<WorkMessage[]>([]);
  const [turns, setTurns] = useState<WorkTurn[]>([]);
  const [dispatching, setDispatching] = useState(false);
  const [question, setQuestion] = useState("");
  const [action, setAction] = useState<WorkAction>("discuss");
  const [running, setRunning] = useState(false);
  const [turnId, setTurnId] = useState("");
  const [streamText, setStreamText] = useState("");
  const [streamStatus, setStreamStatus] = useState("");
  const [refreshKey, setRefreshKey] = useState(0);
  const [reference, setReference] = useState<{
    title: string;
    content: string;
    body?: WorkDocument;
  }>();
  const [newItem, setNewItem] = useState("");
  const [showItemForm, setShowItemForm] = useState(false);
  const doc = useRef<WorkDocumentHandle>(null);
  const epoch = useRef(0);
  const stream = useRef<ReturnType<typeof workService.stream>>();
  const bottom = useRef<HTMLDivElement>(null);
  const activeConversation = useRef("");
  const [autoIntent, setAutoIntent] = useState(true);
  const [uploading, setUploading] = useState(false);
  const [pendingSources, setPendingSources] = useState<string[]>([]);
  const retryInput = useRef<WorkChatInput>();
  const navigationTarget = document.getElementById("work-conversation-navigation");
  const activeKey = `work-turn:${userId}:${topicId}`;
  const readOnly = topic?.status === "archived";
  useEffect(() => {
    sessionStorage.setItem(viewKey, JSON.stringify({ conversationId, itemId, artifactId, tab }));
  }, [viewKey, conversationId, itemId, artifactId, tab]);
  useEffect(() => {
    let valid = true;
    workService
      .sources(topicId, "")
      .then((sources) => {
        if (valid)
          setPendingSources(
            sources.filter((s) => !s.promoted && !s.conversationId).map((s) => s.id)
          );
      })
      .catch(() => null);
    if (initialView.conversationId && !localStorage.getItem(activeKey)) {
      Promise.all([
        workService.messages(topicId, initialView.conversationId),
        workService.turns(topicId, initialView.conversationId)
      ])
        .then(([m, t]) => {
          if (valid) {
            setMessages(m.reverse());
            setTurns(t);
            const active = t.find((turn) => ["accepted", "running"].includes(turn.status));
            if (active) {
              setTurnId(active.id);
              setRunning(true);
              runStream(active.id, epoch.current);
            }
          }
        })
        .catch(() => {
          if (valid) setConversationId("");
        });
    }
    return () => {
      valid = false;
    };
  }, [topicId, activeKey, initialView]);
  const knowledgeCitation = useCallback(
    ({ node }: { node?: Parameters<typeof WorkCitationChip>[0]["node"] }) => (
      <WorkCitationChip node={node} topicId={topicId} conversationId={conversationId} />
    ),
    [topicId, conversationId]
  );
  const refresh = useCallback(async () => {
    const generation = epoch.current;
    const [t, i, c, a, s, p] = await Promise.all([
      workService.topic(topicId),
      workService.items(topicId),
      workService.conversations(topicId),
      workService.artifacts(topicId),
      workService.state(topicId),
      workService.proposals(topicId)
    ]);
    if (generation !== epoch.current) return;
    setTopic(t);
    setItems(i);
    setConversations(c);
    setArtifacts(a);
    setState(s);
    setProposals(p);
  }, [topicId]);
  useEffect(() => {
    refresh().catch(() => null);
    return () => {
      ++epoch.current;
      stream.current?.cancel();
    };
  }, [refresh]);
  useEffect(() => {
    bottom.current?.scrollIntoView({ block: "end" });
  }, [streamText, messages.length]);
  useEffect(() => {
    activeConversation.current = conversationId;
  }, [conversationId]);
  useEffect(() => {
    const key = `work-input:${userId}:${topicId}:${conversationId || "new"}`;
    setQuestion(sessionStorage.getItem(key) || "");
  }, [userId, topicId, conversationId]);
  const changeQuestion = (value: string) => {
    setQuestion(value);
    sessionStorage.setItem(`work-input:${userId}:${topicId}:${conversationId || "new"}`, value);
  };
  const selectConversation = async (id: string) => {
    closeNavigation();
    stream.current?.cancel();
    ++epoch.current;
    setConversationId(id);
    activeConversation.current = id;
    setStreamText("");
    setRunning(false);
    setTurnId("");
    setContinueFrom("");
    setAction("discuss");
    const generation = epoch.current;
    const c = conversations.find((x) => x.id === id);
    setItemId(c?.itemId || "");
    setMessages([]);
    setTurns([]);
    if (id) {
      const [value, history] = await Promise.all([
        workService.messages(topicId, id),
        workService.turns(topicId, id)
      ]);
      if (generation === epoch.current) {
        setMessages(value.reverse());
        setTurns(history);
        const active = history.find((t) => ["accepted", "running"].includes(t.status));
        if (active) {
          setTurnId(active.id);
          setRunning(true);
          await runStream(active.id, generation);
        }
      }
    }
  };
  const showPanel = (next: string) => {
    setTab(next);
    setPanel(true);
    setMobilePanel(true);
    localStorage.setItem("work-panel-open", "true");
  };
  const runStream = async (
    input: Parameters<typeof workService.stream>[1],
    generation: number,
    initial = "",
    initialOffset = 0
  ) => {
    let content = initial;
    let offset = initialOffset;
    let currentTurn = typeof input === "string" ? input : "";
    let currentConversation = conversationId;
    let terminal = false;
    const valid = () => generation === epoch.current;
    const handlers: Parameters<typeof workService.stream>[2] = {
      onEvent: () => {
        if (!valid()) return;
        offset++;
        if (currentTurn)
          localStorage.setItem(
            activeKey,
            JSON.stringify({
              turnId: currentTurn,
              conversationId: currentConversation,
              itemId,
              content,
              offset
            })
          );
      },
      onMeta: (meta) => {
        currentTurn = meta.taskId;
        currentConversation = meta.conversationId;
        if (!valid()) return;
        setTurnId(currentTurn);
        setConversationId(currentConversation);
        activeConversation.current = currentConversation;
        localStorage.setItem(
          activeKey,
          JSON.stringify({
            turnId: currentTurn,
            conversationId: currentConversation,
            itemId,
            content,
            offset
          })
        );
      },
      onMessage: (value) => {
        if (value.type === "response" || value.type === "text") {
          content += value.delta;
          if (valid()) setStreamText(content);
          if (currentTurn)
            localStorage.setItem(
              activeKey,
              JSON.stringify({
                turnId: currentTurn,
                conversationId: currentConversation,
                itemId,
                content,
                offset
              })
            );
        }
      },
      onFinish: (value) => {
        if (!valid() || value.content === undefined) return;
        content = value.content;
        setStreamText(content);
        if (currentTurn)
          localStorage.setItem(
            activeKey,
            JSON.stringify({ turnId: currentTurn, conversationId: currentConversation, itemId, content, offset })
          );
      },
      onToolStart: (value) => {
        if (valid())
          setStreamStatus(value.name.startsWith("work_") ? "正在处理专题内容…" : "正在查阅资料…");
      },
      onTool: (value) => {
        if (valid())
          setStreamStatus(value.name.startsWith("work_") ? "正在处理专题内容…" : "正在查阅资料…");
      },
      onError: (error) => {
        if (valid()) setStreamStatus(error.message);
      },
      onDone: () => {
        terminal = true;
      },
      onCancel: () => {
        terminal = true;
      }
    };
    try {
      stream.current = workService.stream(topicId, input, handlers, initialOffset);
      await stream.current.start();
      if (!valid()) return;
      if (!terminal) {
        setStreamStatus("连接已断开，可重连查看本轮结果。");
        return false;
      }
      localStorage.removeItem(activeKey);
      await refresh();
      if (!valid()) return false;
      const [stored, history] = await Promise.all([
        workService.messages(topicId, currentConversation),
        workService.turns(topicId, currentConversation)
      ]);
      if (!valid()) return false;
      setMessages(stored.reverse());
      setTurns(history);
      setStreamText("");
      setStreamStatus("");
      setRefreshKey((n) => n + 1);
      return true;
    } catch (error) {
      if (valid())
        setStreamStatus(
          `连接未完成，输入已保留。${currentTurn ? "可重连本轮。" : (error as Error).message}`
        );
    } finally {
      if (valid()) setRunning(false);
    }
  };
  useEffect(() => {
    let raw: {
      turnId: string;
      conversationId: string;
      itemId: string;
      content: string;
      offset: number;
    } | null = null;
    try {
      raw = JSON.parse(localStorage.getItem(activeKey) || "null");
    } catch {
      localStorage.removeItem(activeKey);
    }
    if (!raw) return;
    const value = raw;
    setConversationId(value.conversationId);
    setItemId(value.itemId || "");
    setTurnId(value.turnId);
    setStreamText(value.content || "");
    setRunning(true);
    const generation = epoch.current;
    workService
      .messages(topicId, value.conversationId)
      .then((m) => {
        if (generation === epoch.current) setMessages(m.reverse());
      })
      .catch(() => null);
    runStream(value.turnId, generation, value.content, value.offset);
  }, [activeKey]);
  const send = async () => {
    if (!question.trim() || running || dispatching || readOnly || uploading) return;
    setDispatching(true);
    try {
      const generation = epoch.current;
      const inputQuestion = question.trim();
      const oldConversation = conversationId;
      let chosenAction = action;
      setStreamStatus("正在确认本轮工作…");
      if (autoIntent) {
        try {
          chosenAction = (await workService.intent(topicId, inputQuestion, artifactId)).action;
        } catch {
          setStreamStatus("无法识别，请手动选择本轮工作方式再发送。");
          return;
        }
      }
      if (generation !== epoch.current) return;
      const attachments = await workService.sources(topicId, "");
      const unbound = attachments.filter((s) => !s.promoted && !s.conversationId).map((s) => s.id);
      if (unbound.length > 20) {
        toast.error("每轮最多附加 20 份资料，请先移除多余附件或加入专题。");
        return;
      }
      setPendingSources(unbound);
      let current = doc.current?.selected();
      try {
        if ((chosenAction === "edit_document" || chosenAction === "rewrite_document") && current)
          current = await doc.current!.save();
      } catch {
        return;
      }
      if ((chosenAction === "edit_document" || chosenAction === "rewrite_document") && !current) {
        toast.error("请先选择一份文档。");
        showPanel("documents");
        return;
      }
      setRunning(true);
      setStreamText("");
      setStreamStatus("AI 正在接续专题…");
      setMessages((m) => [...m, { id: workRequestId(), role: "user", content: inputQuestion }]);
      const candidate: WorkChatInput = {
        requestId: "",
        question: inputQuestion,
        conversationId: oldConversation,
        continueFrom,
        itemId,
        artifactId: current?.artifact.id || artifactId,
        artifactRevision: current?.version.revision || 0,
        action: chosenAction,
        sourceIds: unbound
      };
      if (JSON.stringify({ ...retryInput.current, requestId: "" }) !== JSON.stringify(candidate))
        retryInput.current = { ...candidate, requestId: workRequestId() };
      const finished = await runStream(retryInput.current!, generation);
      if (finished && generation === epoch.current) {
        setQuestion("");
        sessionStorage.removeItem(`work-input:${userId}:${topicId}:${oldConversation || "new"}`);
        setContinueFrom("");
        setAction("discuss");
        setAutoIntent(true);
        setPendingSources([]);
        retryInput.current = undefined;
      }
    } finally {
      setDispatching(false);
    }
  };
  const openReference = async (ref: WorkReference) => {
    try {
      if (ref.kind === "artifact") {
        const v = await workService.version(topicId, ref.id, ref.revision || 1);
        setReference({
          title: `${v.title} · v${v.revision}`,
          content: documentText(v.body.root)
        });
      } else {
        const m = await workService.message(topicId, ref.id);
        setReference({ title: "原始讨论依据", content: m.content });
      }
    } catch {
      setReference({
        title: "依据暂不可用",
        content: "来源已删除或当前无法访问。已确认进展仍保留。"
      });
    }
  };
  return (
    <main
      className={`work-room ${panel ? "panel-open" : ""} ${tab === "documents" ? "document-open" : ""}`}
    >
      <header className="work-room-header">
        <button className="work-mobile-menu" aria-label="打开导航" onClick={openNavigation}>
          <Menu size={20} />
        </button>
        <Link to="/work" aria-label="返回 Work">
          <ArrowLeft size={18} />
        </Link>
        <div>
          <h1>{topic?.name || "载入专题…"}</h1>
          <small>{readOnly ? "已归档 · 只读" : topic?.description}</small>
        </div>
        <button
          disabled={!topic || running}
          onClick={async () => {
            if (!topic) return;
            setTopic(
              await workService.updateTopic(topic, { status: readOnly ? "active" : "archived" })
            );
            await refreshTopics();
          }}
        >
          <Archive size={16} />
          {readOnly ? "恢复专题" : "归档"}
        </button>
        <button
          aria-label={panel ? "收起专题面板" : "展开专题面板"}
          onClick={() => {
            setPanel(!panel);
            setMobilePanel(!panel);
            localStorage.setItem("work-panel-open", String(!panel));
          }}
        >
          {panel ? <PanelRightClose size={19} /> : <PanelRightOpen size={19} />}
        </button>
      </header>
      {navigationTarget &&
        createPortal(
          <div className="work-conversations">
            <div className="work-panel-actions">
              <h2>专题对话</h2>
              <button aria-label="新对话" disabled={running} onClick={() => selectConversation("")}>
                <Plus size={17} />
              </button>
            </div>
            {conversations.map((c) => (
              <div className="work-conversation-row" key={c.id}>
                <button
                  className={conversationId === c.id ? "selected" : ""}
                  disabled={running || dispatching}
                  onClick={() => selectConversation(c.id)}
                >
                  <MessageSquare size={14} />
                  <span>{c.title}</span>
                </button>
                <button
                  title="接续为新对话"
                  disabled={running || readOnly}
                  onClick={async () => {
                    await selectConversation("");
                    setContinueFrom(c.id);
                    setItemId(c.itemId || "");
                  }}
                >
                  ↗
                </button>
                <button
                  aria-label={`删除对话 ${c.title}`}
                  disabled={running || readOnly}
                  onClick={async () => {
                    await workService.deleteConversation(c.id);
                    if (conversationId === c.id) await selectConversation("");
                    await refresh();
                  }}
                >
                  ×
                </button>
              </div>
            ))}
            {conversations.length > 0 && conversations.length % 30 === 0 && (
              <button
                onClick={async () =>
                  setConversations([
                    ...conversations,
                    ...(await workService.conversations(topicId, conversations.length))
                  ])
                }
              >
                更多对话
              </button>
            )}
          </div>,
          navigationTarget
        )}
      <section className={`work-chat ${mobilePanel && panel ? "mobile-hidden" : ""}`}>
        <div className="work-chat-context">
          <select
            aria-label="当前事项"
            disabled={running || readOnly}
            value={itemId}
            onChange={(e) => setItemId(e.target.value)}
          >
            <option value="">未分事项</option>
            {items.map((i) => (
              <option key={i.id} value={i.id}>
                {i.name}
              </option>
            ))}
          </select>
          {!readOnly && (
            <button disabled={running} onClick={() => setShowItemForm(!showItemForm)}>
              ＋ 新事项
            </button>
          )}
          <div className="work-context-tabs">
            <button onClick={() => showPanel("progress")}>进展</button>
            <button onClick={() => showPanel("documents")}>文档</button>
            <button onClick={() => showPanel("materials")}>资料</button>
          </div>
        </div>
        {showItemForm && (
          <div className="work-panel-actions work-inline-form">
            <input
              autoFocus
              aria-label="事项名称"
              value={newItem}
              onChange={(e) => setNewItem(e.target.value)}
              placeholder="例如：客户反馈"
            />
            <button
              disabled={!newItem.trim()}
              onClick={async () => {
                const i = await workService.createItem(topicId, newItem.trim());
                await refresh();
                setItemId(i.id);
                setNewItem("");
                setShowItemForm(false);
              }}
            >
              创建事项
            </button>
          </div>
        )}
        <div className="work-messages">
          {!messages.length && !running && (
            <div className="work-empty">
              <span className="work-eyebrow">
                {continueFrom ? "接续已有讨论" : "开始这一轮协作"}
              </span>
              <h2>这次想推进什么？</h2>
              <p>专题进展和最新文档会跟随每次讨论。你也可以把本次讨论关联到一个事项。</p>
            </div>
          )}
          {messages.length >= 30 && (
            <button
              onClick={async () => {
                const generation = epoch.current;
                const [older, history] = await Promise.all([
                  workService.messages(topicId, conversationId, messages.length),
                  workService.turns(topicId, conversationId, turns.length)
                ]);
                if (generation !== epoch.current) return;
                setMessages((current) => [
                  ...older.reverse(),
                  ...current.filter((m) => !older.some((o) => o.id === m.id))
                ]);
                setTurns((current) => [
                  ...current,
                  ...history.filter((t) => !current.some((c) => c.id === t.id))
                ]);
              }}
            >
              加载更早消息
            </button>
          )}
          {messages.map((m) => (
            <article
              className={`work-message ${m.role === "user" ? "user" : "assistant"}`}
              key={m.id}
            >
              <small>{m.role === "user" ? "你" : "AI"}</small>
              {m.role === "user" ? (
                <p>{m.content}</p>
              ) : (
                <MarkdownRenderer content={m.content} knowledgeCitation={knowledgeCitation} />
              )}
              {turns
                .filter((t) => t.userMessageId === m.id)
                .map((t) => (
                  <div className="work-turn-results" key={t.id}>
                    {t.outputs?.map((output, index) => (
                      <button
                        className="work-result-card"
                        key={index}
                        onClick={() => {
                          const suggested = proposals.find((p) => p.id === output.proposalId);
                          const target = suggested?.document?.artifactId || output.artifactId;
                          if (
                            (output.kind === "document" || output.kind === "document_suggestion") &&
                            target
                          ) {
                            setArtifactId(target);
                            showPanel("documents");
                          } else
                            showPanel(
                              output.kind === "document_suggestion" ? "documents" : "progress"
                            );
                        }}
                      >
                        <strong>
                          {output.kind === "document"
                            ? `${output.title || "文档"} · v${output.revision}`
                            : output.kind === "document_suggestion"
                              ? proposals.find((p) => p.id === output.proposalId)?.status ===
                                "applied"
                                ? "文档建议已应用"
                                : proposals.find((p) => p.id === output.proposalId)?.status ===
                                    "ignored"
                                  ? "文档建议已忽略"
                                  : "待确认的文档建议"
                              : proposals.find((p) => p.id === output.proposalId)?.status ===
                                  "applied"
                                ? "进展建议已确认"
                                : proposals.find((p) => p.id === output.proposalId)?.status ===
                                    "ignored"
                                  ? "进展建议已忽略"
                                  : "待确认的进展建议"}
                        </strong>
                        <span>
                          {output.summary ||
                            (output.kind === "document"
                              ? "本轮修改已保存，可以查看正文与版本。"
                              : output.kind === "document_suggestion"
                                ? "应用后才会保存文档修改。"
                                : "确认后才会更新专题进展。")}
                        </span>
                      </button>
                    ))}
                    {["failed", "cancelled", "interrupted"].includes(t.status) && (
                      <p className="work-muted">
                        {t.status === "cancelled" ? "本轮已停止" : "本轮未完成"}
                        。已保存的文档和建议仍可查看。
                      </p>
                    )}
                    {["failed", "cancelled", "interrupted"].includes(t.status) &&
                      t.action !== "discuss" &&
                      !t.outputs?.some((o) => o.kind === "document") && (
                        <button
                          onClick={async () => {
                            try {
                              const draft = await workService.documentDraft(topicId, t.id);
                              setReference({
                                title: `本轮未保存修改 · ${draft.title}`,
                                content: `${draft.baseRevision ? `基于 v${draft.baseRevision}` : "新文档尚未保存"}。${draft.summary}。请与最新正文核对后手动合并。`,
                                body: draft.body
                              });
                            } catch {
                              toast.message("本轮没有可预览的未保存文档修改。");
                            }
                          }}
                        >
                          查看本轮未保存修改
                        </button>
                      )}
                  </div>
                ))}
            </article>
          ))}
          {streamText && (
            <article className="work-message assistant">
              <small>AI</small>
              <MarkdownRenderer content={streamText} knowledgeCitation={knowledgeCitation} />
            </article>
          )}
          {streamStatus && (
            <p className="work-stream-status" role="status">
              {streamStatus}
            </p>
          )}
          <div ref={bottom} />
        </div>
        <div className="work-composer">
          {continueFrom && (
            <p className="work-context-tag">
              接续：{conversations.find((c) => c.id === continueFrom)?.title}
            </p>
          )}
          {artifactId && (
            <p className="work-context-tag">
              当前文档：{artifacts.find((a) => a.id === artifactId)?.title}{" "}
              <button
                disabled={running}
                onClick={() => {
                  setArtifactId("");
                  setAction("discuss");
                }}
              >
                移除聚焦
              </button>
            </p>
          )}
          {pendingSources.length > 0 && (
            <p className="work-context-tag">
              已附加 {pendingSources.length} 份资料，解析完成后可查阅。
            </p>
          )}
          <select
            aria-label="本轮工作方式"
            disabled={running || readOnly}
            value={autoIntent ? "auto" : action}
            onChange={(e) => {
              setAutoIntent(e.target.value === "auto");
              if (e.target.value !== "auto") setAction(e.target.value as WorkAction);
            }}
          >
            <option value="auto">根据本轮要求协作</option>
            <option value="discuss">讨论与建议</option>
            <option value="create_document">请 AI 新建文档</option>
            <option value="edit_document">交给 AI 局部修改</option>
            <option value="rewrite_document">交给 AI 整体重写</option>
          </select>
          <button
            className="work-attachment-button"
            disabled={running || readOnly}
            onClick={() => showPanel("materials")}
          >
            ＋ 附件与资料
          </button>
          <textarea
            aria-label="Work 消息"
            disabled={readOnly || running || dispatching}
            value={question}
            onChange={(e) => changeQuestion(e.target.value)}
            placeholder={
              readOnly
                ? "恢复专题后可以继续协作"
                : action === "edit_document"
                  ? "描述需要修改的部分，发送前会先保存你的草稿…"
                  : "描述这次想推进的工作…"
            }
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
                e.preventDefault();
                send();
              }
            }}
          />
          <div className="work-composer-footer">
            <small>
              {running
                ? "正在协作，文档暂时只读"
                : uploading
                  ? "资料上传中…"
                  : "Ctrl / ⌘ + Enter 发送"}
            </small>
            {!running && turnId && streamStatus && (
              <button
                onClick={() => {
                  setRunning(true);
                  runStream(turnId, epoch.current);
                }}
              >
                重连本轮
              </button>
            )}
            {running ? (
              <button
                onClick={async () => {
                  if (turnId) await workService.stop(topicId, turnId);
                }}
              >
                停止
              </button>
            ) : (
              <button
                className="primary"
                disabled={readOnly || uploading || dispatching || !question.trim()}
                onClick={send}
              >
                {dispatching ? "正在接续…" : "开始本轮"}
              </button>
            )}
          </div>
        </div>
      </section>
      <aside
        className={`work-topic-panel ${!panel ? "work-hidden" : ""} ${mobilePanel ? "mobile-visible" : ""}`}
      >
        <div className="work-panel-tabs">
          <button className="work-mobile-return" onClick={() => setMobilePanel(false)}>
            <ArrowLeft size={16} /> 返回对话
          </button>
          {[
            ["progress", "进展"],
            ["documents", "文档"],
            ["materials", "资料"]
          ].map(([value, label]) => (
            <button
              key={value}
              className={tab === value ? "selected" : ""}
              onClick={() => setTab(value)}
            >
              {label}
            </button>
          ))}
        </div>
        <div className={tab === "progress" ? "work-panel-content" : "work-hidden"}>
          <WorkProgressPanel
            topicId={topicId}
            state={state}
            proposals={proposals.filter((p) => !p.document)}
            items={items}
            itemId={itemId}
            readOnly={!!readOnly}
            refresh={refresh}
            openReference={openReference}
          />
          {proposals.length > 0 && proposals.length % 30 === 0 && (
            <button
              onClick={async () =>
                setProposals([
                  ...proposals,
                  ...(await workService.proposals(topicId, proposals.length))
                ])
              }
            >
              加载更早建议
            </button>
          )}
        </div>
        <div className={tab === "documents" ? "work-panel-content" : "work-hidden"}>
          <WorkDocumentPanel
            ref={doc}
            topicId={topicId}
            artifacts={artifacts}
            artifactId={artifactId}
            itemId={itemId}
            readOnly={!!readOnly}
            running={running}
            refreshKey={refreshKey}
            select={setArtifactId}
            refresh={refresh}
            handoff={() => {
              setAction("edit_document");
              setAutoIntent(false);
              setMobilePanel(false);
              toast.message("在聊天中描述需要修改的内容，发送时会先保存草稿。");
            }}
          />
          <WorkDocumentSuggestions
            topicId={topicId}
            proposals={proposals}
            readOnly={!!readOnly || running}
            refresh={refresh}
            select={setArtifactId}
            beforeApply={async (target) => {
              if (target && doc.current?.selected()?.artifact.id === target)
                await doc.current.save();
            }}
          />
          {proposals.length > 0 && proposals.length % 30 === 0 && (
            <button
              onClick={async () =>
                setProposals([
                  ...proposals,
                  ...(await workService.proposals(topicId, proposals.length))
                ])
              }
            >
              加载更早文档建议
            </button>
          )}
          {artifacts.length > 0 && artifacts.length % 30 === 0 && (
            <button
              onClick={async () =>
                setArtifacts([
                  ...artifacts,
                  ...(await workService.artifacts(topicId, artifacts.length))
                ])
              }
            >
              加载更多文档
            </button>
          )}
        </div>
        <div className={tab === "materials" ? "work-panel-content" : "work-hidden"}>
          <WorkMaterialsPanel
            topicId={topicId}
            conversationId={conversationId}
            readOnly={!!readOnly}
            uploading={setUploading}
            uploaded={(s) => {
              if (!s.promoted && !s.conversationId)
                setPendingSources((ids) => [...new Set([...ids, s.id])]);
            }}
          />
        </div>
      </aside>
      {reference && (
        <div className="work-modal-backdrop">
          <section className="work-modal" role="dialog" aria-modal="true">
            <h2>{reference.title}</h2>
            <pre className="work-reference-content">{reference.content}</pre>
            {reference.body && (
              <WorkDocumentEditor
                body={reference.body}
                editable={false}
                onChange={() => {}}
                editorKey={`reference:${reference.title}`}
                showToolbar={false}
              />
            )}
            <button onClick={() => setReference(undefined)}>关闭</button>
          </section>
        </div>
      )}
    </main>
  );
}
