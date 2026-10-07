package wiki

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	wikiGenerationPromptTemplate = `你是知识库整理助手。请将下面文档整理为互相关联的 wiki 页面。

要求：
1. 只输出严格 JSON，禁止任何额外文本或 markdown 代码块。
2. 最多生成 %d 个页面，页面类型为 "%s"。
3. JSON 结构：
{"pages":[{"slug":"entity/xxx","title":"标题","type":"entity","summary":"一句话","content":"# 标题\nMarkdown 正文"}],"links":[{"from":"entity/xxx","to":"concept/yyy","anchor":"锚点文本"}]}
4. slug 必须低熵、可读、唯一（如 entity/go 或 concept/并发）。type 只允许 entity 或 concept。
5. links 的 from/to 必须引用本批 pages 的 slug。

文档标题：%s

## 文档内容
%s`
)
