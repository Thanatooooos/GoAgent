package fetch

import (
	"html"
	"regexp"
	"strings"
	"unicode"
)

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	headTagRe     = regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	commentTagRe  = regexp.MustCompile(`(?is)<!--.*?-->`)
	blockTagRe    = regexp.MustCompile(`(?is)</?(article|aside|blockquote|br|div|footer|h[1-6]|header|hr|li|main|nav|ol|p|section|table|tbody|td|th|thead|tr|ul)[^>]*>`)
	htmlTagRe     = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe  = regexp.MustCompile(`[ \t]+`)
	newlineRe     = regexp.MustCompile(`\n{3,}`)
)

var boilerplatePhrases = []string{
	"privacy policy",
	"terms of service",
	"all rights reserved",
	"cookie policy",
	"accept cookies",
	"sign in",
	"log in",
	"subscribe",
	"contact us",
	"skip to content",
	"back to top",
	"follow us",
	"newsletter",
	"menu",
	"home",
	"search",
	"read more",
	"learn more",
	"share this",
	"copyright",
	"cookie preferences",
	"manage preferences",
}

func extractText(rawHTML string) string {
	rawHTML = scriptStyleRe.ReplaceAllString(rawHTML, "")
	rawHTML = styleTagRe.ReplaceAllString(rawHTML, "")
	rawHTML = headTagRe.ReplaceAllString(rawHTML, "")
	rawHTML = commentTagRe.ReplaceAllString(rawHTML, "")
	rawHTML = normalizeNewlines(rawHTML)
	rawHTML = blockTagRe.ReplaceAllString(rawHTML, "\n")
	text := htmlTagRe.ReplaceAllString(rawHTML, " ")
	text = html.UnescapeString(text)
	text = whitespaceRe.ReplaceAllString(text, " ")
	text = normalizeNewlines(text)

	lines := strings.Split(text, "\n")
	trimmedLines := make([]string, 0, len(lines))
	seen := make(map[string]struct{}, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if isBoilerplateLine(line) || isLowSignalLine(line) {
			continue
		}
		key := normalizeLineKey(line)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		trimmedLines = append(trimmedLines, line)
	}

	result := strings.Join(trimmedLines, "\n\n")
	result = newlineRe.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}

func normalizeNewlines(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return text
}

func isBoilerplateLine(line string) bool {
	normalized := normalizeLineKey(line)
	for _, phrase := range boilerplatePhrases {
		if normalized == phrase || strings.Contains(normalized, phrase) {
			return true
		}
	}
	return false
}

func isLowSignalLine(line string) bool {
	if len(line) >= 20 {
		return false
	}

	letterCount := 0
	digitCount := 0
	for _, r := range line {
		switch {
		case unicode.IsLetter(r):
			letterCount++
		case unicode.IsDigit(r):
			digitCount++
		}
	}
	if letterCount >= 8 || digitCount >= 2 {
		return false
	}

	words := strings.Fields(line)
	if len(words) >= 4 {
		return false
	}
	return true
}

func normalizeLineKey(line string) string {
	line = strings.ToLower(strings.TrimSpace(line))
	line = whitespaceRe.ReplaceAllString(line, " ")
	line = strings.Trim(line, " -|:;,.!?()[]{}\"'")
	return line
}
