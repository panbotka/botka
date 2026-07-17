package handlers

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/net/html"
)

const (
	// bookmarkFetchTimeout bounds the metadata fetch for a new bookmark so a slow
	// or unresponsive page never blocks the create request.
	bookmarkFetchTimeout = 8 * time.Second
	// bookmarkMaxBytes caps how much of the page body we read while looking for
	// the <title> and favicon link — both live in <head>, near the top.
	bookmarkMaxBytes = 512 * 1024
	// bookmarkUserAgent identifies Botka to fetched sites.
	bookmarkUserAgent = "Botka/1.0 (bookmark metadata fetcher)"
	// maxBookmarkTitleLen caps the stored/derived page title length.
	maxBookmarkTitleLen = 500
)

// normalizeBookmarkURL trims the input and ensures it carries an http(s) scheme
// so the stored URL both fetches server-side and opens as an absolute link in a
// new browser tab. A bare host like "example.com" becomes "https://example.com".
// Returns "" when the input is empty.
func normalizeBookmarkURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	// A protocol-relative "//host/path" or a scheme-less "host/path" both need a
	// scheme prepended; anything already carrying "scheme://" is left as-is.
	if strings.HasPrefix(raw, "//") {
		return "https:" + raw
	}
	if !strings.Contains(raw, "://") {
		return "https://" + raw
	}
	return raw
}

// fetchBookmarkMeta fetches rawURL and derives a display title and favicon URL.
// It is time-bounded and never returns an error: on any failure (bad URL,
// timeout, non-200, unreadable body) it falls back to the URL's hostname as the
// title and the site's default /favicon.ico (or a favicon service when no host
// can be determined). rawURL is expected to already be normalized (see
// normalizeBookmarkURL).
func fetchBookmarkMeta(ctx context.Context, rawURL string) (title, faviconURL string) {
	base, parseErr := url.Parse(rawURL)
	if parseErr != nil {
		base = nil
	}

	// Defaults derived purely from the URL, used when the fetch fails or the page
	// declares no title/icon of its own.
	title = rawURL
	if base != nil && base.Hostname() != "" {
		title = base.Hostname()
	}
	faviconURL = defaultFavicon(base, rawURL)

	ctx, cancel := context.WithTimeout(ctx, bookmarkFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return truncateTitle(title), faviconURL
	}
	req.Header.Set("User-Agent", bookmarkUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return truncateTitle(title), faviconURL
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return truncateTitle(title), faviconURL
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, bookmarkMaxBytes))
	if err != nil {
		return truncateTitle(title), faviconURL
	}

	pageTitle, iconHref := parseBookmarkMeta(string(body))
	if pageTitle != "" {
		title = pageTitle
	}
	if iconHref != "" {
		if resolved := resolveBookmarkURL(base, iconHref); resolved != "" {
			faviconURL = resolved
		}
	}
	return truncateTitle(title), faviconURL
}

// defaultFavicon returns a best-effort favicon URL for base's origin: the
// conventional /favicon.ico when the host is known, otherwise a favicon-service
// URL keyed on the raw input. Returns "" only when nothing usable is available;
// the frontend renders a generic globe icon in that case.
func defaultFavicon(base *url.URL, rawURL string) string {
	if base != nil && base.Host != "" {
		scheme := base.Scheme
		if scheme == "" {
			scheme = "https"
		}
		return scheme + "://" + base.Host + "/favicon.ico"
	}
	if d := strings.TrimSpace(rawURL); d != "" {
		return "https://www.google.com/s2/favicons?domain=" + url.QueryEscape(d) + "&sz=64"
	}
	return ""
}

// resolveBookmarkURL resolves a possibly-relative href (from an HTML <link>)
// against the page's base URL, returning an absolute URL string. Returns "" when
// the href is unparseable or cannot be made absolute.
func resolveBookmarkURL(base *url.URL, href string) string {
	href = strings.TrimSpace(href)
	if href == "" {
		return ""
	}
	ref, err := url.Parse(href)
	if err != nil {
		return ""
	}
	if base == nil {
		if ref.IsAbs() {
			return ref.String()
		}
		return ""
	}
	return base.ResolveReference(ref).String()
}

// parseBookmarkMeta parses HTML and returns the document <title> text and the
// best favicon href it declares (as written — may be relative). It prefers a
// <link rel="icon"> / rel="shortcut icon" over rel="apple-touch-icon". Either
// return value may be "" when the element is absent.
func parseBookmarkMeta(body string) (title, iconHref string) {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return "", ""
	}

	var appleIcon string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.Data {
			case "title":
				if title == "" {
					if t := textContent(n); t != "" {
						title = t
					}
				}
			case "link":
				rel, href := linkRelHref(n)
				switch {
				case href == "":
					// nothing to record
				case relHasIcon(rel):
					if iconHref == "" {
						iconHref = href
					}
				case strings.Contains(rel, "apple-touch-icon"):
					if appleIcon == "" {
						appleIcon = href
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)

	if iconHref == "" {
		iconHref = appleIcon
	}
	return strings.TrimSpace(title), iconHref
}

// textContent returns the concatenated, trimmed text of a node's direct text
// children.
func textContent(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return strings.TrimSpace(sb.String())
}

// linkRelHref extracts the lower-cased rel and the raw href of a <link> element.
func linkRelHref(n *html.Node) (rel, href string) {
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "rel":
			rel = strings.ToLower(strings.TrimSpace(a.Val))
		case "href":
			href = strings.TrimSpace(a.Val)
		}
	}
	return rel, href
}

// relHasIcon reports whether a rel attribute denotes a plain favicon
// ("icon" or "shortcut icon"). It deliberately excludes "apple-touch-icon",
// which is a single space-free token handled as a lower-priority fallback.
func relHasIcon(rel string) bool {
	for _, tok := range strings.Fields(rel) {
		if tok == "icon" || tok == "shortcut" {
			return true
		}
	}
	return false
}

// truncateTitle bounds a derived page title to maxBookmarkTitleLen runes.
func truncateTitle(s string) string {
	r := []rune(s)
	if len(r) > maxBookmarkTitleLen {
		return string(r[:maxBookmarkTitleLen])
	}
	return s
}
