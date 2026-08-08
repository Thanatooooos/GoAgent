import type { DailyBriefTopicCatalogNode } from "@/types/dailyBrief";

export function flattenTopicCatalog(nodes: DailyBriefTopicCatalogNode[]): DailyBriefTopicCatalogNode[] {
  const result: DailyBriefTopicCatalogNode[] = [];
  const walk = (node: DailyBriefTopicCatalogNode) => {
    result.push(node);
    node.children.forEach(walk);
  };
  nodes.forEach(walk);
  return result;
}

export function topicBreadcrumbMap(nodes: DailyBriefTopicCatalogNode[]): Record<string, string> {
  const result: Record<string, string> = {};
  const walk = (node: DailyBriefTopicCatalogNode, parents: string[]) => {
    const next = [...parents, node.displayName];
    result[node.key] = next.join(" · ");
    node.children.forEach((child) => walk(child, next));
  };
  nodes.forEach((node) => walk(node, []));
  return result;
}

export function collectSelectableLeaves(nodes: DailyBriefTopicCatalogNode[]): DailyBriefTopicCatalogNode[] {
  const result: DailyBriefTopicCatalogNode[] = [];
  const walk = (node: DailyBriefTopicCatalogNode) => {
    if (node.selectable) {
      result.push(node);
      return;
    }
    node.children.forEach(walk);
  };
  nodes.forEach(walk);
  return result;
}

export function isTopicNodeDisabled(node: DailyBriefTopicCatalogNode): boolean {
  return !node.selectable || !node.enabled || node.hasSources === false;
}

export type TopicCatalogSection = {
  label: string;
  leaves: DailyBriefTopicCatalogNode[];
};

/** Groups leaf topics under a domain for card rendering (supports L2/L3 nesting). */
export function collectTopicSections(domain: DailyBriefTopicCatalogNode): TopicCatalogSection[] {
  const sections: TopicCatalogSection[] = [];

  for (const child of domain.children) {
    if (child.selectable) {
      sections.push({ label: "", leaves: [child] });
      continue;
    }
    const leaves = collectSelectableLeaves([child]);
    if (leaves.length > 0) {
      sections.push({ label: child.displayName, leaves });
    }
  }

  return sections;
}

export function domainHasSelectableLeaves(domain: DailyBriefTopicCatalogNode): boolean {
  return collectSelectableLeaves([domain]).some((leaf) => !isTopicNodeDisabled(leaf));
}

export function isDomainComingSoon(domain: DailyBriefTopicCatalogNode): boolean {
  return !domain.enabled || !domainHasSelectableLeaves(domain);
}
