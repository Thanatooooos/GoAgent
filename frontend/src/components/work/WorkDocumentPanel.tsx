import { forwardRef, useEffect, useImperativeHandle, useRef, useState } from "react";
import { toast } from "sonner";
import { WorkDocumentEditor } from "@/components/work/WorkDocumentEditor";
import {
  workService,
  workRequestId,
  type WorkArtifact,
  type WorkArtifactDetail,
  type WorkVersion,
  type WorkVersionSummary
} from "@/services/workService";
import { storage } from "@/utils/storage";

export interface WorkDocumentHandle {
  save: () => Promise<WorkArtifactDetail | null>;
  selected: () => WorkArtifactDetail | null;
}
export const WorkDocumentPanel = forwardRef<
  WorkDocumentHandle,
  {
    topicId: string;
    artifacts: WorkArtifact[];
    artifactId: string;
    itemId: string;
    readOnly: boolean;
    running: boolean;
    refreshKey: number;
    select: (id: string) => void;
    refresh: () => Promise<void>;
    handoff: () => void;
  }
>(function WorkDocumentPanel(
  {
    topicId,
    artifacts,
    artifactId,
    itemId,
    readOnly,
    running,
    refreshKey,
    select,
    refresh,
    handoff
  },
  ref
) {
  const [detail, setDetail] = useState<WorkArtifactDetail | null>(null);
  const [dirty, setDirty] = useState(false);
  const [saving, setSaving] = useState(false);
  const [versions, setVersions] = useState<WorkVersionSummary[]>([]);
  const [history, setHistory] = useState(false);
  const [preview, setPreview] = useState<WorkVersion | null>(null);
  const [conflict, setConflict] = useState(false);
  const [newTitle, setNewTitle] = useState("");
  const draftRef = useRef(detail);
  const epoch = useRef(0);
  const retry = useRef<{ hash: string; id: string }>();
  const userId = storage.getUser()?.userId || "";
  const key = `work-draft:${userId}:${topicId}:${artifactId}`;
  useEffect(() => {
    const current = ++epoch.current;
    setPreview(null);
    setHistory(false);
    setConflict(false);
    setDetail(null);
    draftRef.current = null;
    setDirty(false);
    setSaving(false);
    if (!artifactId) return;
    workService
      .artifact(topicId, artifactId)
      .then((value) => {
        if (current !== epoch.current) return;
        let draft: WorkArtifactDetail | null = null;
        try {
          const raw = sessionStorage.getItem(key);
          if (raw) draft = JSON.parse(raw);
        } catch {
          /* The server version remains usable. */
        }
        const next = draft?.artifact.id === artifactId ? draft : value;
        setDetail(next);
        draftRef.current = next;
        setDirty(Boolean(draft));
        setConflict(Boolean(draft && draft.version.revision !== value.version.revision));
      })
      .catch(() => {
        if (current === epoch.current) toast.error("文档读取失败，可重新选择文档。");
      });
    return () => {
      ++epoch.current;
    };
  }, [topicId, artifactId, refreshKey, key]);
  const update = (next: WorkArtifactDetail) => {
    setDetail(next);
    draftRef.current = next;
    setDirty(true);
    try {
      sessionStorage.setItem(key, JSON.stringify(next));
    } catch {
      toast.error("本地草稿空间不足，请保存文档后再切换。");
    }
  };
  const save = async () => {
    const current = draftRef.current;
    if (!current) return null;
    if (!dirty) return current;
    if (readOnly || running) throw new Error("当前文档只读");
    const generation = epoch.current;
    const draftKey = key;
    setSaving(true);
    const hash = JSON.stringify(current);
    if (retry.current?.hash !== hash) retry.current = { hash, id: workRequestId() };
    try {
      const saved = await workService.saveArtifact(topicId, current, retry.current.id);
      sessionStorage.removeItem(draftKey);
      if (generation === epoch.current) {
        setDetail(saved);
        draftRef.current = saved;
        setDirty(false);
        setConflict(false);
      }
      await refresh();
      return saved;
    } catch (error) {
      if (generation === epoch.current) {
        setConflict(true);
        toast.error("保存未完成，草稿已保留。请查看最新版本后再合并。");
      }
      throw error;
    } finally {
      if (generation === epoch.current) setSaving(false);
    }
  };
  useImperativeHandle(ref, () => ({ save, selected: () => draftRef.current }));
  const create = async () => {
    if (!newTitle.trim()) return;
    try {
      const value = await workService.createArtifact(topicId, newTitle.trim(), itemId);
      setNewTitle("");
      await refresh();
      select(value.artifact.id);
    } catch {
      /* API presents the failure. */
    }
  };
  const showHistory = async () => {
    if (!detail) return;
    const value = await workService.versions(topicId, detail.artifact.id);
    setVersions(value);
    setHistory(true);
  };
  const restore = async (version: WorkVersionSummary) => {
    if (!detail || dirty) return;
    setSaving(true);
    try {
      const saved = await workService.restore(
        topicId,
        artifactId,
        detail.version.revision,
        version.revision
      );
      setDetail(saved);
      draftRef.current = saved;
      setPreview(null);
      setHistory(false);
      await refresh();
    } catch {
      setConflict(true);
    } finally {
      setSaving(false);
    }
  };
  return (
    <div className="work-panel-body work-document-panel">
      <div className="work-panel-heading">
        <h2>协作文档</h2>
        <span>{running ? "AI 正在工作" : readOnly ? "归档只读" : "轮到你编辑"}</span>
      </div>
      <select aria-label="选择文档" value={artifactId} onChange={(e) => select(e.target.value)}>
        <option value="">选择一份文档</option>
        {artifacts.map((a) => (
          <option value={a.id} key={a.id}>
            {a.title} · v{a.revision}
          </option>
        ))}
      </select>
      {!readOnly && (
        <div className="work-panel-actions">
          <input
            aria-label="新文档名称"
            placeholder="新文档名称"
            value={newTitle}
            onChange={(e) => setNewTitle(e.target.value)}
          />
          <button disabled={running || !newTitle.trim()} onClick={create}>
            新建
          </button>
        </div>
      )}
      {!detail && (
        <div className="work-empty">
          <h3>一起完成一份文档</h3>
          <p>
            先写草稿，或在聊天中选择“请 AI 新建文档”。保存后把本轮修改交给
            AI，完成后你可以继续编辑。
          </p>
        </div>
      )}
      {detail && (
        <>
          <input
            className="work-document-title"
            aria-label="文档标题"
            value={detail.version.title}
            disabled={readOnly || running || saving}
            onChange={(e) =>
              update({ ...detail, version: { ...detail.version, title: e.target.value } })
            }
          />
          <div className="work-panel-actions">
            <small>
              v{detail.version.revision} · {dirty ? "有未保存修改" : "已保存"} ·{" "}
              {detail.version.author === "ai" ? "AI 修改" : "你修改"}
            </small>
            <button
              disabled={saving || running || readOnly || !dirty}
              className="primary"
              onClick={() => save().catch(() => null)}
            >
              保存
            </button>
            <button disabled={saving || running || readOnly} onClick={handoff}>
              {dirty ? "保存并交给 AI" : "交给 AI 修改"}
            </button>
            <button onClick={showHistory}>版本记录</button>
          </div>
          {conflict && (
            <div className="work-notice">
              当前草稿仍保留。可先查看服务器版本，复制需要保留的内容，再载入最新版本继续合并。
              <div className="work-panel-actions">
                <button
                  onClick={async () => {
                    const latest = await workService.artifact(topicId, artifactId);
                    setPreview(latest.version);
                  }}
                >
                  查看最新版本
                </button>
                <button
                  onClick={async () => {
                    const latest = await workService.artifact(topicId, artifactId);
                    sessionStorage.removeItem(key);
                    setDetail(latest);
                    draftRef.current = latest;
                    setDirty(false);
                    setConflict(false);
                  }}
                >
                  放弃草稿并载入最新版本
                </button>
              </div>
            </div>
          )}
          {detail.version.summary && (
            <p className="work-change-summary">本次修改：{detail.version.summary}</p>
          )}
          <WorkDocumentEditor
            key={`${artifactId}:${detail.version.revision}`}
            editorKey={`${artifactId}:${detail.version.revision}`}
            body={detail.version.body}
            editable={!readOnly && !running && !saving}
            onChange={(body) => update({ ...detail, version: { ...detail.version, body } })}
          />
          {history && (
            <section className="work-version-list">
              <div className="work-panel-heading">
                <h3>版本记录</h3>
                <button onClick={() => setHistory(false)}>关闭</button>
              </div>
              {versions.map((v) => (
                <div key={v.revision}>
                  <button
                    onClick={async () =>
                      setPreview(await workService.version(topicId, artifactId, v.revision))
                    }
                  >
                    v{v.revision} · {v.author === "ai" ? "AI" : "你"} · {v.summary || "保存文档"}
                  </button>
                  {v.revision !== detail.version.revision && (
                    <button
                      disabled={dirty || running || readOnly || saving}
                      onClick={() => restore(v)}
                    >
                      恢复为新版本
                    </button>
                  )}
                </div>
              ))}
              {versions.length % 20 === 0 && versions.length > 0 && (
                <button
                  onClick={async () =>
                    setVersions([
                      ...versions,
                      ...(await workService.versions(topicId, artifactId, versions.length))
                    ])
                  }
                >
                  加载更早版本
                </button>
              )}
              {dirty && <p className="work-muted">请先保存草稿，再恢复历史版本。</p>}
            </section>
          )}
          {preview && (
            <section className="work-version-preview">
              <div className="work-panel-heading">
                <h3>
                  查看修改 · v{preview.revision} ↔ 当前 v{detail.version.revision}
                </h3>
                <button onClick={() => setPreview(null)}>关闭</button>
              </div>
              <div className="work-version-compare">
                <div>
                  <small>查看的版本</small>
                  <WorkDocumentEditor
                    body={preview.body}
                    editable={false}
                    onChange={() => {}}
                    editorKey={`version:${artifactId}:${preview.revision}`}
                    showToolbar={false}
                  />
                </div>
                <div>
                  <small>当前正文</small>
                  <WorkDocumentEditor
                    body={detail.version.body}
                    editable={false}
                    onChange={() => {}}
                    editorKey={`current:${artifactId}:${detail.version.revision}`}
                    showToolbar={false}
                  />
                </div>
              </div>
            </section>
          )}
        </>
      )}
    </div>
  );
});
