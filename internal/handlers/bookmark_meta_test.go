package handlers

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNormalizeBookmarkURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"   ", ""},
		{"https://example.com", "https://example.com"},
		{"http://example.com/path", "http://example.com/path"},
		{"example.com", "https://example.com"},
		{"  example.com/x  ", "https://example.com/x"},
		{"//cdn.example.com/a", "https://cdn.example.com/a"},
	}
	for _, tc := range cases {
		if got := normalizeBookmarkURL(tc.in); got != tc.want {
			t.Errorf("normalizeBookmarkURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseBookmarkMeta(t *testing.T) {
	cases := []struct {
		name      string
		html      string
		wantTitle string
		wantIcon  string
	}{
		{
			name:      "title and icon",
			html:      `<html><head><title>Example Page</title><link rel="icon" href="/fav.png"></head><body>x</body></html>`,
			wantTitle: "Example Page",
			wantIcon:  "/fav.png",
		},
		{
			name:      "shortcut icon",
			html:      `<head><title>T</title><link rel="shortcut icon" href="/s.ico"></head>`,
			wantTitle: "T",
			wantIcon:  "/s.ico",
		},
		{
			name:      "apple touch icon fallback",
			html:      `<head><title>A</title><link rel="apple-touch-icon" href="/apple.png"></head>`,
			wantTitle: "A",
			wantIcon:  "/apple.png",
		},
		{
			name:      "plain icon wins over apple",
			html:      `<head><link rel="apple-touch-icon" href="/apple.png"><link rel="icon" href="/icon.png"></head>`,
			wantTitle: "",
			wantIcon:  "/icon.png",
		},
		{
			name:      "no metadata",
			html:      `<html><body>nothing here</body></html>`,
			wantTitle: "",
			wantIcon:  "",
		},
		{
			name:      "title whitespace trimmed",
			html:      "<head><title>   Spaced   </title></head>",
			wantTitle: "Spaced",
			wantIcon:  "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			title, icon := parseBookmarkMeta(tc.html)
			if title != tc.wantTitle {
				t.Errorf("title = %q, want %q", title, tc.wantTitle)
			}
			if icon != tc.wantIcon {
				t.Errorf("icon = %q, want %q", icon, tc.wantIcon)
			}
		})
	}
}

func TestResolveBookmarkURL(t *testing.T) {
	base := mustParseURL(t, "https://example.com/a/b")
	cases := []struct {
		href string
		want string
	}{
		{"/favicon.ico", "https://example.com/favicon.ico"},
		{"icon.png", "https://example.com/a/icon.png"},
		{"https://cdn.example.com/i.png", "https://cdn.example.com/i.png"},
		{"//cdn.example.com/i.png", "https://cdn.example.com/i.png"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := resolveBookmarkURL(base, tc.href); got != tc.want {
			t.Errorf("resolveBookmarkURL(%q) = %q, want %q", tc.href, got, tc.want)
		}
	}
}

func TestDefaultFavicon(t *testing.T) {
	base := mustParseURL(t, "https://example.com/path")
	if got := defaultFavicon(base, "https://example.com/path"); got != "https://example.com/favicon.ico" {
		t.Errorf("defaultFavicon with host = %q, want /favicon.ico", got)
	}
	// No host: falls back to the favicon service keyed on the raw input.
	if got := defaultFavicon(nil, "not a url"); !strings.Contains(got, "s2/favicons") {
		t.Errorf("defaultFavicon without host = %q, want favicon service URL", got)
	}
	if got := defaultFavicon(nil, ""); got != "" {
		t.Errorf("defaultFavicon empty = %q, want empty", got)
	}
}

func TestFetchBookmarkMeta_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>My Site</title><link rel="icon" href="/icon.png"></head><body>hi</body></html>`)
	}))
	defer srv.Close()

	title, favicon := fetchBookmarkMeta(context.Background(), srv.URL)
	if title != "My Site" {
		t.Errorf("title = %q, want %q", title, "My Site")
	}
	if want := srv.URL + "/icon.png"; favicon != want {
		t.Errorf("favicon = %q, want %q", favicon, want)
	}
}

func TestFetchBookmarkMeta_NoIconUsesFavicon(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>No Icon</title></head><body>hi</body></html>`)
	}))
	defer srv.Close()

	title, favicon := fetchBookmarkMeta(context.Background(), srv.URL)
	if title != "No Icon" {
		t.Errorf("title = %q, want %q", title, "No Icon")
	}
	if want := srv.URL + "/favicon.ico"; favicon != want {
		t.Errorf("favicon = %q, want %q", favicon, want)
	}
}

func TestFetchBookmarkMeta_FetchFailureFallsBack(t *testing.T) {
	// Start a server, capture its URL, then close it so the connection is refused.
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	addr := srv.URL
	srv.Close()

	title, favicon := fetchBookmarkMeta(context.Background(), addr)
	// Hostname (127.0.0.1) is used as the title, and /favicon.ico as the icon.
	if !strings.Contains(title, "127.0.0.1") {
		t.Errorf("title = %q, want hostname fallback", title)
	}
	if !strings.HasSuffix(favicon, "/favicon.ico") {
		t.Errorf("favicon = %q, want /favicon.ico fallback", favicon)
	}
}

func mustParseURL(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse %q: %v", raw, err)
	}
	return u
}
