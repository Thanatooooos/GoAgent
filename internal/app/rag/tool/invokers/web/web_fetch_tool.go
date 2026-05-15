package builtin

import (
	"context"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	ragtool "local/rag-project/internal/app/rag/tool"
	ragcore "local/rag-project/internal/app/rag/tool/core"
)

const (
	webFetchTimeout      = 10 * time.Second
	webFetchMaxBytes     = 1 << 20 // 1MB response limit
	webFetchMaxTextBytes = 8192    // 8KB extracted text limit
	webFetchMaxURLs      = 3       // max URLs per call
)

var (
	scriptStyleRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleTagRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	headTagRe     = regexp.MustCompile(`(?is)<head[^>]*>.*?</head>`)
	htmlTagRe     = regexp.MustCompile(`<[^>]*>`)
	whitespaceRe  = regexp.MustCompile(`[ \t]+`)
	newlineRe     = regexp.MustCompile(`\n{3,}`)
)

// WebFetchTool fetches URLs and extracts their main text content.
// Supports fetching up to 3 URLs in a single call (fetched concurrently).
type WebFetchTool struct {
	client *http.Client
}

func NewWebFetchTool() *WebFetchTool {
	return &WebFetchTool{
		client: &http.Client{Timeout: webFetchTimeout},
	}
}

func (t *WebFetchTool) Definition() ragtool.Definition {
	return ragtool.Definition{
		Name:        "web_fetch",
		Description: "Fetch and extract readable text content from one or more web page URLs. Supports up to 3 URLs per call. Use after web_search to get full details from promising results.",
		ReadOnly:    true,
		Parameters: []ragtool.ParameterDefinition{
			{
				Name:        "urls",
				Type:        ragtool.ParamTypeArray,
				Description: "One or more URLs to fetch (max 3). Each URL must start with http:// or https://.",
				Required:    true,
			},
		},
	}
}

func (t *WebFetchTool) Invoke(_ context.Context, call ragtool.Call) (ragtool.Result, error) {
	urls := readStringSliceArg(call.Arguments, "urls")
	if len(urls) == 0 {
		// Fallback: try single "url" parameter for LLM-driven calls.
		if single := strings.TrimSpace(ragcore.ReadStringArg(call.Arguments, "url")); single != "" {
			urls = []string{single}
		}
	}
	if len(urls) == 0 {
		return ragtool.Result{Name: "web_fetch", Status: ragtool.CallStatusFailed, ErrorMessage: "urls is required"}, nil
	}
	if len(urls) > webFetchMaxURLs {
		urls = urls[:webFetchMaxURLs]
	}

	results := t.fetchMultiple(urls)
	return t.buildResult(urls, results), nil
}

type fetchItem struct {
	url         string
	text        string
	originalLen int
	truncated   bool
	err         error
}

func (t *WebFetchTool) fetchMultiple(urls []string) []fetchItem {
	if len(urls) == 1 {
		item := t.fetchOne(urls[0])
		return []fetchItem{item}
	}

	items := make([]fetchItem, len(urls))
	var wg sync.WaitGroup
	for i, u := range urls {
		wg.Add(1)
		go func(idx int, rawURL string) {
			defer wg.Done()
			items[idx] = t.fetchOne(rawURL)
		}(i, u)
	}
	wg.Wait()
	return items
}

func (t *WebFetchTool) fetchOne(rawURL string) fetchItem {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return fetchItem{url: rawURL, err: fmt.Errorf("empty url")}
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return fetchItem{url: rawURL, err: fmt.Errorf("invalid url scheme")}
	}

	text, err := t.fetchAndExtract(rawURL)
	if err != nil {
		return fetchItem{url: rawURL, err: err}
	}

	originalLen := len(text)
	truncated := originalLen > webFetchMaxTextBytes
	text = ragcore.TruncateText(text, webFetchMaxTextBytes)
	return fetchItem{
		url:         rawURL,
		text:        text,
		originalLen: originalLen,
		truncated:   truncated,
	}
}

func (t *WebFetchTool) buildResult(urls []string, items []fetchItem) ragtool.Result {
	pages := make([]map[string]any, 0, len(items))
	successCount := 0
	failCount := 0
	var combinedText strings.Builder

	for _, item := range items {
		page := map[string]any{"url": item.url}
		if item.err != nil {
			page["error"] = item.err.Error()
			page["text"] = ""
			failCount++
		} else {
			page["text"] = item.text
			page["originalLen"] = item.originalLen
			page["wasTruncated"] = item.truncated
			if item.text != "" {
				successCount++
			}
		}
		pages = append(pages, page)

		if item.text != "" {
			if combinedText.Len() > 0 {
				combinedText.WriteString("\n\n---\n\n")
			}
			combinedText.WriteString(fmt.Sprintf("[%s]\n%s", item.url, item.text))
		}
	}

	status := ragtool.CallStatusSuccess
	if failCount == len(items) {
		status = ragtool.CallStatusFailed
	}

	return ragtool.Result{
		Name:    "web_fetch",
		Status:  status,
		Summary: fmt.Sprintf("fetched %d urls: %d ok, %d failed", len(urls), successCount, failCount),
		Data: map[string]any{
			"pages":        pages,
			"urls":         urls,
			"combinedText": combinedText.String(),
			"successCount": successCount,
			"failCount":    failCount,
		},
	}
}

func (t *WebFetchTool) fetchAndExtract(rawURL string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "goagent/1.0")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")

	resp, err := t.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, webFetchMaxBytes))
	if err != nil {
		return "", err
	}

	return extractText(string(body)), nil
}

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

// readStringSliceArg reads a []string argument from call arguments.
func readStringSliceArg(arguments map[string]any, key string) []string {
	if len(arguments) == 0 {
		return nil
	}
	value, ok := arguments[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				if trimmed := strings.TrimSpace(s); trimmed != "" {
					items = append(items, trimmed)
				}
			}
		}
		return items
	case string:
		if trimmed := strings.TrimSpace(typed); trimmed != "" {
			return []string{trimmed}
		}
		return nil
	}
	return nil
}

// Ensure WebFetchTool implements Tool.
var _ ragtool.Tool = (*WebFetchTool)(nil)
