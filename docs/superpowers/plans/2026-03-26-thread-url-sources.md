# Thread URL Sources Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add per-thread URL sources that are fetched and included in context assembly when a new Claude session starts.

**Architecture:** New `thread_sources` table with CRUD API, HTML content extraction via `golang.org/x/net/html`, 5-minute fetch cache, and a Sources UI section in the thread context menu. Sources are fetched during context assembly (after system prompt, before conversation history).

**Tech Stack:** Go (GORM model, Gin handler, net/http fetcher, x/net/html parser), PostgreSQL migration, React/TypeScript frontend component.

---

### Task 1: Database Migration

**Files:**
- Create: `migrations/009_thread_sources.up.sql`
- Create: `migrations/009_thread_sources.down.sql`

- [ ] **Step 1: Create up migration**

```sql
CREATE TABLE thread_sources (
    id BIGSERIAL PRIMARY KEY,
    thread_id BIGINT NOT NULL REFERENCES threads(id) ON DELETE CASCADE,
    url TEXT NOT NULL,
    label TEXT NOT NULL DEFAULT '',
    position INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_thread_sources_thread_id ON thread_sources(thread_id);
```

Write to `migrations/009_thread_sources.up.sql`.

- [ ] **Step 2: Create down migration**

```sql
DROP TABLE IF EXISTS thread_sources;
```

Write to `migrations/009_thread_sources.down.sql`.

- [ ] **Step 3: Commit**

```bash
git add migrations/009_thread_sources.up.sql migrations/009_thread_sources.down.sql
git commit -m "feat: add thread_sources migration"
```

---

### Task 2: GORM Model

**Files:**
- Create: `internal/models/thread_source.go`

- [ ] **Step 1: Create the ThreadSource model**

```go
package models

import "time"

// ThreadSource represents a URL source attached to a chat thread.
// Sources are fetched during context assembly and their content is
// included in the system prompt sent to Claude.
type ThreadSource struct {
	ID        int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	ThreadID  int64     `gorm:"not null" json:"thread_id"`
	URL       string    `gorm:"type:text;not null" json:"url"`
	Label     string    `gorm:"type:text;not null;default:''" json:"label"`
	Position  int       `gorm:"not null;default:0" json:"position"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName returns the database table name for the ThreadSource model.
func (ThreadSource) TableName() string {
	return "thread_sources"
}
```

- [ ] **Step 2: Add ThreadSource to test DB setup**

In `internal/handlers/testhelpers_test.go`, add `&models.ThreadSource{}` to the `AutoMigrate` call (after `&models.WebAuthnCredential{}`), and add `thread_sources` to the `cleanTables` TRUNCATE list (before `thread_tags`).

- [ ] **Step 3: Commit**

```bash
git add internal/models/thread_source.go internal/handlers/testhelpers_test.go
git commit -m "feat: add ThreadSource GORM model"
```

---

### Task 3: URL Fetcher and HTML Extractor

**Files:**
- Create: `internal/claude/fetcher.go`
- Create: `internal/claude/fetcher_test.go`

- [ ] **Step 1: Write failing tests for HTML text extraction**

Create `internal/claude/fetcher_test.go`:

```go
package claude

