package service

import (
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"local/rag-project/internal/app/dailybrief/domain"
)

var trackingQueryPrefix = []string{
	"utm_",
	"fbclid",
	"gclid",
}

func DeduplicateCandidates(candidates []domain.Candidate) []domain.Candidate {
	deduped := make([]domain.Candidate, 0, len(candidates))
	urlIndex := map[string]int{}
	externalIndex := map[string]int{}
	titleIndex := map[string]int{}

	for _, candidate := range candidates {
		normalized := normalizeCandidate(candidate)
		duplicateIdx := findDuplicateIndex(normalized, urlIndex, externalIndex, titleIndex)
		if duplicateIdx == -1 {
			deduped = append(deduped, normalized)
			registerDedupKeys(normalized, len(deduped)-1, urlIndex, externalIndex, titleIndex)
			continue
		}

		current := deduped[duplicateIdx]
		replacement := choosePreferredCandidate(current, normalized)
		deduped[duplicateIdx] = replacement
		registerDedupKeys(replacement, duplicateIdx, urlIndex, externalIndex, titleIndex)
	}
	return deduped
}

func NormalizeURL(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" {
		return ""
	}

	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Host = stripDefaultPort(parsed.Host, parsed.Scheme)
	parsed.Fragment = ""
	parsed.Path = normalizePath(parsed.Path)
	parsed.RawQuery = normalizeQuery(parsed.Query())
	return parsed.String()
}

func normalizeCandidate(candidate domain.Candidate) domain.Candidate {
	candidate.Title = strings.TrimSpace(candidate.Title)
	candidate.URL = NormalizeURL(candidate.URL)
	candidate.Source = strings.TrimSpace(candidate.Source)
	candidate.Topic = strings.TrimSpace(candidate.Topic)
	candidate.ExternalID = strings.TrimSpace(candidate.ExternalID)
	candidate.SummarySnippet = strings.Join(strings.Fields(strings.TrimSpace(candidate.SummarySnippet)), " ")
	if candidate.Metadata == nil {
		candidate.Metadata = map[string]string{}
	}
	return candidate
}

func findDuplicateIndex(candidate domain.Candidate, urlIndex, externalIndex, titleIndex map[string]int) int {
	externalKey := dedupExternalKey(candidate)
	if externalKey != "" {
		if idx, ok := externalIndex[externalKey]; ok {
			return idx
		}
	}

	urlKey := dedupURLKey(candidate)
	if urlKey != "" {
		if idx, ok := urlIndex[urlKey]; ok {
			return idx
		}
	}

	titleKey := dedupTitleKey(candidate)
	if titleKey != "" {
		if idx, ok := titleIndex[titleKey]; ok {
			return idx
		}
	}
	return -1
}

func registerDedupKeys(candidate domain.Candidate, idx int, urlIndex, externalIndex, titleIndex map[string]int) {
	if key := dedupExternalKey(candidate); key != "" {
		externalIndex[key] = idx
	}
	if key := dedupURLKey(candidate); key != "" {
		urlIndex[key] = idx
	}
	if key := dedupTitleKey(candidate); key != "" {
		titleIndex[key] = idx
	}
}

func dedupExternalKey(candidate domain.Candidate) string {
	if candidate.Source == "" || candidate.ExternalID == "" {
		return ""
	}
	return candidate.Source + ":" + strings.ToLower(candidate.ExternalID)
}

func dedupURLKey(candidate domain.Candidate) string {
	if candidate.URL == "" {
		return ""
	}
	return strings.ToLower(candidate.URL)
}

func dedupTitleKey(candidate domain.Candidate) string {
	if candidate.Title == "" {
		return ""
	}
	fingerprint := titleFingerprint(candidate.Title)
	// Conservative fallback: compare titles only within the same source and topic bucket.
	if candidate.Source == "" || candidate.Topic == "" || len(fingerprint) < 16 {
		return ""
	}
	return candidate.Source + ":" + candidate.Topic + ":" + fingerprint
}

func titleFingerprint(title string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(title) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func choosePreferredCandidate(left, right domain.Candidate) domain.Candidate {
	if right.PublishedAt.After(left.PublishedAt) {
		return right
	}
	if left.PublishedAt.After(right.PublishedAt) {
		return left
	}
	if len(right.SummarySnippet) > len(left.SummarySnippet) {
		return right
	}
	if len(left.SummarySnippet) > len(right.SummarySnippet) {
		return left
	}
	if len(right.Metadata) > len(left.Metadata) {
		return right
	}
	if len(left.Metadata) > len(right.Metadata) {
		return left
	}
	if right.URL < left.URL {
		return right
	}
	return left
}

func normalizePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "/"
	}
	if path != "/" && strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/")
	}
	return path
}

func normalizeQuery(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	filtered := make(url.Values, len(values))
	for key, list := range values {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		if lowerKey == "" || isTrackingQuery(lowerKey) {
			continue
		}
		cloned := append([]string(nil), list...)
		sort.Strings(cloned)
		filtered[lowerKey] = cloned
	}
	if len(filtered) == 0 {
		return ""
	}
	return filtered.Encode()
}

func isTrackingQuery(key string) bool {
	for _, prefix := range trackingQueryPrefix {
		if strings.HasPrefix(key, prefix) {
			return true
		}
	}
	return false
}

func stripDefaultPort(host, scheme string) string {
	name, port, err := net.SplitHostPort(host)
	if err != nil {
		return host
	}
	if (scheme == "http" && port == strconv.Itoa(80)) || (scheme == "https" && port == strconv.Itoa(443)) {
		return name
	}
	return host
}
