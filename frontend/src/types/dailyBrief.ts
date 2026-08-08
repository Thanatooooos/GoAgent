export type DailyBriefPageState = "empty" | "generating" | "failed" | "ready";

export interface DailyBriefTopicCatalogNode {
  key: string;
  displayName: string;
  description: string;
  selectable: boolean;
  enabled: boolean;
  hasSources?: boolean;
  children: DailyBriefTopicCatalogNode[];
}

export interface DailyBriefTopicCatalogResponse {
  nodes: DailyBriefTopicCatalogNode[];
}

export interface DailyBriefIssueItem {
  title: string;
  summary: string;
  whyItMatters: string;
  url: string;
  source: string;
  topic: string;
}

export interface DailyBriefIssueSection {
  key: string;
  title: string;
  items: DailyBriefIssueItem[];
}

export interface DailyBriefIssue {
  headline: string;
  topSummary: string;
  sections: DailyBriefIssueSection[];
  items: DailyBriefIssueItem[];
}

export interface DailyBriefIssueResponse {
  pageState: DailyBriefPageState;
  issue?: DailyBriefIssue | null;
  briefDate: string;
  lastGeneratedAt?: string | null;
}

export interface DailyBriefSubscription {
  enabled: boolean;
  timezone: string;
  deliveryTimeLocal: string;
  topics: string[];
}

export interface UpdateDailyBriefSubscriptionInput {
  enabled: boolean;
  timezone: string;
  deliveryTimeLocal: string;
  topics: string[];
}

const sourceLabelByKey: Record<string, string> = {
  "hacker-news": "Hacker News",
  "github-trending": "GitHub 热榜",
  "arxiv-cs-ai": "arXiv cs.AI",
  "arxiv-cs-cl": "arXiv cs.CL",
  "arxiv-cs-lg": "arXiv cs.LG",
  "papers-with-code": "Hugging Face Papers",
  "openai-blog": "OpenAI 博客",
  "anthropic-blog": "Anthropic 博客",
  "google-deepmind-blog": "Google DeepMind 博客",
  "meta-ai-blog": "Meta AI 博客",
  "the-decoder": "The Decoder",
  "venturebeat-ai": "VentureBeat AI",
  "techcrunch-ai": "TechCrunch AI",
  "dezeen": "Dezeen",
  "hyperallergic": "Hyperallergic",
  "variety": "Variety",
  "petapixel": "PetaPixel",
  "archdaily": "ArchDaily",
  "colossal": "Colossal",
  "music-business-worldwide": "Music Business Worldwide",
  "mixmag": "Mixmag",
  "pitchfork": "Pitchfork",
  "slipped-disc": "Slipped Disc",
  "music-ally": "Music Ally",
  "billboard": "Billboard",
  "bbc-world": "BBC World",
  "scmp": "SCMP",
  "techcrunch-policy": "TechCrunch Policy",
  "ft-world-economy": "FT World Economy",
  "carbon-brief": "Carbon Brief",
  "bbc-politics": "BBC Politics"
};

export function dailyBriefSourceLabel(key: string): string {
  return sourceLabelByKey[key] ?? key;
}

export function dailyBriefTopicLabel(key: string, breadcrumbMap?: Record<string, string>): string {
  if (breadcrumbMap?.[key]) {
    return breadcrumbMap[key];
  }
  return key;
}
