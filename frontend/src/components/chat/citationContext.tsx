import * as React from "react";

interface CitationNumberContextValue {
  numberFor: (chunkId: string) => number;
  total: number;
}

const CitationNumberContext = React.createContext<CitationNumberContextValue>({
  numberFor: () => 0,
  total: 0
});

function buildNumberMap(content: string): Map<string, number> {
  const map = new Map<string, number>();
  interface MatchEntry {
    index: number;
    key: string;
  }
  const entries: MatchEntry[] = [];
  const kbRe = /<kb\b[^>]*\bchunk_id\s*=\s*"([^"]+)"/gi;
  const webRe = /<web\b[^>]*\burl\s*=\s*"([^"]+)"/gi;
  let match: RegExpExecArray | null;
  while ((match = kbRe.exec(content)) !== null) {
    const key = (match[1] || "").trim();
    if (key) entries.push({ index: match.index, key });
  }
  while ((match = webRe.exec(content)) !== null) {
    const key = (match[1] || "").trim();
    if (key) entries.push({ index: match.index, key });
  }
  entries.sort((a, b) => a.index - b.index);
  let index = 0;
  for (const entry of entries) {
    if (map.has(entry.key)) continue;
    index += 1;
    map.set(entry.key, index);
  }
  return map;
}

// CitationNumberProvider 为单条消息内的引用角标派生稳定编号：编号来自消息内容中
// <kb chunk_id=.../> 的出现顺序。内容固定时结果确定，虚拟列表卸载/重挂载也不会漂移。
export function CitationNumberProvider({
  content,
  children
}: {
  content: string;
  children: React.ReactNode;
}) {
  const numberMap = React.useMemo(() => buildNumberMap(content), [content]);
  const value = React.useMemo<CitationNumberContextValue>(
    () => ({
      numberFor: (chunkId: string) => numberMap.get(chunkId) ?? 0,
      total: numberMap.size
    }),
    [numberMap]
  );
  return <CitationNumberContext.Provider value={value}>{children}</CitationNumberContext.Provider>;
}

export function useCitationNumber() {
  return React.useContext(CitationNumberContext);
}
