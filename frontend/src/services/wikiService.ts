import { api } from "@/services/api";

export interface WikiPageItem {
  id: string;
  kbId: string;
  slug: string;
  title: string;
  pageType: string;
  status: string;
  summary: string;
  content?: string;
  inLinks?: number;
  outLinks?: number;
}

export interface WikiGraphData {
  nodes: { id: string; slug: string; title: string; inLinks: number; outLinks: number }[];
  edges: { from: string; to: string; anchor: string }[];
}

export const listWikiPages = async (kbId: string, current = 1, size = 100): Promise<WikiPageItem[]> => {
  const page = await api.get<{ records: WikiPageItem[] }, { records: WikiPageItem[] }>(
    `/knowledge-base/${kbId}/wiki/pages?current=${current}&size=${size}`
  );
  return page.records || [];
};

export const getWikiPage = async (kbId: string, slug: string): Promise<WikiPageItem> => {
  return api.get<WikiPageItem, WikiPageItem>(`/knowledge-base/${kbId}/wiki/pages/${encodeURIComponent(slug)}`);
};

export const getWikiGraph = async (kbId: string): Promise<WikiGraphData> => {
  return api.get<WikiGraphData, WikiGraphData>(`/knowledge-base/${kbId}/wiki/graph`);
};
