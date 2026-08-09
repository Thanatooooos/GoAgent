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
  const re = /<kb\b[^>]*\bchunk_id\s*=\s*"([^"]+)"/gi;
  let match: RegExpExecArray | null;
  let index = 0;
  while ((match = re.exec(content)) !== null) {
    const chunkId = (match[1] || "").trim();
    if (!chunkId || map.has(chunkId)) {
      continue;
    }
    index += 1;
    map.set(chunkId, index);
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
