package runtime

import "strings"

func TruncateForLog(raw string) string {
	raw = strings.TrimSpace(raw)
	if len(raw) <= 300 {
		return raw
	}
	return raw[:300] + "..."
}
