import * as React from "react";
import { Database, Check } from "lucide-react";

import {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger
} from "@/components/ui/dropdown-menu";
import { cn } from "@/lib/utils";
import { getKnowledgeBases, type KnowledgeBase } from "@/services/knowledgeService";
import { useChatStore } from "@/stores/chatStore";

export function KnowledgeBasePicker() {
  const { knowledgeBaseIds, setKnowledgeBaseIds, isStreaming } = useChatStore();
  const [knowledgeBases, setKnowledgeBases] = React.useState<KnowledgeBase[]>([]);
  const [loaded, setLoaded] = React.useState(false);

  React.useEffect(() => {
    let cancelled = false;
    getKnowledgeBases()
      .then((list) => {
        if (cancelled) return;
        setKnowledgeBases(list);
      })
      .catch(() => {
        if (!cancelled) setKnowledgeBases([]);
      })
      .finally(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, []);

  const toggle = (id: string, checked: boolean) => {
    const next = checked
      ? [...knowledgeBaseIds, id]
      : knowledgeBaseIds.filter((existing) => existing !== id);
    setKnowledgeBaseIds(next);
  };

  const selectedCount = knowledgeBaseIds.length;
  const label =
    selectedCount === 0
      ? "选择知识库"
      : selectedCount === 1
        ? knowledgeBases.find((kb) => kb.id === knowledgeBaseIds[0])?.name ?? "1 个知识库"
        : `${selectedCount} 个知识库`;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={isStreaming}
          className={cn(
            "rounded-lg border px-3 py-1.5 text-xs font-medium transition-all",
            selectedCount > 0
              ? "border-[#BFDBFE] bg-[#DBEAFE] text-[#2563EB]"
              : "border-transparent bg-[#F5F5F5] text-[#999999] hover:bg-[#EEEEEE]",
            isStreaming && "cursor-not-allowed opacity-60"
          )}
          title="选择检索的知识库（可多选）"
        >
          <span className="inline-flex items-center gap-2">
            <Database className={cn("h-3.5 w-3.5", selectedCount > 0 && "text-[#3B82F6]")} />
            {label}
          </span>
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="start" className="max-h-72 w-64 overflow-y-auto">
        <DropdownMenuLabel>选择知识库</DropdownMenuLabel>
        <DropdownMenuSeparator />
        {loaded && knowledgeBases.length === 0 ? (
          <div className="px-2 py-3 text-center text-xs text-[#999999]">
            暂无知识库，请先到「知识库」页面创建
          </div>
        ) : null}
        {knowledgeBases.map((kb) => {
          const checked = knowledgeBaseIds.includes(kb.id);
          return (
            <DropdownMenuCheckboxItem
              key={kb.id}
              checked={checked}
              onCheckedChange={(next) => toggle(kb.id, Boolean(next))}
              onSelect={(event) => event.preventDefault()}
            >
              <span className="flex w-full items-center gap-2">
                <span className="flex-1 truncate">{kb.name}</span>
                {checked ? <Check className="h-3.5 w-3.5 shrink-0 text-[#2563EB]" /> : null}
              </span>
            </DropdownMenuCheckboxItem>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
