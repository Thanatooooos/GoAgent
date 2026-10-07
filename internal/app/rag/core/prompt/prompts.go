package prompt

// Fixed model instructions and templates. Runtime data is supplied at the call sites.
const (
	memoryContextInstruction = "## Long-Term Memory\nUse these persistent user- or knowledge-base-specific memories when they are relevant to the current question. If the current user request explicitly conflicts with a recalled preference, follow the current user request.\n"

	sessionContextHeader = "## 会话上下文片段\n"

	knowledgeContextHeader = "## Knowledge Context\n"

	toolContextHeader = "## Tool Context\n"

	workflowPolicyHeader = "## Workflow Policy\n"

	answerGuidanceHeader = "## Answer Guidance\n"

	answerFormattingInstruction = "\n\nOutput formatting: use Markdown; put a blank line before and after headings; write every bullet or numbered item on its own line; use a space after heading markers such as `### `; do not output internal citation tags or empty citation parentheses."

	citationProtocolHeader = "## Citation Protocol\n"
)
