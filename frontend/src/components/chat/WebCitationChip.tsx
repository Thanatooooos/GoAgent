import * as React from "react";
import { Globe } from "lucide-react";

import { useCitationNumber } from "@/components/chat/citationContext";

interface WebCitationChipProps {
  node?: any;
}

export function WebCitationChip({ node }: WebCitationChipProps) {
  const { numberFor } = useCitationNumber();

  const properties = node?.properties ?? {};
  const url = String(properties.url ?? "");
  const title = String(properties.title ?? "");
  const index = url ? numberFor(url) : 0;

  if (index <= 0 || !url) {
    return null;
  }

  const safeUrl = /^https?:\/\//i.test(url) ? url : undefined;
  if (!safeUrl) {
    return null;
  }

  return (
    <a
      href={safeUrl}
      target="_blank"
      rel="noreferrer"
      title={title || url}
      aria-label={`${title || "外部来源"} 引用 ${index}`}
      className="mx-0.5 inline-flex items-center rounded bg-green-50 px-1.5 py-0.5 align-super text-[11px] font-semibold text-green-700 no-underline hover:bg-green-100 dark:bg-green-950 dark:text-green-300"
    >
      <Globe className="mr-0.5 h-3 w-3" />
      {index}
    </a>
  );
}
