package llmgen

import (
	"regexp"
	"strings"
)

type RewriteRule struct {
	Find    string
	Replace string
}

type RewriteOptions struct {
	SkipCodeBlocks bool
	SkipLinks      bool
	WordBoundary   bool
}

type RewriteStats struct {
	Rewritten int
	Skipped   int
}

type span struct{ start, end int }

// RewriteRefs rewrites occurrences of rule.Find in text, skipping occurrences
// inside forbidden spans (code blocks / inline code / existing links) and
// optionally protecting word boundaries. Rules are applied in a single pass
// (leftmost-earliest match wins), so produced text is never re-processed.
func RewriteRefs(text string, rules []RewriteRule, opts RewriteOptions) (string, RewriteStats) {
	var stats RewriteStats
	if len(rules) == 0 || text == "" {
		return text, stats
	}
	active := make([]RewriteRule, 0, len(rules))
	for _, rule := range rules {
		if rule.Find != "" {
			active = append(active, rule)
		}
	}
	if len(active) == 0 {
		return text, stats
	}
	forbidden := buildForbiddenSpans(text, opts)

	var b strings.Builder
	b.Grow(len(text))
	i := 0
	nextSearch := make([]int, len(active))
	for i < len(text) {
		bestRule, bestIdx, bestEnd := -1, -1, -1
		for ri, rule := range active {
			if nextSearch[ri] < i {
				nextSearch[ri] = i
			}
			searchFrom := nextSearch[ri]
			for searchFrom < len(text) {
				rel := strings.Index(text[searchFrom:], rule.Find)
				if rel < 0 {
					nextSearch[ri] = len(text)
					break
				}
				idx := searchFrom + rel
				if insideSpan(forbidden, idx, len(rule.Find)) {
					stats.Skipped++
					nextSearch[ri] = idx + 1
					searchFrom = idx + 1
					continue
				}
				if opts.WordBoundary && !wordBoundaryOK(text, idx, len(rule.Find)) {
					nextSearch[ri] = idx + 1
					searchFrom = idx + 1
					continue
				}
				if bestIdx < 0 || idx < bestIdx {
					bestRule, bestIdx, bestEnd = ri, idx, idx+len(rule.Find)
				}
				break
			}
		}
		if bestRule < 0 {
			b.WriteString(text[i:])
			break
		}
		b.WriteString(text[i:bestIdx])
		b.WriteString(active[bestRule].Replace)
		stats.Rewritten++
		i = bestEnd
	}
	return b.String(), stats
}

func buildForbiddenSpans(text string, opts RewriteOptions) []span {
	var spans []span
	if opts.SkipCodeBlocks {
		spans = append(spans, codeSpans(text)...)
	}
	if opts.SkipLinks {
		spans = append(spans, linkSpans(text)...)
	}
	return spans
}

func codeSpans(text string) []span {
	var spans []span
	// inline code: `...`
	for i := 0; i < len(text); {
		open := strings.IndexByte(text[i:], '`')
		if open < 0 {
			break
		}
		open += i
		close := strings.IndexByte(text[open+1:], '`')
		if close < 0 {
			break
		}
		close += open + 1
		spans = append(spans, span{open, close + 1})
		i = close + 1
	}
	// fenced code blocks: ``` ... ```
	lines := strings.Split(text, "\n")
	inFence := false
	fenceStart := 0
	offset := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			if !inFence {
				inFence = true
				fenceStart = offset
			} else {
				spans = append(spans, span{fenceStart, offset + len(line)})
				inFence = false
			}
		}
		offset += len(line) + 1
	}
	if inFence {
		spans = append(spans, span{fenceStart, len(text)})
	}
	return spans
}

func linkSpans(text string) []span {
	var spans []span
	for i := 0; i < len(text); {
		open := strings.IndexByte(text[i:], '[')
		if open < 0 {
			break
		}
		open += i
		// find "(...)" after the closing ]
		closeBracket := strings.IndexByte(text[open+1:], ']')
		if closeBracket < 0 {
			break
		}
		closeBracket += open + 1
		paren := closeBracket + 1
		if paren >= len(text) || text[paren] != '(' {
			i = closeBracket + 1
			continue
		}
		depth := 0
		end := -1
		for j := paren; j < len(text); j++ {
			switch text[j] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = j + 1
				}
			}
			if end > 0 {
				break
			}
		}
		if end > 0 {
			spans = append(spans, span{open, end})
			i = end
		} else {
			i = closeBracket + 1
		}
	}
	return spans
}

func insideSpan(spans []span, idx, length int) bool {
	for _, s := range spans {
		if idx < s.end && idx+length > s.start {
			return true
		}
	}
	return false
}

func wordBoundaryOK(text string, idx, length int) bool {
	if idx > 0 && isWordChar(text[idx-1]) {
		return false
	}
	end := idx + length
	if end < len(text) && isWordChar(text[end]) {
		return false
	}
	return true
}

func isWordChar(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// CleanDeadRefs removes references matching refPattern whose id is not kept by
// the keep predicate. It returns the cleaned text and the number of removed refs.
func CleanDeadRefs(text string, keep func(id string) bool, refPattern *regexp.Regexp) (string, int) {
	if text == "" || refPattern == nil {
		return text, 0
	}
	removed := 0
	out := refPattern.ReplaceAllStringFunc(text, func(match string) string {
		m := refPattern.FindStringSubmatch(match)
		if len(m) < 2 {
			return match
		}
		if keep != nil && keep(m[1]) {
			return match
		}
		removed++
		return ""
	})
	return out, removed
}
