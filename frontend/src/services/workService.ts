import { api } from "@/services/api";
import { createStreamResponse, type StreamHandlers } from "@/hooks/useStreamResponse";
import { storage } from "@/utils/storage";

export interface WorkTopic {
  id: string;
  name: string;
  description: string;
  status: "active" | "archived";
  revision: number;
  updatedAt: string;
}
export interface WorkItem {
  id: string;
  name: string;
}
export interface WorkConversation {
  id: string;
  title: string;
  itemId?: string;
  continueFrom?: string;
}
export interface WorkNode {
  id?: string;
  type: string;
  text?: string;
  level?: number;
  marks?: string[];
  content?: WorkNode[];
}
export interface WorkDocument {
  schemaVersion: 1;
  root: WorkNode;
}
export interface WorkArtifact {
  id: string;
  title: string;
  revision: number;
  itemId?: string;
  updatedAt: string;
}
export interface WorkVersion {
  artifactId: string;
  revision: number;
  title: string;
  body: WorkDocument;
  author: string;
  summary: string;
  restoredFrom?: number;
  conversationId?: string;
  createdAt: string;
}
export interface WorkArtifactDetail {
  artifact: WorkArtifact;
  version: WorkVersion;
}
export interface WorkReference {
  kind: string;
  id: string;
  revision?: number;
}
export interface WorkEntry {
  id: string;
  kind: "goal" | "constraint" | "decision" | "question" | "next";
  text: string;
  itemId?: string;
  references?: WorkReference[];
}
export interface WorkState {
  revision: number;
  entries: WorkEntry[];
}
export interface WorkChange {
  kind: "add" | "update" | "remove";
  entry: WorkEntry;
}
export interface WorkProposal {
  id: string;
  turnId: string;
  baseRevision: number;
  status: string;
  changes: WorkChange[];
  appliedRevision?: number;
  document?: {
    kind: "artifact_create" | "artifact_update";
    artifactId?: string;
    itemId?: string;
    title: string;
    summary: string;
    body: WorkDocument;
  };
}
export interface WorkMessage {
  id: string;
  role: string;
  content: string;
  conversationId?: string;
}
export interface WorkTurn {
  id: string;
  conversationId: string;
  itemId?: string;
  artifactId?: string;
  status: string;
  action: string;
  error?: string;
  userMessageId: string;
  outputs: {
    kind: string;
    artifactId?: string;
    revision?: number;
    title?: string;
    summary?: string;
    proposalId?: string;
  }[];
}
export type WorkVersionSummary = Omit<WorkVersion, "body">;
export type WorkAction = "discuss" | "create_document" | "edit_document" | "rewrite_document";
export interface WorkChatInput {
  requestId: string;
  question: string;
  conversationId?: string;
  continueFrom?: string;
  itemId?: string;
  artifactId?: string;
  artifactRevision?: number;
  action: WorkAction;
  sourceIds?: string[];
}
export interface WorkSource {
  id: string;
  name: string;
  documentId?: string;
  knowledgeBaseId: string;
  sourceType: string;
  sourceLocation?: string;
  promoted: boolean;
  status: string;
  processingStatus?: string;
  available: boolean;
  chunkCount: number;
  error?: string;
  conversationId?: string;
}
export const workRequestId = (): string => crypto.randomUUID();
export const workEmptyDocument = (): WorkDocument => ({
  schemaVersion: 1,
  root: { id: "root", type: "doc", content: [{ id: workRequestId(), type: "paragraph" }] }
});
const root = "/work/topics";
const topicURL = (id: string) => `${root}/${encodeURIComponent(id)}`;
const get = <T>(url: string): Promise<T> => api.get(url) as unknown as Promise<T>;
const post = <T>(url: string, data: unknown): Promise<T> =>
  api.post(url, data) as unknown as Promise<T>;
const put = <T>(url: string, data: unknown): Promise<T> =>
  api.put(url, data) as unknown as Promise<T>;
