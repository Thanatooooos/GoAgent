export function convertWikiLinks(content: string, kbId: string): string {
  return content.replace(/\[\[([^\]|]+)(?:\|([^\]]*))?\]\]/g, (_match, slug, title) => {
    const s = String(slug ?? "").trim();
    const t = String(title || slug || "").trim();
    if (!s) {
      return _match;
    }
    return `[${t}](/admin/knowledge/${encodeURIComponent(kbId)}/wiki?slug=${encodeURIComponent(s)})`;
  });
}
