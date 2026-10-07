import * as React from "react";
import { Check, Sparkles } from "lucide-react";

import { cn } from "@/lib/utils";
import {
  collectTopicSections,
  isDomainComingSoon,
  isTopicNodeDisabled,
  topicBreadcrumbMap
} from "@/lib/dailyBriefTopicCatalog";
import type { DailyBriefTopicCatalogNode } from "@/types/dailyBrief";

type TopicCatalogPickerProps = {
  catalog: DailyBriefTopicCatalogNode[];
  selectedTopics: string[];
  onToggleTopic: (key: string, selected: boolean) => void;
};

export function TopicCatalogPicker({ catalog, selectedTopics, onToggleTopic }: TopicCatalogPickerProps) {
  const breadcrumbs = React.useMemo(() => topicBreadcrumbMap(catalog), [catalog]);
  const [expandedDomains, setExpandedDomains] = React.useState<Set<string>>(() => new Set());

  React.useEffect(() => {
    if (catalog.length === 0) return;
    setExpandedDomains((prev) => {
      const next = new Set(prev);
      for (const domain of catalog) {
        const leaves = domain.children.flatMap((child) =>
          child.selectable ? [child] : child.children.filter((node) => node.selectable)
        );
        if (leaves.some((leaf) => selectedTopics.includes(leaf.key))) {
          next.add(domain.key);
        }
      }
      return next;
    });
  }, [catalog, selectedTopics]);

  const toggleDomain = (domain: DailyBriefTopicCatalogNode) => {
    if (isDomainComingSoon(domain)) return;
    setExpandedDomains((prev) => {
      const next = new Set(prev);
      if (next.has(domain.key)) {
        next.delete(domain.key);
      } else {
        next.add(domain.key);
      }
      return next;
    });
  };

  const selectedLabels = selectedTopics
    .map((key) => breadcrumbs[key])
    .filter(Boolean);

  return (
    <div className="brief-topic-picker space-y-4">
      <div>
        <p className="text-sm font-semibold text-slate-900">选择你关心的领域</p>
        <p className="mt-1 text-xs text-slate-500">点击大卡片展开细分类别，可多选</p>
      </div>

      <div className="grid grid-cols-2 gap-2">
        {catalog.map((domain) => {
          const comingSoon = isDomainComingSoon(domain);
          const expanded = expandedDomains.has(domain.key);
          return (
            <button
              key={domain.key}
              type="button"
              disabled={comingSoon}
              onClick={() => toggleDomain(domain)}
              className={cn(
                "rounded-xl border p-3 text-left transition-colors",
                comingSoon
                  ? "cursor-not-allowed border-slate-200 bg-slate-50 text-slate-400"
                  : expanded
                    ? "border-blue-600 bg-blue-50 text-slate-900 shadow-sm"
                    : "border-slate-200 bg-white text-slate-900 hover:border-blue-300 hover:bg-slate-50"
              )}
            >
              <div className="flex items-start justify-between gap-1">
                <span className="text-sm font-semibold leading-tight">{domain.displayName}</span>
                {expanded && !comingSoon ? <Check className="h-3.5 w-3.5 shrink-0 text-blue-600" /> : null}
              </div>
              <p className="mt-1 line-clamp-2 text-[11px] leading-4 text-slate-500">{domain.description}</p>
              {comingSoon ? <span className="mt-2 inline-block text-[10px] text-slate-400">即将上线</span> : null}
            </button>
          );
        })}
      </div>

      {catalog
        .filter((domain) => expandedDomains.has(domain.key) && !isDomainComingSoon(domain))
        .map((domain) => (
          <div key={`panel-${domain.key}`} className="space-y-3 rounded-xl border border-slate-200 bg-slate-50/80 p-3">
            <p className="text-xs font-semibold text-slate-700">{domain.displayName} · 细分类别</p>
            {collectTopicSections(domain).map((section) => (
              <div key={`${domain.key}-${section.label || "default"}`} className="space-y-2">
                {section.label ? <p className="text-[11px] font-medium text-slate-500">{section.label}</p> : null}
                <div className="grid grid-cols-1 gap-2 sm:grid-cols-2">
                  {section.leaves.map((leaf) => {
                    const disabled = isTopicNodeDisabled(leaf);
                    const selected = selectedTopics.includes(leaf.key);
                    return (
                      <button
                        key={leaf.key}
                        type="button"
                        disabled={disabled}
                        title={disabled ? "即将上线" : undefined}
                        onClick={() => onToggleTopic(leaf.key, !selected)}
                        className={cn(
                          "rounded-lg border px-3 py-2 text-left text-sm transition-colors",
                          disabled
                            ? "cursor-not-allowed border-slate-200 bg-white text-slate-400"
                            : selected
                              ? "border-blue-600 bg-blue-600 text-white shadow-sm"
                              : "border-slate-200 bg-white text-slate-800 hover:border-blue-300 hover:bg-blue-50"
                        )}
                      >
                        <span className="font-medium">{leaf.displayName}</span>
                        {disabled ? (
                          <span className="mt-0.5 block text-[10px]">即将上线</span>
                        ) : leaf.description ? (
                          <span
                            className={cn(
                              "mt-0.5 block line-clamp-2 text-[10px] leading-4",
                              selected ? "text-blue-100" : "text-slate-500"
                            )}
                          >
                            {leaf.description}
                          </span>
                        ) : null}
                      </button>
                    );
                  })}
                </div>
              </div>
            ))}
          </div>
        ))}

      {selectedLabels.length > 0 ? (
        <div className="space-y-2 rounded-xl border border-dashed border-slate-200 bg-white p-3">
          <div className="flex items-center gap-1.5 text-xs font-medium text-slate-600">
            <Sparkles className="h-3.5 w-3.5 text-blue-600" />
            已选主题
          </div>
          <div className="flex flex-wrap gap-1.5">
            {selectedLabels.map((label) => (
              <span
                key={label}
                className="inline-flex rounded-full bg-blue-50 px-2.5 py-1 text-[11px] font-medium text-blue-800"
              >
                {label}
              </span>
            ))}
          </div>
        </div>
      ) : null}
    </div>
  );
}