import (
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
		{label: "A", url: "http://a.com", content: "short", err: nil},
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/pi/projects/botka && go test ./internal/claude/ -run TestExtract -v`
Expected: Compilation failure (functions not defined yet)

- [ ] **Step 3: Implement fetcher**

Create `internal/claude/fetcher.go`:

```go
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

// SourceInput is the input for a single source to fetch.
type SourceInput struct {
	URL   string
	Label string
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
	budget := maxTotalSourceChars / len(sources)
	result := make([]fetchedSource, len(sources))
	copy(result, sources)
	for i := range result {
		if len(result[i].content) > budget {
			result[i].content = result[i].content[:budget] + "\n[truncated]"
		}
	}
	return result
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /home/pi/projects/botka && go test ./internal/claude/ -run "TestExtract|TestTruncate" -v`
Expected: All PASS

- [ ] **Step 5: Write test for FetchSources with HTTP test server**

Add to `internal/claude/fetcher_test.go`:

```go
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
	// Clear any cached result
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
```

Add these imports to the test file: `"context"`, `"fmt"`, `"net/http"`, `"net/http/httptest"`.

- [ ] **Step 6: Run all fetcher tests**

Run: `cd /home/pi/projects/botka && go test ./internal/claude/ -run "TestExtract|TestTruncate|TestFetchSources" -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/claude/fetcher.go internal/claude/fetcher_test.go
git commit -m "feat: add URL fetcher with HTML extraction and caching"
```

---

### Task 4: Integrate Sources into Context Assembly

**Files:**
- Modify: `internal/claude/context.go` (AssembleContext signature and body)
- Modify: `internal/claude/context_test.go` (update tests for new parameter)
- Modify: `internal/handlers/chat.go` (pass sources to AssembleContext)

- [ ] **Step 1: Update AssembleContext to accept and include sources**

Add a new parameter `sources []SourceInput` to `AssembleContext`. After the system prompt layer (Layer 6) and before the project CLAUDE.md layer (Layer 7), add:

```go
// Layer 6b: Thread URL sources (fetched fresh)
if len(sources) > 0 {
    if sourceContent := FetchSources(ctx, sources); sourceContent != "" {
        parts = append(parts, "# Reference Sources\n\n"+sourceContent)
    }
}
```

The updated signature:

```go
func AssembleContext(ctx context.Context, cfg ContextConfig, threadID int64, getMemories MemoryFunc, systemPrompt, folderClaudeMD, projectName, projectPath string, sources []SourceInput, messages []models.Message) (string, error)
```

- [ ] **Step 2: Update all existing callers**

In `internal/handlers/chat.go`, update the `AssembleContext` call (around line 439) to load thread sources from DB and pass them:

```go
// Load thread sources
var dbSources []models.ThreadSource
h.db.Where("thread_id = ?", threadID).Order("position ASC").Find(&dbSources)
var sourceInputs []claude.SourceInput
for _, s := range dbSources {
    sourceInputs = append(sourceInputs, claude.SourceInput{URL: s.URL, Label: s.Label})
}

contextFile, err = claude.AssembleContext(c.Request.Context(), h.contextCfg, threadID, getMemories, thread.SystemPrompt, projectClaudeMD, projectName, projectPath, sourceInputs, existingMessages)
```

- [ ] **Step 3: Update existing context tests**

In `internal/claude/context_test.go`, update all `AssembleContext` calls to include the new `sources` parameter (pass `nil` for existing tests):

```go
// Every existing call changes from:
//   AssembleContext(ctx, cfg, id, memFn, sysPrompt, folderMD, projName, projPath, messages)
// To:
//   AssembleContext(ctx, cfg, id, memFn, sysPrompt, folderMD, projName, projPath, nil, messages)
```

- [ ] **Step 4: Add a test for sources in context assembly**

Add to `internal/claude/context_test.go`:

```go
func TestAssembleContext_WithSources(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		fmt.Fprint(w, "Reference documentation content")
	}))
	defer srv.Close()

	// Clear cache
	fetchCacheMu.Lock()
	delete(fetchCache, srv.URL)
	fetchCacheMu.Unlock()

	workspace := t.TempDir()
	contextDir := t.TempDir()
	cfg := ContextConfig{OpenClawWorkspace: workspace, ContextDir: contextDir}

	sources := []SourceInput{{URL: srv.URL, Label: "Test Docs"}}

	path, err := AssembleContext(context.Background(), cfg, 99, nil, "Be helpful", "", "", "", sources, nil)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	assembled := string(content)
	if !strings.Contains(assembled, "# Reference Sources") {
		t.Error("expected Reference Sources section")
	}
	if !strings.Contains(assembled, "Reference documentation content") {
		t.Error("expected fetched source content")
	}
	if !strings.Contains(assembled, "Test Docs") {
		t.Error("expected source label")
	}
}
```

Add imports: `"fmt"`, `"net/http"`, `"net/http/httptest"`.

- [ ] **Step 5: Run all context tests**

Run: `cd /home/pi/projects/botka && go test ./internal/claude/ -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/claude/context.go internal/claude/context_test.go internal/handlers/chat.go
git commit -m "feat: integrate URL sources into context assembly"
```

---

### Task 5: CRUD Handler for Thread Sources

**Files:**
- Create: `internal/handlers/thread_source.go`
- Create: `internal/handlers/thread_source_test.go`

- [ ] **Step 1: Write failing tests**

Create `internal/handlers/thread_source_test.go`:

```go
package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func threadSourceRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewThreadSourceHandler(db)
	v1 := r.Group("/api/v1")
	RegisterThreadSourceRoutes(v1, h)
	return r
}

