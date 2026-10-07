import { useCallback, useEffect, useRef, useState } from "react";
import { toast } from "sonner";
import { workService, workRequestId, type WorkSource } from "@/services/workService";

export function WorkMaterialsPanel({
  topicId,
  conversationId,
  readOnly,
  uploaded,
  uploading
}: {
  topicId: string;
  conversationId: string;
  readOnly: boolean;
  uploaded: (source: WorkSource) => void;
  uploading: (active: boolean) => void;
}) {
  const [sources, setSources] = useState<WorkSource[]>([]);
  const [busy, setBusy] = useState(false);
  const [scope, setScope] = useState("topic");
  const [name, setName] = useState("");
  const [url, setURL] = useState("");
  const [linkForm, setLinkForm] = useState(false);
  const [bases, setBases] = useState<{ ID: string; Name: string }[]>([]);
  const [kb, setKB] = useState("");
  const [showBases, setShowBases] = useState(false);
  const [preview, setPreview] = useState<{ name: string; text: string }>();
  const sequence = useRef(0);
  const fileInput = useRef<HTMLInputElement>(null);
  const retry = useRef<{ file: File; requestId: string }>();
  const refresh = useCallback(async () => {
    const current = ++sequence.current;
    const value = await workService.sources(topicId, conversationId);
    if (current === sequence.current) {
      setSources(value);
      value.filter((s) => !s.promoted && !s.conversationId).forEach(uploaded);
    }
  }, [topicId, conversationId]);
  useEffect(() => {
    refresh().catch(() => null);
    return () => {
      ++sequence.current;
    };
  }, [refresh]);
  useEffect(() => {
    if (
      !sources.some(
        (s) =>
          ["reserved", "uploading"].includes(s.status) ||
          ["pending", "running"].includes(s.processingStatus || "")
      )
    )
      return;
    const interval = window.setInterval(() => refresh().catch(() => null), 4000);
    return () => clearInterval(interval);
  }, [sources, refresh]);
  const upload = async (file: File) => {
    if (file.size > 20 * 1024 * 1024) {
      toast.error("文件不能超过 20 MB");
      return;
    }
    setBusy(true);
    uploading(true);
    if (retry.current?.file !== file) retry.current = { file, requestId: workRequestId() };
    try {
      const s = await workService.upload(
        topicId,
        conversationId,
        file,
        scope === "conversation",
        retry.current.requestId
      );
      uploaded(s);
      retry.current = undefined;
      await refresh();
      toast.success("已上传，解析完成后可供 AI 查阅。");
    } catch {
      toast.error("上传未完成，可重试同一文件。");
    } finally {
      setBusy(false);
      uploading(false);
    }
  };
  const addLink = async () => {
    setBusy(true);
    uploading(true);
    try {
      const s = await workService.link(
        topicId,
        name.trim() || url,
        url,
        conversationId,
        scope === "conversation"
      );
      uploaded(s);
      await refresh();
      setURL("");
      setName("");
      setLinkForm(false);
    } finally {
      setBusy(false);
      uploading(false);
    }
  };
  const download = async (s: WorkSource) => {
    const blob = await workService.original(topicId, s.id, conversationId);
    const objectURL = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = objectURL;
    a.download = s.name;
    a.click();
    window.setTimeout(() => URL.revokeObjectURL(objectURL), 1000);
  };
  return (
    <div className="work-panel-body">
      <div className="work-panel-heading">
        <h2>专题资料</h2>
        <button onClick={refresh}>刷新</button>
      </div>
      <p className="work-muted">上传的 Word、PDF 等供查阅使用。共同编辑的正文在“文档”中。</p>
      {!readOnly && (
        <>
          <select
            className="work-material-scope"
            aria-label="上传资料范围"
            value={scope}
            disabled={busy}
            onChange={(e) => setScope(e.target.value)}
          >
            <option value="topic">加入专题 · 所有对话可用</option>
            <option value="conversation">本次对话附件</option>
          </select>
          <div className="work-panel-actions">
            <input
              hidden
              type="file"
              ref={fileInput}
              onChange={(e) => {
                const f = e.target.files?.[0];
                if (f) upload(f);
                e.target.value = "";
              }}
            />
            <button className="primary" disabled={busy} onClick={() => fileInput.current?.click()}>
              {busy ? "上传中…" : "上传资料"}
            </button>
            <button disabled={busy} onClick={() => setLinkForm(!linkForm)}>
              添加网页
            </button>
            <button
              onClick={async () => {
                setBases((await workService.knowledgeBases(topicId)).Items || []);
                setShowBases(!showBases);
              }}
            >
              关联知识库
            </button>
          </div>
          {retry.current && !busy && (
            <button onClick={() => upload(retry.current!.file)}>
              重试：{retry.current.file.name}
            </button>
          )}
          {linkForm && (
            <div className="work-material-form">
              <input
                aria-label="网页名称"
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="网页名称（可选）"
              />
              <input
                aria-label="网页链接"
                value={url}
                onChange={(e) => setURL(e.target.value)}
                placeholder="https://…"
              />
              <button className="primary" disabled={busy || !url.trim()} onClick={addLink}>
                添加并解析
              </button>
            </div>
          )}
          {showBases && (
            <div className="work-material-form">
              <select aria-label="关联知识库" value={kb} onChange={(e) => setKB(e.target.value)}>
                <option value="">选择已有共享知识库</option>
                {bases.map((b) => (
                  <option key={b.ID} value={b.ID}>
                    {b.Name}
                  </option>
                ))}
              </select>
              <button
                disabled={!kb}
                onClick={async () => {
                  const b = bases.find((x) => x.ID === kb)!;
                  await workService.shared(topicId, b.Name, b.ID);
                  await refresh();
                  setShowBases(false);
                }}
              >
                关联到专题
              </button>
            </div>
          )}
        </>
      )}
      {!sources.length && (
        <div className="work-empty">
          <h3>保留工作的依据</h3>
          <p>资料上传并解析完成后，AI 可以在当前范围内查阅和引用。</p>
        </div>
      )}
      {sources.map((s) => (
        <article className="work-material-card" key={s.id}>
          <h3>{s.name}</h3>
          <small>
            {s.promoted ? "专题资料" : s.conversationId ? "本次对话附件" : "待发送的附件"} ·{" "}
            {s.sourceType === "shared"
              ? "共享知识库"
              : s.available
                ? `可查阅 · ${s.chunkCount} 段内容`
                : s.processingStatus === "failed" || s.status === "failed"
                  ? "处理失败"
                  : s.processingStatus === "running" || s.status === "uploading"
                    ? "正在处理"
                    : "待处理"}
          </small>
          {s.error && <p className="work-muted">{s.error}</p>}
          {s.sourceType === "url" && s.sourceLocation && (
            <p>
              <a href={s.sourceLocation} target="_blank" rel="noopener noreferrer">
                打开原网页 ↗
              </a>
            </p>
          )}
          <div className="work-panel-actions">
            {s.available && s.sourceType !== "shared" && (
              <button
                onClick={async () =>
                  setPreview({
                    name: s.name,
                    text: (await workService.sourceText(topicId, s.id, conversationId)).text
                  })
                }
              >
                查看解析内容
              </button>
            )}
            {s.sourceType === "file" && s.documentId && (
              <button onClick={() => download(s)}>下载原文件</button>
            )}
            {!readOnly && (
              <>
                {!s.promoted && (
                  <button
                    onClick={async () => {
                      await workService.changeSource(topicId, s.id, true);
                      await refresh();
                    }}
                  >
                    加入专题
                  </button>
                )}
                {s.sourceType !== "shared" &&
                  !s.available &&
                  s.documentId &&
                  s.processingStatus !== "running" && (
                    <button
                      onClick={async () => {
                        await workService.retrySource(topicId, s.id, conversationId);
                        await refresh();
                      }}
                    >
                      重新解析
                    </button>
                  )}
                <button
                  onClick={async () => {
                    await workService.changeSource(topicId, s.id, false);
                    await refresh();
                  }}
                >
                  移除资料
                </button>
              </>
            )}
          </div>
        </article>
      ))}
      {sources.length > 0 && sources.length % 30 === 0 && (
        <button
          onClick={async () =>
            setSources([
              ...sources,
              ...(await workService.sources(topicId, conversationId, sources.length))
            ])
          }
        >
          更多资料
        </button>
      )}
      {preview && (
        <section className="work-version-preview">
          <div className="work-panel-heading">
            <h3>{preview.name}</h3>
            <button onClick={() => setPreview(undefined)}>关闭</button>
          </div>
          <pre className="work-reference-content">{preview.text || "暂无可展示内容"}</pre>
        </section>
      )}
    </div>
  );
}
