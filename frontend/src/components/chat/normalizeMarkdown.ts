const inlineHeadingPattern = /([。！？；：:)\]])(#{1,6})([^\s#])/g;
const lineHeadingPattern = /^(#{1,6})([^\s#])/gm;
const headingInlineBulletPattern = /^(#{1,6}\s+[^\n-]+)-\s+(\*\*)/gm;
const inlineBulletPattern = /([。！？；：:)\]])\s*(-\s+\*\*)/g;
const headingBeforeListPattern = /^(#{1,6}\s+[^\n]+)\n(-\s+)/gm;
const emptyParenthesesPattern = /（\s*）/g;
const danglingOpeningPattern = /（\s*$/;
const protectedMarkdownPattern = /```[\s\S]*?```|`[^`\n]+`/g;
const webTagPattern = /<web\b[^>]*>/gi;

export function normalizeAssistantMarkdown(content: string): string {
  if (!content) return "";

  const protectedParts: string[] = [];
  let normalized = content.replace(/\r\n?/g, "\n");
  normalized = normalized.replace(protectedMarkdownPattern, (code) => {
    const index = protectedParts.length;
    protectedParts.push(code);
    return `\u0000md-code-${index}\u0000`;
  });
  normalized = normalized.replace(inlineHeadingPattern, "$1\n\n$2 $3");
  normalized = normalized.replace(lineHeadingPattern, "$1 $2");
  normalized = normalized.replace(headingInlineBulletPattern, "$1\n- $2");
  normalized = normalized.replace(inlineBulletPattern, "$1\n$2");
  normalized = normalized.replace(headingBeforeListPattern, "$1\n\n$2");
  normalized = normalized.replace(emptyParenthesesPattern, "");
  normalized = normalized.replace(danglingOpeningPattern, "");
  normalized = normalized.replace(webTagPattern, (tag) =>
    /\burl=["']https?:\/\//i.test(tag) ? tag : ""
  );
  normalized = normalized.replace(/\u0000md-code-(\d+)\u0000/g, (_placeholder, index) => protectedParts[Number(index)] ?? _placeholder);
  return normalized.trim();
}
