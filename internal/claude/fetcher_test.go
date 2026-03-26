package claude

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExtractText_SimpleHTML(t *testing.T) {
	html := `<html><body><p>Hello world</p></body></html>`
	got := extractText(html)
	if !strings.Contains(got, "Hello world") {
		t.Errorf("expected 'Hello world', got %q", got)
	}
}

func TestExtractText_StripsScriptAndStyle(t *testing.T) {
	html := `<html><head><style>body{color:red}</style></head><body><script>alert(1)</script><p>Content</p></body></html>`
	got := extractText(html)
	if strings.Contains(got, "alert") {
		t.Error("expected script content to be stripped")
	}
	if strings.Contains(got, "color:red") {
		t.Error("expected style content to be stripped")
	}
	if !strings.Contains(got, "Content") {
		t.Errorf("expected 'Content' in output, got %q", got)
	}
}

func TestExtractText_StripsNavFooterHeader(t *testing.T) {
	html := `<html><body><nav>Menu</nav><main><p>Main content</p></main><footer>Footer</footer></body></html>`
	got := extractText(html)
	if strings.Contains(got, "Menu") {
		t.Error("expected nav content to be stripped")
	}
	if strings.Contains(got, "Footer") {
		t.Error("expected footer content to be stripped")
	}
	if !strings.Contains(got, "Main content") {
		t.Errorf("expected 'Main content', got %q", got)
	}
}

func TestExtractText_PlainText(t *testing.T) {
	text := "Just plain text without any HTML"
	got := extractText(text)
	if !strings.Contains(got, "Just plain text") {
		t.Errorf("expected plain text passthrough, got %q", got)
	}
}

func TestExtractText_Truncation(t *testing.T) {
	long := strings.Repeat("x", maxCharsPerSource+1000)
	got := extractText(long)
	if len(got) > maxCharsPerSource+100 {
		t.Errorf("expected truncation to ~%d chars, got %d", maxCharsPerSource, len(got))
	}
}

func TestTruncateSources_UnderBudget(t *testing.T) {
	sources := []fetchedSource{
		{label: "A", url: "http://a.com", content: "short"},
	}
	got := truncateSources(sources)
	if len(got) != 1 || got[0].content != "short" {
		t.Errorf("expected unchanged source, got %+v", got)
	}
}

func TestTruncateSources_OverBudget(t *testing.T) {
	big := strings.Repeat("a", 150_000)
	sources := []fetchedSource{
		{label: "A", url: "http://a.com", content: big},
		{label: "B", url: "http://b.com", content: big},
	}
	got := truncateSources(sources)
	total := 0
	for _, s := range got {
		total += len(s.content)
	}
	if total > maxTotalSourceChars {
		t.Errorf("expected total <= %d, got %d", maxTotalSourceChars, total)
	}
}

func TestFetchSources_Integration(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		fmt.Fprint(w, "<html><body><p>Test content</p></body></html>")
	}))
	defer srv.Close()

	// Clear cache for clean test
	fetchCacheMu.Lock()
	delete(fetchCache, srv.URL)
	fetchCacheMu.Unlock()

	result := FetchSources(context.Background(), []SourceInput{
		{URL: srv.URL, Label: "Test"},
	})

	if !strings.Contains(result, "Test content") {
		t.Errorf("expected fetched content, got %q", result)
	}
	if !strings.Contains(result, "## Source: Test") {
		t.Errorf("expected source header, got %q", result)
	}
}

func TestFetchSources_FailedURL(t *testing.T) {
	badURL := "http://127.0.0.1:1"
	fetchCacheMu.Lock()
	delete(fetchCache, badURL)
	fetchCacheMu.Unlock()

	result := FetchSources(context.Background(), []SourceInput{
		{URL: badURL, Label: "Bad"},
	})

	if !strings.Contains(result, "FETCH FAILED") {
		t.Errorf("expected FETCH FAILED marker, got %q", result)
	}
}

func TestFetchSources_PlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "Plain text content")
	}))
	defer srv.Close()

	fetchCacheMu.Lock()
	delete(fetchCache, srv.URL)
	fetchCacheMu.Unlock()

	result := FetchSources(context.Background(), []SourceInput{
		{URL: srv.URL, Label: "Docs"},
	})

	if !strings.Contains(result, "Plain text content") {
		t.Errorf("expected plain text, got %q", result)
	}
}

func TestFetchSources_Empty(t *testing.T) {
	result := FetchSources(context.Background(), nil)
	if result != "" {
		t.Errorf("expected empty string for no sources, got %q", result)
	}
}