func TestThreadSource_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

func TestThreadSource_CreateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"url":"https://example.com/docs","label":"Example Docs"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["url"] != "https://example.com/docs" {
		t.Errorf("expected url, got %v", data["url"])
	}
	if data["label"] != "Example Docs" {
		t.Errorf("expected label, got %v", data["label"])
	}
}

func TestThreadSource_CreateMissingURL(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"label":"No URL"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestThreadSource_UpdateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	// Create first
	body := `{"url":"https://old.com","label":"Old"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	sourceID := int64(createResp["data"].(map[string]interface{})["id"].(float64))

	// Update
	body = `{"url":"https://new.com","label":"New"}`
	w = doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/sources/%d", th.ID, sourceID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["url"] != "https://new.com" {
		t.Errorf("expected updated url, got %v", data["url"])
	}
}

func TestThreadSource_DeleteSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"url":"https://delete.me","label":"Bye"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	sourceID := int64(createResp["data"].(map[string]interface{})["id"].(float64))

	w = doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/sources/%d", th.ID, sourceID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestThreadSource_Reorder(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	// Create 3 sources
	var ids []int64
	for _, u := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		body := fmt.Sprintf(`{"url":"%s"}`, u)
		w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
		if w.Code != http.StatusCreated {
			t.Fatalf("create failed: %d", w.Code)
		}
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		ids = append(ids, int64(resp["data"].(map[string]interface{})["id"].(float64)))
	}

	// Reorder: reverse
	reorderBody := fmt.Sprintf(`{"ids":[%d,%d,%d]}`, ids[2], ids[0], ids[1])
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/sources/reorder", th.ID), reorderBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List and verify order
	w = doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), "")
	var listResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	data := listResp["data"].([]interface{})
	firstURL := data[0].(map[string]interface{})["url"].(string)
	if firstURL != "https://c.com" {
		t.Errorf("expected first source to be c.com after reorder, got %s", firstURL)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /home/pi/projects/botka && go test ./internal/handlers/ -run TestThreadSource -v`
Expected: Compilation failure (handler not defined yet)

- [ ] **Step 3: Implement the handler**

Create `internal/handlers/thread_source.go`:

```go
package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// ThreadSourceHandler handles HTTP requests for thread URL source resources.
type ThreadSourceHandler struct {
	db *gorm.DB
}

// NewThreadSourceHandler creates a new ThreadSourceHandler.
func NewThreadSourceHandler(db *gorm.DB) *ThreadSourceHandler {
	return &ThreadSourceHandler{db: db}
}

// RegisterThreadSourceRoutes attaches thread source endpoints to the given router group.
func RegisterThreadSourceRoutes(rg *gin.RouterGroup, h *ThreadSourceHandler) {
	rg.GET("/threads/:id/sources", h.List)
	rg.POST("/threads/:id/sources", h.Create)
	rg.PUT("/threads/:id/sources/reorder", h.Reorder)
	rg.PUT("/threads/:id/sources/:source_id", h.Update)
	rg.DELETE("/threads/:id/sources/:source_id", h.Delete)
}

// List returns all sources for a thread, ordered by position.
func (h *ThreadSourceHandler) List(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var sources []models.ThreadSource
	if err := h.db.Where("thread_id = ?", threadID).Order("position ASC, id ASC").Find(&sources).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list sources")
		return
	}
	if sources == nil {
		sources = []models.ThreadSource{}
	}
	respondOK(c, sources)
}

type createSourceRequest struct {
	URL   string `json:"url"`
	Label string `json:"label"`
}

// Create adds a new source to a thread.
func (h *ThreadSourceHandler) Create(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		respondError(c, http.StatusBadRequest, "url is required")
		return
	}

	// Get next position
	var maxPos int
	h.db.Model(&models.ThreadSource{}).Where("thread_id = ?", threadID).Select("COALESCE(MAX(position), -1)").Scan(&maxPos)

	source := models.ThreadSource{
		ThreadID: threadID,
		URL:      req.URL,
		Label:    req.Label,
		Position: maxPos + 1,
	}
	if err := h.db.Create(&source).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create source")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": source})
}

// Update modifies an existing source.
func (h *ThreadSourceHandler) Update(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}
	sourceID, err := paramInt64(c, "source_id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid source id")
		return
	}

	var req createSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.URL == "" {
		respondError(c, http.StatusBadRequest, "url is required")
		return
	}

	var source models.ThreadSource
	if err := h.db.Where("id = ? AND thread_id = ?", sourceID, threadID).First(&source).Error; err != nil {
		respondError(c, http.StatusNotFound, "source not found")
		return
	}

	source.URL = req.URL
	source.Label = req.Label
	if err := h.db.Save(&source).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update source")
		return
	}

	respondOK(c, source)
}

// Delete removes a source from a thread.
func (h *ThreadSourceHandler) Delete(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}
	sourceID, err := paramInt64(c, "source_id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid source id")
		return
	}

	result := h.db.Where("id = ? AND thread_id = ?", sourceID, threadID).Delete(&models.ThreadSource{})
	if result.RowsAffected == 0 {
		respondError(c, http.StatusNotFound, "source not found")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

type reorderRequest struct {
	IDs []int64 `json:"ids"`
}

// Reorder updates the position of sources within a thread.
func (h *ThreadSourceHandler) Reorder(c *gin.Context) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var req reorderRequest
	if err := c.ShouldBindJSON(&req); err != nil || len(req.IDs) == 0 {
		respondError(c, http.StatusBadRequest, "ids array is required")
		return
	}

	tx := h.db.Begin()
	for i, id := range req.IDs {
		if err := tx.Model(&models.ThreadSource{}).Where("id = ? AND thread_id = ?", id, threadID).Update("position", i).Error; err != nil {
			tx.Rollback()
			respondError(c, http.StatusInternalServerError, "failed to reorder sources")
			return
		}
	}
	tx.Commit()

	respondOK(c, gin.H{"status": "ok"})
}
```

- [ ] **Step 4: Run handler tests**

Run: `cd /home/pi/projects/botka && go test ./internal/handlers/ -run TestThreadSource -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/thread_source.go internal/handlers/thread_source_test.go
git commit -m "feat: add CRUD handler for thread sources"
```

---

### Task 6: Register Routes in Server

**Files:**
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Register thread source routes**

In `cmd/server/main.go` `setupRouter()`, after the thread handler registration (line ~186), add:

```go
threadSourceHandler := handlers.NewThreadSourceHandler(db)
handlers.RegisterThreadSourceRoutes(v1, threadSourceHandler)
```

- [ ] **Step 2: Run `make check` to verify everything compiles and passes**

Run: `cd /home/pi/projects/botka && make check`
Expected: All checks pass

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: register thread source routes in server"
```

---

### Task 7: Frontend Types and API Client

**Files:**
- Modify: `frontend/src/types/index.ts`
- Modify: `frontend/src/api/client.ts`

- [ ] **Step 1: Add ThreadSource type**

In `frontend/src/types/index.ts`, after the `Thread` interface (around line 152), add:

```ts
export interface ThreadSource {
  id: number
  thread_id: number
  url: string
  label: string
  position: number
  created_at: string
  updated_at: string
}
```

- [ ] **Step 2: Add API functions for thread sources**

In `frontend/src/api/client.ts`, after the thread operations section (around line 291), add:

```ts
// Thread Sources

export function fetchThreadSources(threadId: number): Promise<ThreadSource[]> {
  return requestData<ThreadSource[]>(`/threads/${threadId}/sources`)
}

export function createThreadSource(threadId: number, data: { url: string; label?: string }): Promise<ThreadSource> {
  return requestData<ThreadSource>(`/threads/${threadId}/sources`, {
    method: 'POST',
    body: JSON.stringify(data),
  })
}

export function updateThreadSource(threadId: number, sourceId: number, data: { url: string; label?: string }): Promise<ThreadSource> {
  return requestData<ThreadSource>(`/threads/${threadId}/sources/${sourceId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  })
}

export function deleteThreadSource(threadId: number, sourceId: number): Promise<void> {
  return request<void>(`/threads/${threadId}/sources/${sourceId}`, { method: 'DELETE' })
}

export function reorderThreadSources(threadId: number, ids: number[]): Promise<void> {
  return requestData<void>(`/threads/${threadId}/sources/reorder`, {
    method: 'PUT',
    body: JSON.stringify({ ids }),
  })
}
```

Add `ThreadSource` to the import in line 1. Add all five functions to the `api` object at the bottom of the file.

- [ ] **Step 3: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/client.ts
git commit -m "feat: add ThreadSource types and API client functions"
```

---

### Task 8: Frontend Sources UI Component

**Files:**
- Create: `frontend/src/components/ThreadSourcesEditor.tsx`

- [ ] **Step 1: Create the ThreadSourcesEditor component**

```tsx
import { useState, useEffect, useCallback } from 'react'
import { Plus, Trash2, Globe, GripVertical } from 'lucide-react'
import type { ThreadSource } from '../types'
import {
  fetchThreadSources,
  createThreadSource,
  updateThreadSource,
  deleteThreadSource,
  reorderThreadSources,
} from '../api/client'

interface Props {
  threadId: number
  onClose: () => void
}

export default function ThreadSourcesEditor({ threadId, onClose }: Props) {
  const [sources, setSources] = useState<ThreadSource[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<number | null>(null)
  const [dragIdx, setDragIdx] = useState<number | null>(null)

  const load = useCallback(async () => {
    try {
      const data = await fetchThreadSources(threadId)
      setSources(data)
    } finally {
      setLoading(false)
    }
  }, [threadId])

  useEffect(() => { load() }, [load])

  const handleAdd = async () => {
    const src = await createThreadSource(threadId, { url: '', label: '' })
    setSources(prev => [...prev, src])
  }

  const handleSave = async (source: ThreadSource) => {
    if (!source.url.trim()) return
    setSaving(source.id)
    try {
      await updateThreadSource(threadId, source.id, { url: source.url, label: source.label })
    } finally {
      setSaving(null)
    }
  }

  const handleDelete = async (id: number) => {
    await deleteThreadSource(threadId, id)
    setSources(prev => prev.filter(s => s.id !== id))
  }

  const handleChange = (id: number, field: 'url' | 'label', value: string) => {
    setSources(prev => prev.map(s => s.id === id ? { ...s, [field]: value } : s))
  }

  const handleDragStart = (idx: number) => setDragIdx(idx)
  const handleDragOver = (e: React.DragEvent, idx: number) => {
    e.preventDefault()
    if (dragIdx === null || dragIdx === idx) return
    const reordered = [...sources]
    const [moved] = reordered.splice(dragIdx, 1)
    reordered.splice(idx, 0, moved)
    setSources(reordered)
    setDragIdx(idx)
  }
  const handleDragEnd = async () => {
    setDragIdx(null)
    await reorderThreadSources(threadId, sources.map(s => s.id))
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
         onClick={onClose}>
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col"
           onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
          <div className="flex items-center gap-2">
            <Globe className="w-4 h-4 text-zinc-500" />
            <h2 className="text-sm font-semibold text-zinc-800">URL Sources</h2>
          </div>
          <button onClick={onClose}
                  className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer">
            Done
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-3 space-y-2">
          {loading ? (
            <p className="text-xs text-zinc-400 py-4 text-center">Loading...</p>
          ) : sources.length === 0 ? (
            <p className="text-xs text-zinc-400 py-4 text-center">
              No sources yet. Add URLs to include their content in the chat context.
            </p>
          ) : (
            sources.map((src, idx) => (
              <div key={src.id}
                   draggable
                   onDragStart={() => handleDragStart(idx)}
                   onDragOver={e => handleDragOver(e, idx)}
                   onDragEnd={handleDragEnd}
                   className={`flex items-center gap-2 group rounded-lg p-2 -mx-2
                              ${dragIdx === idx ? 'bg-blue-50 opacity-70' : 'hover:bg-zinc-50'}`}>
                <GripVertical className="w-3.5 h-3.5 text-zinc-300 cursor-grab flex-shrink-0
                                         opacity-0 group-hover:opacity-100 transition-opacity" />
                <input
                  type="text"
                  value={src.label}
                  onChange={e => handleChange(src.id, 'label', e.target.value)}
                  onBlur={() => handleSave(src)}
                  placeholder="Label"
                  className="w-24 flex-shrink-0 text-xs bg-transparent border border-zinc-200
                             rounded px-2 py-1.5 text-zinc-700 placeholder-zinc-300
                             focus:border-zinc-400 focus:outline-none"
                />
                <input
                  type="url"
                  value={src.url}
                  onChange={e => handleChange(src.id, 'url', e.target.value)}
                  onBlur={() => handleSave(src)}
                  placeholder="https://..."
                  className="flex-1 min-w-0 text-xs bg-transparent border border-zinc-200
                             rounded px-2 py-1.5 text-zinc-700 placeholder-zinc-300
                             focus:border-zinc-400 focus:outline-none"
                />
                {saving === src.id && (
                  <span className="text-[10px] text-zinc-400 flex-shrink-0">Saving...</span>
                )}
                <button onClick={() => handleDelete(src.id)}
                        className="text-zinc-300 hover:text-red-500 p-1 flex-shrink-0
                                   opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))
          )}
        </div>

        <div className="px-5 py-3 border-t border-zinc-100">
          <button onClick={handleAdd}
                  className="flex items-center gap-1.5 text-xs text-zinc-500
                             hover:text-zinc-700 transition-colors cursor-pointer">
            <Plus className="w-3.5 h-3.5" />
            Add source
          </button>
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 2: Commit**

```bash
git add frontend/src/components/ThreadSourcesEditor.tsx
git commit -m "feat: add ThreadSourcesEditor UI component"
```

---

### Task 9: Wire Sources Editor into Thread Sidebar

**Files:**
- Modify: `frontend/src/components/ThreadSidebar.tsx`

- [ ] **Step 1: Add Sources button to thread context menu**

In `ThreadSidebar.tsx`:

1. Add import at top:
```tsx
import ThreadSourcesEditor from './ThreadSourcesEditor'
```

2. Add `Globe` to the lucide-react imports (already imported above).

3. Add state for the sources editor modal (near other state declarations):
```tsx
const [sourcesThreadId, setSourcesThreadId] = useState<number | null>(null)
```

4. In the context menu, after the "Change model" button (around line 390) and before the tags section, add:
```tsx
<button
  onClick={() => { setSourcesThreadId(thread.id); setMenuOpenId(null) }}
  className="w-full flex items-center gap-3 px-3 py-2
             text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
>
  <Globe className="w-4 h-4 flex-shrink-0 text-zinc-400" />
  Sources
</button>
```

5. Render the modal at the component bottom (before the final closing tag):
```tsx
{sourcesThreadId && (
  <ThreadSourcesEditor
    threadId={sourcesThreadId}
    onClose={() => setSourcesThreadId(null)}
  />
)}
```

- [ ] **Step 2: Run frontend type check**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit`
Expected: No type errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/ThreadSidebar.tsx
git commit -m "feat: wire Sources editor into thread sidebar context menu"
```

---

### Task 10: Final Verification

- [ ] **Step 1: Run full check**

Run: `cd /home/pi/projects/botka && make check`
Expected: All checks pass (formatting, vetting, linting, tests, frontend type-check)

- [ ] **Step 2: Fix any issues found by make check**

If there are lint/format/test issues, fix them and re-run.

- [ ] **Step 3: Final commit (if any fixes)**

Commit any remaining fixes needed.
