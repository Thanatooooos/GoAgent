import * as React from "react";
import { FileText } from "lucide-react";
import * as Popover from "@radix-ui/react-popover";

import { useCitationNumber } from "@/components/chat/citationContext";
import { getChunkDetail } from "@/services/chatService";
import type { ChunkDetail } from "@/types";

interface CitationChipProps {
  node?: any;
}

export function CitationChip({ node }: CitationChipProps) {
  const { numberFor } = useCitationNumber();

  const properties = node?.properties ?? {};
  const doc = String(properties.doc ?? "");
  const chunkId = String(properties.chunk_id ?? properties.chunkId ?? "");
  const kbId = String(properties.kb_id ?? properties.kbId ?? "");
  const index = chunkId ? numberFor(chunkId) : 0;

  const [detail, setDetail] = React.useState<ChunkDetail | null>(null);
  const [open, setOpen] = React.useState(false);
  const [loadFailed, setLoadFailed] = React.useState(false);

  const handleOpenChange = async (next: boolean) => {
    setOpen(next);
    if (next && chunkId && !detail) {
      setLoadFailed(false);
      try {
        setDetail(await getChunkDetail(chunkId));
      } catch {
        setLoadFailed(true);
      }
    }
  };

  // 编号来自消息内容的确定性派生；缺失（数据不一致）时不渲染角标。
  if (index <= 0) {
    return null;
  }

  return (
    <Popover.Root open={open} onOpenChange={handleOpenChange}>
      <Popover.Trigger asChild>
        <button
          type="button"
          className="mx-0.5 inline-flex items-center rounded bg-blue-50 px-1.5 py-0.5 align-super text-[11px] font-semibold text-blue-700 hover:bg-blue-100 dark:bg-blue-950 dark:text-blue-300"
          title={doc || "来源"}
          aria-label={`${doc || "来源"} 引用 ${index}`}
        >
          <FileText className="mr-0.5 h-3 w-3" />
          {index}
        </button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content
          side="top"
          className="z-50 w-80 rounded-lg border border-gray-200 bg-white p-3 shadow-lg dark:border-gray-700 dark:bg-gray-800"
        >
          <div className="text-sm font-medium text-gray-900 dark:text-gray-100">
            {doc || (detail?.docId ? `文档 ${detail.docId}` : "来源文档")}
          </div>
          {kbId ? <div className="mt-1 text-xs text-gray-500">知识库：{kbId}</div> : null}
          {detail ? (
            <div className="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-xs leading-5 text-gray-700 dark:bg-gray-900 dark:text-gray-300">
              {detail.content}
            </div>
          ) : loadFailed ? (
            <div className="mt-2 text-xs text-gray-400">加载失败，请重新点击查看</div>
          ) : (
            <div className="mt-2 text-xs text-gray-400">加载中…</div>
          )}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
