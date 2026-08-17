package chat

import (
	"regexp"
	"strconv"
	"strings"
)

var (
	inlineMarkdownHeadingRE = regexp.MustCompile(`([。！？；：:）)\]])(#{1,6})([^\s#])`)
	lineMarkdownHeadingRE   = regexp.MustCompile(`(?m)^(#{1,6})([^\s#])`)
	headingInlineBulletRE   = regexp.MustCompile(`(?m)^(#{1,6}\s+[^\n-]+)-\s+(\*\*)`)
	inlineMarkdownBulletRE  = regexp.MustCompile(`([。！？；：:）)\]])\s*(-\s+\*\*)`)
	headingBeforeListRE     = regexp.MustCompile(`(?m)^(#{1,6}\s+[^\n]+)\n(-\s+)`)
	emptyParenthesesRE      = regexp.MustCompile(`（\s*）`)
	danglingOpeningRE       = regexp.MustCompile(`（\s*$`)
	protectedMarkdownRE     = regexp.MustCompile("(?s)```.*?```|`[^`\\n]+`")
	protectedPlaceholderRE  = regexp.MustCompile("\\x00md-code-(\\d+)\\x00")
)

// normalizeAssistantMarkdown repairs common model formatting slips at the
// persistence boundary so stored answers and subsequent conversation turns
// are not polluted by concatenated headings or incomplete citation syntax.
func normalizeAssistantMarkdown(content string) string {
	content = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\r", "\n"))
	if content == "" {
		return ""
	}

	content, protected := protectMarkdownCode(content)
	content = inlineMarkdownHeadingRE.ReplaceAllString(content, "$1\n\n$2 $3")
	content = lineMarkdownHeadingRE.ReplaceAllString(content, "$1 $2")
	content = headingInlineBulletRE.ReplaceAllString(content, "$1\n- $2")
	content = inlineMarkdownBulletRE.ReplaceAllString(content, "$1\n$2")
	content = headingBeforeListRE.ReplaceAllString(content, "$1\n\n$2")
	content = emptyParenthesesRE.ReplaceAllString(content, "")
	content = danglingOpeningRE.ReplaceAllString(content, "")
	content = protectedPlaceholderRE.ReplaceAllStringFunc(content, func(placeholder string) string {
		match := protectedPlaceholderRE.FindStringSubmatch(placeholder)
		if len(match) != 2 {
			return placeholder
		}
		idx, err := strconv.Atoi(match[1])
		if err != nil || idx < 0 || idx >= len(protected) {
			return placeholder
		}
		return protected[idx]
	})
	return strings.TrimSpace(content)
}

func protectMarkdownCode(content string) (string, []string) {
	protected := make([]string, 0, 2)
	content = protectedMarkdownRE.ReplaceAllStringFunc(content, func(code string) string {
		idx := len(protected)
		protected = append(protected, code)
		return "\x00md-code-" + strconv.Itoa(idx) + "\x00"
	})
	return content, protected
}
