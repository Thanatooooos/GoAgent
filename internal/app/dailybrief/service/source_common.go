package service

import (
	"encoding/xml"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"local/rag-project/internal/app/dailybrief/domain"
)

var htmlTagRe = regexp.MustCompile("<[^>]+>")

type rssFeed struct {
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
}

type atomFeed struct {
	Entries []atomEntry `xml:"entry"`
}

type atomEntry struct {
	ID        string     `xml:"id"`
	Title     string     `xml:"title"`
	Updated   string     `xml:"updated"`
	Published string     `xml:"published"`
	Summary   string     `xml:"summary"`
	Links     []atomLink `xml:"link"`
}

type atomLink struct {
	Rel  string `xml:"rel,attr"`
	Href string `xml:"href,attr"`
}

func parseRSSCandidates(raw []byte, sourceKey string, topic string) ([]domain.Candidate, error) {
	var feed rssFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}

	candidates := make([]domain.Candidate, 0, len(feed.Channel.Items))
	for _, item := range feed.Channel.Items {
		publishedAt := parseTimeWithFallback(item.PubDate)
		candidates = append(candidates, domain.Candidate{
			Title:          strings.TrimSpace(item.Title),
			URL:            strings.TrimSpace(item.Link),
			Source:         sourceKey,
			Topic:          topic,
			PublishedAt:    publishedAt,
			ExternalID:     strings.TrimSpace(item.GUID),
			SummarySnippet: cleanSummary(item.Description),
			Metadata: map[string]string{
				"format": "rss",
			},
		})
	}
	return candidates, nil
}

func parseAtomCandidates(raw []byte, sourceKey string, topic string) ([]domain.Candidate, error) {
	var feed atomFeed
	if err := xml.Unmarshal(raw, &feed); err != nil {
		return nil, err
	}

	candidates := make([]domain.Candidate, 0, len(feed.Entries))
	for _, entry := range feed.Entries {
		link := ""
		for _, item := range entry.Links {
			if item.Rel == "alternate" && item.Href != "" {
				link = item.Href
				break
			}
			if link == "" && item.Href != "" {
				link = item.Href
			}
		}
		publishedAt := parseTimeWithFallback(firstNonEmpty(entry.Published, entry.Updated))
		candidates = append(candidates, domain.Candidate{
			Title:          strings.TrimSpace(entry.Title),
			URL:            strings.TrimSpace(link),
			Source:         sourceKey,
			Topic:          topic,
			PublishedAt:    publishedAt,
			ExternalID:     strings.TrimSpace(entry.ID),
			SummarySnippet: cleanSummary(entry.Summary),
			Metadata: map[string]string{
				"format": "atom",
			},
		})
	}
	return candidates, nil
}

func parseTimeWithFallback(raw string) time.Time {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}
	}

	layouts := []string{
		time.RFC3339,
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
	}

	for _, layout := range layouts {
		if value, err := time.Parse(layout, raw); err == nil {
			return value.UTC()
		}
	}
	return time.Time{}
}

func cleanSummary(raw string) string {
	raw = htmlTagRe.ReplaceAllString(raw, " ")
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func normalizeSourceURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if parsed.Scheme == "" {
		parsed.Scheme = "https"
	}
	return parsed.String()
}

var metaArticleRe = regexp.MustCompile(`(?s)<article[^>]*data-post-id="([^"]+)"[^>]*>.*?<a[^>]*class="post-link"[^>]*href="([^"]+)"[^>]*>(.*?)</a>.*?<time[^>]*datetime="([^"]+)".*?</time>.*?<p[^>]*class="summary"[^>]*>(.*?)</p>`)

func parseHTMLCandidates(raw []byte, spec domain.SourceFeedSpec) ([]domain.Candidate, error) {
	switch spec.HTMLParser {
	case "meta-ai":
		return parseMetaAIBlogCandidates(raw, spec.Key, spec.Topic)
	case "huggingface-papers":
		return parseHuggingFacePapersCandidates(raw, spec.Key, spec.Topic)
	default:
		return nil, fmt.Errorf("unsupported html parser %q", spec.HTMLParser)
	}
}

func parseMetaAIBlogCandidates(raw []byte, sourceKey string, topic string) ([]domain.Candidate, error) {
	matches := metaArticleRe.FindAllStringSubmatch(string(raw), -1)
	candidates := make([]domain.Candidate, 0, len(matches))
	for _, match := range matches {
		if len(match) < 6 {
			continue
		}
		candidates = append(candidates, domain.Candidate{
			Title:          strings.TrimSpace(cleanSummary(match[3])),
			URL:            normalizeSourceURL(match[2]),
			Source:         sourceKey,
			Topic:          topic,
			PublishedAt:    parseTimeWithFallback(match[4]),
			ExternalID:     strings.TrimSpace(match[1]),
			SummarySnippet: strings.TrimSpace(cleanSummary(match[5])),
			Metadata: map[string]string{
				"format": "html",
			},
		})
	}
	return candidates, nil
}

var hfPaperSectionRe = regexp.MustCompile(`(?is)<h3[^>]*>(.*?)</h3>[\s\S]*?href="([^"]*/papers/[^"]+)"`)

func parseHuggingFacePapersCandidates(raw []byte, sourceKey string, topic string) ([]domain.Candidate, error) {
	matches := hfPaperSectionRe.FindAllStringSubmatch(string(raw), -1)
	candidates := make([]domain.Candidate, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		title := strings.TrimSpace(cleanSummary(match[1]))
		urlRaw := strings.TrimSpace(match[2])
		if strings.HasPrefix(urlRaw, "/") {
			urlRaw = "https://huggingface.co" + urlRaw
		}
		url := normalizeSourceURL(urlRaw)
		if title == "" || url == "" {
			continue
		}
		externalID := strings.TrimPrefix(strings.TrimSuffix(url, "/"), "https://huggingface.co/papers/")
		candidates = append(candidates, domain.Candidate{
			Title:          title,
			URL:            url,
			Source:         sourceKey,
			Topic:          topic,
			ExternalID:     externalID,
			SummarySnippet: title,
			Metadata: map[string]string{
				"format": "html",
			},
		})
	}
	return candidates, nil
}
