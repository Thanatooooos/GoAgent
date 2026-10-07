import { useState } from "react";
import { toast } from "sonner";
import { workService, type WorkProposal } from "@/services/workService";
import { WorkDocumentEditor } from "./WorkDocumentEditor";

export function WorkDocumentSuggestions({
  topicId,
  proposals,
  readOnly,
  beforeApply,
  refresh,
  select
}: {
  topicId: string;
  proposals: WorkProposal[];
  readOnly: boolean;
  beforeApply: (artifactId?: string) => Promise<void>;
  refresh: () => Promise<void>;
  select: (artifactId: string) => void;
}) {
  const [preview, setPreview] = useState("");
  const [busy, setBusy] = useState(false);
  const resolve = async (proposal: WorkProposal, ignore: boolean) => {
    setBusy(true);
    try {
      if (!ignore) await beforeApply(proposal.document?.artifactId);
      const applied = await workService.resolve(
        topicId,
        proposal.id,
        proposal.baseRevision,
        ignore
      );
      await refresh();
      if (!ignore && applied.document?.artifactId) select(applied.document.artifactId);
    } catch {
      toast.error(
        "建议尚未应用，正文或基准版本可能已变化。草稿与建议已保留，请查看最新正文后重新提出修改。"
      );
    } finally {
      setBusy(false);
    }
  };
  return (
    <div className="work-panel-body">
      {proposals
        .filter((p) => p.document && p.status === "pending")
        .map((p) => (
          <section className="work-proposal" key={p.id}>
            <h3>待确认的文档建议 · {p.document!.title}</h3>
            <p>{p.document!.summary}</p>
            <small>
              {p.document!.kind === "artifact_create"
                ? "建议新建文档"
                : `基于 v${p.baseRevision} 的修改`}{" "}
              · 应用前不会改变正文
            </small>
            <div className="work-panel-actions">
              <button onClick={() => setPreview(preview === p.id ? "" : p.id)}>
                查看拟修改正文
              </button>
              <button disabled={readOnly || busy} onClick={() => resolve(p, false)}>
                应用文档建议
              </button>
              <button disabled={readOnly || busy} onClick={() => resolve(p, true)}>
                忽略文档建议
              </button>
            </div>
            {preview === p.id && (
              <WorkDocumentEditor
                body={p.document!.body}
                editable={false}
                onChange={() => {}}
                editorKey={`proposal:${p.id}`}
                showToolbar={false}
              />
            )}
          </section>
        ))}
    </div>
  );
}