export const workService = {
  topics: (status = "active", offset = 0) =>
    get<WorkTopic[]>(`${root}?status=${status}&limit=30&offset=${offset}`),
  createTopic: (name: string, description: string) =>
    post<WorkTopic>(root, { requestId: workRequestId(), name, description }),
  topic: (id: string) => get<WorkTopic>(topicURL(id)),
  updateTopic: (topic: WorkTopic, changes: Partial<WorkTopic>) =>
    put<WorkTopic>(topicURL(topic.id), {
      requestId: workRequestId(),
      expectedRevision: topic.revision,
      name: topic.name,
      description: topic.description,
      status: topic.status,
      ...changes
    }),
  items: (id: string, offset = 0) =>
    get<WorkItem[]>(`${topicURL(id)}/items?limit=100&offset=${offset}`),
  createItem: (id: string, name: string) =>
    post<WorkItem>(`${topicURL(id)}/items`, { requestId: workRequestId(), name }),
  conversations: (id: string, offset = 0) =>
    get<WorkConversation[]>(`${topicURL(id)}/conversations?limit=30&offset=${offset}`),
  messages: (id: string, conversation: string, offset = 0) =>
    get<WorkMessage[]>(
      `${topicURL(id)}/conversations/${conversation}/messages?limit=30&offset=${offset}`
    ),
  message: (id: string, message: string) => get<WorkMessage>(`${topicURL(id)}/messages/${message}`),
  state: (id: string) => get<WorkState>(`${topicURL(id)}/state`),
  saveState: (id: string, state: WorkState) =>
    put<WorkState>(`${topicURL(id)}/state`, {
      requestId: workRequestId(),
      expectedRevision: state.revision,
      entries: state.entries
    }),
  artifacts: (id: string, offset = 0) =>
    get<WorkArtifact[]>(`${topicURL(id)}/artifacts?limit=30&offset=${offset}`),
  artifact: (id: string, artifact: string) =>
    get<WorkArtifactDetail>(`${topicURL(id)}/artifacts/${artifact}`),
  createArtifact: (id: string, title: string, itemId: string) =>
    post<WorkArtifactDetail>(`${topicURL(id)}/artifacts`, {
      requestId: workRequestId(),
      title,
      itemId,
      body: workEmptyDocument(),
      summary: "新建文档"
    }),
  saveArtifact: (id: string, detail: WorkArtifactDetail, requestId = workRequestId()) =>
    put<WorkArtifactDetail>(`${topicURL(id)}/artifacts/${detail.artifact.id}`, {
      requestId,
      expectedRevision: detail.version.revision,
      title: detail.version.title,
      itemId: detail.artifact.itemId || "",
      body: detail.version.body,
      summary: "手动修改"
    }),
  versions: (id: string, artifact: string, offset = 0) =>
    get<WorkVersionSummary[]>(
      `${topicURL(id)}/artifacts/${artifact}/versions?limit=20&offset=${offset}`
    ),
  version: (id: string, artifact: string, revision: number) =>
    get<WorkVersion>(`${topicURL(id)}/artifacts/${artifact}/versions/${revision}`),
  restore: (id: string, artifact: string, current: number, revision: number) =>
    post<WorkArtifactDetail>(`${topicURL(id)}/artifacts/${artifact}/restore`, {
      requestId: workRequestId(),
      expectedRevision: current,
      revision
    }),
  proposals: (id: string, offset = 0) =>
    get<WorkProposal[]>(`${topicURL(id)}/proposals?limit=30&offset=${offset}`),
  resolve: (
    id: string,
    proposal: string,
    revision: number,
    ignore: boolean,
    changes?: WorkChange[]
  ) =>
    post<WorkProposal>(`${topicURL(id)}/proposals/${proposal}/resolve`, {
      requestId: workRequestId(),
      expectedRevision: revision,
      ignore,
      ...(changes ? { changes } : {})
    }),
  turn: (id: string, turn: string) => get<WorkTurn>(`${topicURL(id)}/turns/${turn}`),
  documentDraft: (id: string, turn: string) =>
    get<{ title: string; summary: string; baseRevision: number; body: WorkDocument }>(
      `${topicURL(id)}/turns/${turn}/document-draft`
    ),
  turns: (id: string, conversationId: string, offset = 0) =>
    get<WorkTurn[]>(
      `${topicURL(id)}/turns?conversationId=${conversationId}&limit=30&offset=${offset}`
    ),
  stop: (id: string, turn: string) => post(`${topicURL(id)}/turns/${turn}/stop`, {}),
  intent: (id: string, question: string, artifactId: string) =>
    post<{ action: WorkAction; reason: string }>(`${topicURL(id)}/intent`, {
      question,
      artifactId
    }),
  sources: (id: string, conversationId: string, offset = 0) =>
    get<WorkSource[]>(
      `${topicURL(id)}/sources?conversationId=${conversationId}&limit=30&offset=${offset}`
    ),
  upload: (
    id: string,
    conversationId: string,
    file: File,
    attachment: boolean,
    requestId = workRequestId()
  ) => {
    const form = new FormData();
    form.append("file", file);
    form.append("conversationId", attachment ? conversationId : "");
    form.append("attachment", String(attachment));
    form.append("requestId", requestId);
    return post<WorkSource>(`${topicURL(id)}/sources/upload`, form);
  },
  link: (
    id: string,
    name: string,
    sourceLocation: string,
    conversationId: string,
    attachment: boolean
  ) =>
    post<WorkSource>(`${topicURL(id)}/sources/link`, {
      requestId: workRequestId(),
      name,
      sourceLocation,
      conversationId: attachment ? conversationId : "",
      attachment
    }),
  shared: (id: string, name: string, knowledgeBaseId: string) =>
    post<WorkSource>(`${topicURL(id)}/sources/shared`, {
      requestId: workRequestId(),
      name,
      knowledgeBaseId
    }),
  knowledgeBases: (id: string) =>
    get<{ Items: { ID: string; Name: string }[] }>(`${topicURL(id)}/available-knowledge-bases`),
  changeSource: (id: string, source: string, promote: boolean) =>
    post<WorkSource>(`${topicURL(id)}/sources/${source}/${promote ? "promote" : "remove"}`, {
      requestId: workRequestId()
    }),
  retrySource: (id: string, source: string, conversationId: string) =>
    post(`${topicURL(id)}/sources/${source}/retry?conversationId=${conversationId}`, {}),
  sourceText: (id: string, source: string, conversationId: string) =>
    get<{ text: string }>(
      `${topicURL(id)}/sources/${source}/text?conversationId=${conversationId}`
    ),
  original: (id: string, source: string, conversationId: string) =>
    api.get<Blob, Blob>(
      `${topicURL(id)}/sources/${source}/original?conversationId=${conversationId}`,
      { responseType: "blob" }
    ),
  citation: (id: string, chunk: string, conversationId: string) =>
    get<{ title: string; content: string; sourceId?: string; image?: boolean }>(
      `${topicURL(id)}/citations/${encodeURIComponent(chunk)}?conversationId=${conversationId}`
    ),
  deleteConversation: (id: string) => api.delete(`/conversations/${id}`),
  citationImage: (id: string, chunk: string, conversationId: string) =>
    api.get<Blob, Blob>(
      `${topicURL(id)}/citations/${encodeURIComponent(chunk)}/original?conversationId=${conversationId}`,
      { responseType: "blob" }
    ),
  stream: (id: string, input: WorkChatInput | string, handlers: StreamHandlers, offset = 0) =>
    createStreamResponse(
      {
        url: `${import.meta.env.VITE_API_BASE_URL || ""}${topicURL(id)}${typeof input === "string" ? `/turns/${input}/stream?offset=${offset}` : "/chat"}`,
        method: typeof input === "string" ? "GET" : "POST",
        ...(typeof input === "string" ? {} : { body: JSON.stringify(input) }),
        headers: { Authorization: storage.getToken() || "" },
        retryCount: 0
      },
      handlers
    )
};
