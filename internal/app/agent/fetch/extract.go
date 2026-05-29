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
	htmlTagRe     = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe  = regexp.MustCompile(`[ \t]+`)
	newlineRe     = regexp.MustCompile(`\n{3,}`)
)

func extractText(rawHTML string) string {
	rawHTML = scriptStyleRe.ReplaceAllString(rawHTML, "")
	rawHTML = styleTagRe.ReplaceAllString(rawHTML, "")
	rawHTML = headTagRe.ReplaceAllString(rawHTML, "")
	text := htmlTagRe.ReplaceAllString(rawHTML, " ")
	text = html.UnescapeString(text)
	text = whitespaceRe.ReplaceAllString(text, " ")

	lines := strings.Split(text, "\n")
	trimmedLines := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if len(line) < 20 {
			letterCount := 0
			for _, r := range line {
				if unicode.IsLetter(r) {
					letterCount++
				}
			}
			if letterCount < 5 {
				continue
			}
		}
		trimmedLines = append(trimmedLines, line)
	}

	result := strings.Join(trimmedLines, "\n")
	result = newlineRe.ReplaceAllString(result, "\n\n")
	return strings.TrimSpace(result)
}
