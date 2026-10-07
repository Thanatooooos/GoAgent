import { useState, useEffect, useRef } from "react";
import { workService } from "@/services/workService";
export function WorkCitationChip({
  node,
  topicId,
  conversationId
}: {
  node?: { properties?: Record<string, unknown> };
  topicId: string;
  conversationId: string;
}) {
  const props = node?.properties || {};
  const chunk = String(props.chunk_id || props.chunkId || "");
  const title = String(props.doc || "资料依据");
  const [open, setOpen] = useState(false);
  const [content, setContent] = useState("");
  const [imageURL, setImageURL] = useState("");
  const sequence = useRef(0);
  useEffect(
    () => () => {
      if (imageURL) URL.revokeObjectURL(imageURL);
    },
    [imageURL]
  );
  useEffect(
    () => () => {
      ++sequence.current;
    },
    [topicId, conversationId, chunk]
  );
  if (!chunk) return null;
  return (
    <span className="work-citation">
      <button
        className="work-reference"
        onClick={async () => {
          const current = ++sequence.current;
          if (open) {
            setOpen(false);
            setImageURL("");
            return;
          }
          setOpen(true);
          setContent("载入中…");
          try {
            const citation = await workService.citation(topicId, chunk, conversationId);
            if (current !== sequence.current) return;
            setContent(citation.content);
            if (citation.image) {
              const blob = await workService.citationImage(topicId, chunk, conversationId);
              if (current === sequence.current) setImageURL(URL.createObjectURL(blob));
            }
          } catch {
            if (current === sequence.current) setContent("来源已删除、移除或当前无法访问。");
          }
        }}
      >
        ↗ {title}
      </button>
      {open && (
        <span className="work-citation-detail">
          {imageURL && <img src={imageURL} alt={title} style={{ maxWidth: "100%" }} />}
          {content}
        </span>
      )}
    </span>
  );
}
