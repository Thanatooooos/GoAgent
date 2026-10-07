import * as React from "react";
import { FileText } from "lucide-react";
import * as Popover from "@radix-ui/react-popover";

import { useCitationNumber } from "@/components/chat/citationContext";
import { getChunkDetail, getImageEvidenceDetail, getImageEvidenceOriginal } from "@/services/chatService";
import type { ChunkDetail, ImageEvidenceDetail } from "@/types";

interface CitationChipProps {
  node?: any;
}

export function CitationChip({ node }: CitationChipProps) {
  const { numberFor } = useCitationNumber();

  const properties = node?.properties ?? {};
  const doc = String(properties.doc ?? "");
  const chunkId = String(properties.chunk_id ?? properties.chunkId ?? "");
  const kbId = String(properties.kb_id ?? properties.kbId ?? "");
	const isImage = String(properties.kind ?? "") === "image";
  const index = chunkId ? numberFor(chunkId) : 0;

  const [detail, setDetail] = React.useState<ChunkDetail | null>(null);
	const [imageDetail, setImageDetail] = React.useState<ImageEvidenceDetail | null>(null);
	const [imageURL, setImageURL] = React.useState<string | null>(null);
	const loadSequence = React.useRef(0);
  const [open, setOpen] = React.useState(false);
  const [loadFailed, setLoadFailed] = React.useState(false);
	React.useEffect(() => () => {
		if (imageURL) URL.revokeObjectURL(imageURL);
	}, [imageURL]);

  const handleOpenChange = async (next: boolean) => {
    const sequence = ++loadSequence.current;
    setOpen(next);
    if (!next && imageURL) {
		URL.revokeObjectURL(imageURL);
		setImageURL(null);
	}
    if (!next) setImageDetail(null);
    if (next && chunkId && !(isImage ? imageDetail && imageURL : detail)) {
      setLoadFailed(false);
      try {
		if (isImage) {
			const evidence = await getImageEvidenceDetail(chunkId);
			const blob = await getImageEvidenceOriginal(chunkId);
			if (sequence !== loadSequence.current) return;
			setImageDetail(evidence);
			setImageURL(URL.createObjectURL(blob));
		} else {
			const chunk = await getChunkDetail(chunkId);
			if (sequence !== loadSequence.current) return;
			setDetail(chunk);
		}
      } catch {
        if (sequence === loadSequence.current) {
			setImageDetail(null);
			setLoadFailed(true);
		}
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
          {isImage && imageDetail ? (
			<div className="mt-2 max-h-96 overflow-y-auto text-xs text-gray-700 dark:text-gray-300">
				{imageURL ? <a href={imageURL} target="_blank" rel="noreferrer"><img src={imageURL} alt="引用的原图" className="max-h-52 w-full object-contain" /></a> : <div>图片加载中</div>}
				<div className="mt-2">图片顺序：{imageDetail.ordinal + 1}{imageDetail.page ? ` · 第 ${imageDetail.page} 页` : ""}</div>
				<div className="mt-2 font-medium">OCR 原文</div>
				<div className="whitespace-pre-wrap">{imageDetail.ocrText || (imageDetail.ocrStatus === "no_text" ? "未识别到文字" : "暂无结果")}</div>
				<div className="mt-2 font-medium">AI 生成的图片描述</div>
				<div className="whitespace-pre-wrap">{imageDetail.captionText || (imageDetail.captionStatus === "no_content" ? "无可描述内容" : "暂无结果")}</div>
				{imageDetail.adjacentText ? <><div className="mt-2 font-medium">相邻正文</div><div className="whitespace-pre-wrap">{imageDetail.adjacentText}</div></> : null}
			</div>
		  ) : detail ? (
            <div className="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap rounded bg-gray-50 p-2 text-xs leading-5 text-gray-700 dark:bg-gray-900 dark:text-gray-300">
              {detail.content}
            </div>
          ) : loadFailed ? (
            <div className="mt-2 text-xs text-gray-400">{isImage ? "图片来源已删除、不可用或无权访问" : "加载失败，请重新点击查看"}</div>
          ) : (
            <div className="mt-2 text-xs text-gray-400">加载中…</div>
          )}
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
