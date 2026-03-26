package claude

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/html"
)

const (
	maxCharsPerSource   = 50_000
	maxTotalSourceChars = 200_000
	fetchTimeout        = 15 * time.Second
	totalFetchTimeout   = 60 * time.Second
	cacheTTL            = 5 * time.Minute
)

// fetchedSource holds the result of fetching a single URL source.
type fetchedSource struct {
	label   string
	url     string
	content string
	err     error
}

// cacheEntry stores a cached fetch result with expiry.
type cacheEntry struct {
	content   string
	err       error
	fetchedAt time.Time
}

var (
	fetchCache   = make(map[string]cacheEntry)
	fetchCacheMu sync.Mutex
)

// SourceInput is the input for a single source to fetch.
type SourceInput struct {
	URL   string
	Label string
}

// FetchSources fetches all URLs concurrently and returns formatted context content.
func FetchSources(ctx context.Context, sources []SourceInput) string {
	if len(sources) == 0 {
		return ""
	}

	ctx, cancel := context.WithTimeout(ctx, totalFetchTimeout)
	defer cancel()

	results := make([]fetchedSource, len(sources))
	var wg sync.WaitGroup

	for i, src := range sources {
		wg.Add(1)
		go func(idx int, s SourceInput) {
			defer wg.Done()
			content, err := fetchURL(ctx, s.URL)
			results[idx] = fetchedSource{
				label:   s.Label,
				url:     s.URL,
				content: content,
				err:     err,
			}
		}(i, src)
	}
	wg.Wait()

	results = truncateSources(results)

	var parts []string
	for _, r := range results {
		label := r.label
		if label == "" {
			label = r.url
		}
		if r.err != nil {
			parts = append(parts, fmt.Sprintf("## Source: %s (%s) — FETCH FAILED: %s", label, r.url, r.err))
		} else {
			parts = append(parts, fmt.Sprintf("## Source: %s (%s)\n\n%s", label, r.url, r.content))
		}
	}
	return strings.Join(parts, "\n\n")
}

func fetchURL(ctx context.Context, url string) (string, error) {
	// Check cache
	fetchCacheMu.Lock()
	if entry, ok := fetchCache[url]; ok && time.Since(entry.fetchedAt) < cacheTTL {
		fetchCacheMu.Unlock()
		return entry.content, entry.err
	}
	fetchCacheMu.Unlock()

	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	req.Header.Set("User-Agent", "Botka/1.0 (context fetcher)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cacheResult(url, "", err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP %d", resp.StatusCode)
		cacheResult(url, "", err)
		return "", err
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, int64(maxCharsPerSource*2)))
	if err != nil {
		cacheResult(url, "", err)
		return "", err
	}

	ct := resp.Header.Get("Content-Type")
	var content string
	if strings.Contains(ct, "text/html") || strings.Contains(ct, "application/xhtml") {
		content = extractText(string(body))
	} else {
		content = string(body)
		if len(content) > maxCharsPerSource {
			content = content[:maxCharsPerSource] + "\n[truncated]"
		}
	}

	cacheResult(url, content, nil)
	return content, nil
}

func cacheResult(url, content string, err error) {
	fetchCacheMu.Lock()
	fetchCache[url] = cacheEntry{content: content, err: err, fetchedAt: time.Now()}
	fetchCacheMu.Unlock()
}

// skipTags is the set of HTML elements whose content should be stripped entirely.
var skipTags = map[string]bool{
	"script": true, "style": true, "nav": true,
	"footer": true, "header": true, "noscript": true,
}

// extractText parses HTML and extracts visible text content,
// stripping script, style, nav, footer, and header elements.
func extractText(raw string) string {
	doc, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		// Not valid HTML — return as plain text
		if len(raw) > maxCharsPerSource {
			return raw[:maxCharsPerSource] + "\n[truncated]"
		}
		return raw
	}

	var sb strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && skipTags[n.Data] {
			return
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				sb.WriteString(text)
				sb.WriteByte(' ')
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if sb.Len() > maxCharsPerSource {
				break
			}
			walk(c)
		}
	}
	walk(doc)

	result := strings.TrimSpace(sb.String())
	if len(result) > maxCharsPerSource {
		result = result[:maxCharsPerSource] + "\n[truncated]"
	}
	return result
}

// truncateSources ensures total content across all sources stays within budget.
func truncateSources(sources []fetchedSource) []fetchedSource {
	total := 0
	for _, s := range sources {
		total += len(s.content)
	}
	if total <= maxTotalSourceChars {
		return sources
	}

	// Distribute budget evenly across sources
	suffix := "\n[truncated]"
	budget := maxTotalSourceChars/len(sources) - len(suffix)
	result := make([]fetchedSource, len(sources))
	copy(result, sources)
	for i := range result {
		if len(result[i].content) > budget {
			result[i].content = result[i].content[:budget] + suffix
		}
	}
	return result
}
