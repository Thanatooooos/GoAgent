import { useEffect, useState } from "react";
import { toast } from "sonner";
import { storage } from "@/utils/storage";
import {
  workService,
  workRequestId,
  type WorkState,
  type WorkProposal,
  type WorkEntry,
  type WorkItem,
  type WorkReference
} from "@/services/workService";

export const workKinds: Record<WorkEntry["kind"], string> = {
  goal: "目标",
  constraint: "约束",
  decision: "已确认决定",
  question: "待解问题",
  next: "下一步"
};
export function WorkProgressPanel({
  topicId,
  state,
  proposals,
  items,
  itemId,
  readOnly,
  refresh,
  openReference
}: {
  topicId: string;
  state: WorkState;
  proposals: WorkProposal[];
  items: WorkItem[];
  itemId: string;
  readOnly: boolean;
  refresh: () => Promise<void>;
  openReference: (ref: WorkReference) => void;
}) {
  const [scope, setScope] = useState("all");
  const draftKey = `work-progress:${storage.getUser()?.userId || ""}:${topicId}`;
  const [savedDraft] = useState<WorkState | null>(() => {
    try {
      return JSON.parse(sessionStorage.getItem(draftKey) || "null");
    } catch {
      return null;
    }
  });
  const [editing, setEditing] = useState(Boolean(savedDraft));
  const [draft, setDraft] = useState(savedDraft || state);
  const [saving, setSaving] = useState(false);
  const proposalDraftKey = draftKey + ":proposals";
  const [proposalDrafts, setProposalDrafts] = useState<Record<string, WorkProposal["changes"]>>(
    () => {
      try {
        return JSON.parse(sessionStorage.getItem(proposalDraftKey) || "{}");
      } catch {
        return {};
      }
    }
  );
  useEffect(() => {
    sessionStorage.setItem(proposalDraftKey, JSON.stringify(proposalDrafts));
  }, [proposalDraftKey, proposalDrafts]);
  useEffect(() => {
    if (!editing) setDraft(state);
  }, [state, editing]);
  useEffect(() => {
    if (editing) sessionStorage.setItem(draftKey, JSON.stringify(draft));
  }, [draftKey, draft, editing]);
  const entries = (editing ? draft.entries : state.entries).filter(
    (e) => scope === "all" || (e.itemId || "") === itemId
  );
  const save = async () => {
    setSaving(true);
    try {
      await workService.saveState(topicId, draft);
      sessionStorage.removeItem(draftKey);
      setEditing(false);
      await refresh();
    } catch {
      await refresh();
      toast.error("进展未保存，输入已保留。若版本已变化，请查看最新进展后再合并。");
    } finally {
      setSaving(false);
    }
  };
  const resolve = async (proposal: WorkProposal, ignore: boolean) => {
    setSaving(true);
    try {
      await workService.resolve(
        topicId,
        proposal.id,
        state.revision,
        ignore,
        proposalDrafts[proposal.id]
      );
      setProposalDrafts((drafts) => {
        const next = { ...drafts };
        delete next[proposal.id];
        return next;
      });
      await refresh();
    } catch {
      toast.error("建议尚未确认。进展可能已变化，请重新查看并编辑建议。");
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="work-panel-body">
      <div className="work-panel-heading">
        <h2>专题进展</h2>
        <span>v{state.revision}</span>
      </div>
      <div className="work-panel-actions">
        <select aria-label="进展范围" value={scope} onChange={(e) => setScope(e.target.value)}>
          <option value="all">全部事项</option>
          <option value="current">{itemId ? "当前事项" : "未分事项"}</option>
        </select>
        {!readOnly && (
          <button
            onClick={() => {
              setDraft(state);
              setEditing(true);
            }}
          >
            编辑进展
          </button>
        )}
      </div>
      {editing && (
        <div className="work-notice">
          手动修改进展，保存后用于后续协作。
          <div className="work-panel-actions">
            <button className="primary" disabled={saving} onClick={save}>
              保存进展
            </button>
            <button
              disabled={saving}
              onClick={() => {
                sessionStorage.removeItem(draftKey);
                setEditing(false);
              }}
            >
              取消编辑
            </button>
          </div>
          {draft.revision !== state.revision && (
            <div className="work-conflict">
              <p>
                进展已更新到 v{state.revision}，你的草稿仍保留在 v{draft.revision}
                。请与最新内容核对后合并。
              </p>
              <details>
                <summary>查看最新进展</summary>
                {state.entries.map((e) => (
                  <p key={e.id}>
                    {workKinds[e.kind]}：{e.text}
                  </p>
                ))}
              </details>
              <button onClick={() => setDraft({ ...draft, revision: state.revision })}>
                已核对，使用我的合并草稿
              </button>
            </div>
          )}
        </div>
      )}
      {(Object.keys(workKinds) as WorkEntry["kind"][]).map((kind) => (
        <section className="work-progress-section" key={kind}>
          <h3>{workKinds[kind]}</h3>
          {entries
            .filter((e) => e.kind === kind)
            .map((entry) => (
              <div className="work-progress-entry" key={entry.id}>
                {editing ? (
                  <>
                    <textarea
                      aria-label={workKinds[kind]}
                      value={entry.text}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          entries: draft.entries.map((x) =>
                            x.id === entry.id ? { ...x, text: e.target.value } : x
                          )
                        })
                      }
                    />
                    <div className="work-panel-actions">
                      <select
                        aria-label="所属事项"
                        value={entry.itemId || ""}
                        onChange={(e) =>
                          setDraft({
                            ...draft,
                            entries: draft.entries.map((x) =>
                              x.id === entry.id ? { ...x, itemId: e.target.value } : x
                            )
                          })
                        }
                      >
                        <option value="">未分事项</option>
                        {items.map((x) => (
                          <option key={x.id} value={x.id}>
                            {x.name}
                          </option>
                        ))}
                      </select>
                      <button
                        onClick={() =>
                          setDraft({
                            ...draft,
                            entries: draft.entries.filter((x) => x.id !== entry.id)
                          })
                        }
                      >
                        移除
                      </button>
                    </div>
                  </>
                ) : (
                  <>
                    <p>{entry.text}</p>
                    {entry.itemId && (
                      <small>{items.find((x) => x.id === entry.itemId)?.name || "事项"}</small>
                    )}
                    {entry.references?.map((ref, i) => (
                      <button className="work-reference" key={i} onClick={() => openReference(ref)}>
                        查看依据 {i + 1}
                      </button>
                    ))}
                  </>
                )}
              </div>
            ))}
          {!entries.some((e) => e.kind === kind) && <p className="work-muted">尚未记录</p>}
          {editing && (
            <button
              className="work-text-button"
              onClick={() =>
                setDraft({
                  ...draft,
                  entries: [...draft.entries, { id: workRequestId(), kind, text: "", itemId }]
                })
              }
            >
              ＋ 添加{workKinds[kind]}
            </button>
          )}
        </section>
      ))}
      <section className="work-progress-section">
        <h3>待确认建议</h3>
        {!proposals.some((p) => p.status === "pending") && (
          <p className="work-muted">新的建议会出现在这里，确认后才更新进展。</p>
        )}
        {proposals
          .filter((p) => p.status === "pending")
          .map((p) => (
            <article className="work-proposal" key={p.id}>
              <small>根据本轮讨论 · 基于进展 v{p.baseRevision}</small>
              {(proposalDrafts[p.id] || p.changes).map((c, i) => (
                <div key={`${c.entry.id}-${i}`}>
                  <span>
                    {c.kind === "remove" ? "移除" : c.kind === "update" ? "更新" : "添加"} ·{" "}
                    {workKinds[c.entry.kind]}
                  </span>
                  <textarea
                    aria-label="建议内容"
                    disabled={readOnly || c.kind === "remove"}
                    value={c.entry.text}
                    onChange={(e) =>
                      setProposalDrafts({
                        ...proposalDrafts,
                        [p.id]: (proposalDrafts[p.id] || p.changes).map((x, index) =>
                          index === i ? { ...x, entry: { ...x.entry, text: e.target.value } } : x
                        )
                      })
                    }
                  />
                </div>
              ))}
              {p.baseRevision !== state.revision && (
                <p className="work-notice">
                  进展已更新到 v{state.revision}。请检查建议后点击“已复查”再确认。
                </p>
              )}
              <div className="work-panel-actions">
                {p.baseRevision !== state.revision && (
                  <button
                    onClick={() =>
                      setProposalDrafts({
                        ...proposalDrafts,
                        [p.id]: proposalDrafts[p.id] || structuredClone(p.changes)
                      })
                    }
                  >
                    已复查
                  </button>
                )}
                <button
                  className="primary"
                  disabled={
                    readOnly ||
                    saving ||
                    (p.baseRevision !== state.revision && !proposalDrafts[p.id])
                  }
                  onClick={() => resolve(p, false)}
                >
                  确认进展
                </button>
                <button disabled={readOnly || saving} onClick={() => resolve(p, true)}>
                  忽略
                </button>
              </div>
            </article>
          ))}
      </section>
    </div>
  );
}
