package handlers

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

func searchRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewSearchHandler(db)
	v1 := r.Group("/api/v1")
	RegisterSearchRoutes(v1, h)
	return r
}

type searchRespEnvelope struct {
	Data []struct {
		Thread struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"thread"`
		Matches []struct {
			MessageID int64  `json:"message_id"`
			Snippet   string `json:"snippet"`
		} `json:"matches"`
	} `json:"data"`
}

type globalSearchRespEnvelope struct {
	Data struct {
		Threads []struct {
			ID    int64  `json:"id"`
			Title string `json:"title"`
		} `json:"threads"`
		Messages []struct {
			ID       int64  `json:"id"`
			ThreadID int64  `json:"thread_id"`
			Snippet  string `json:"snippet"`
		} `json:"messages"`
	} `json:"data"`
}

func TestBuildSnippet(t *testing.T) {
	tests := []struct {
		name     string
		content  string
		query    string
		wantMark bool   // should contain <mark>...</mark>
		wantSub  string // substring expected in output
	}{
		{
			name:     "simple match",
			content:  "hello world",
			query:    "world",
			wantMark: true,
			wantSub:  "<mark>world</mark>",
		},
		{
			name:     "no match fallback",
			content:  "abc",
			query:    "xyz",
			wantMark: false,
			wantSub:  "abc",
		},
		{
			name:     "long content match in middle",
			content:  strings.Repeat("a", 200) + "TARGET" + strings.Repeat("b", 200),
			query:    "TARGET",
			wantMark: true,
			wantSub:  "<mark>TARGET</mark>",
		},
		{
			name:     "short content no ellipsis",
			content:  "short text here",
			query:    "text",
			wantMark: true,
			wantSub:  "<mark>text</mark>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSnippet(tt.content, tt.query)
			if tt.wantMark && !strings.Contains(got, "<mark>") {
				t.Errorf("expected <mark> tag in snippet, got %q", got)
			}
			if !tt.wantMark && strings.Contains(got, "<mark>") {
				t.Errorf("did not expect <mark> tag in snippet, got %q", got)
			}
			if !strings.Contains(got, tt.wantSub) {
				t.Errorf("expected snippet to contain %q, got %q", tt.wantSub, got)
			}
		})
	}
}

func TestBuildSnippet_NoMatch(t *testing.T) {
	got := buildSnippet("abc def ghi", "xyz")
	if strings.Contains(got, "<mark>") {
		t.Errorf("expected no mark tag for unmatched query, got %q", got)
	}
	if got != "abc def ghi" {
		t.Errorf("expected full content for short unmatched, got %q", got)
	}
}

func TestBuildSnippet_ShortContent(t *testing.T) {
	got := buildSnippet("hello world", "hello")
	if !strings.Contains(got, "<mark>hello</mark>") {
		t.Errorf("expected <mark>hello</mark> in snippet, got %q", got)
	}
	// Short content should not have leading "..."
	if strings.HasPrefix(got, "...") {
		t.Errorf("short content should not have leading ellipsis, got %q", got)
	}
}

func TestBuildSnippet_LongContent(t *testing.T) {
	content := strings.Repeat("x", 200) + "NEEDLE" + strings.Repeat("y", 200)
	got := buildSnippet(content, "NEEDLE")

	if !strings.Contains(got, "<mark>NEEDLE</mark>") {
		t.Errorf("expected <mark>NEEDLE</mark>, got %q", got)
	}
	// Match is far from start, so we expect leading "..."
	if !strings.HasPrefix(got, "...") {
		t.Errorf("expected leading ellipsis for long content, got %q", got)
	}
	// Match is far from end, so we expect trailing "..."
	if !strings.HasSuffix(got, "...") {
		t.Errorf("expected trailing ellipsis for long content, got %q", got)
	}
}

func TestBuildSnippet_DiacriticMatch(t *testing.T) {
	got := buildSnippet("příliš žluťoučký kůň", "prilis")
	if !strings.Contains(got, "<mark>") {
		t.Errorf("expected diacritic match to produce <mark> tag, got %q", got)
	}
	// The original accented text should be in the mark.
	if !strings.Contains(got, "příliš") {
		t.Errorf("expected original accented text preserved, got %q", got)
	}
}

func TestStripDiacritics(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"příliš žluťoučký", "prilis zlutoucky"},
		{"café", "cafe"},
		{"ascii", "ascii"},
		{"", ""},
		{"Ñoño", "Nono"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := stripDiacritics(tt.input)
			if got != tt.want {
				t.Errorf("stripDiacritics(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// addMessage inserts a message in the given thread for tests.
func addMessage(t *testing.T, db *gorm.DB, threadID int64, role, content string) models.Message {
	t.Helper()
	m := models.Message{ThreadID: threadID, Role: role, Content: content}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	return m
}

func TestSearch_ByThreadTitle(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	model := "sonnet"
	th := models.Thread{Title: "Project Phoenix kickoff", Model: &model}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search?q=phoenix", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp searchRespEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 result, got %d (%s)", len(resp.Data), w.Body.String())
	}
	got := resp.Data[0]
	if got.Thread.ID != th.ID {
		t.Errorf("expected thread %d, got %d", th.ID, got.Thread.ID)
	}
	if len(got.Matches) != 0 {
		t.Errorf("expected empty matches for title-only hit, got %d", len(got.Matches))
	}
}

func TestSearch_ByTagName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Untitled chat"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	tag := models.Tag{Name: "research", Color: "#abcdef"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := db.Exec("INSERT INTO thread_tags (thread_id, tag_id) VALUES (?, ?)", th.ID, tag.ID).Error; err != nil {
		t.Fatalf("link thread_tags: %v", err)
	}

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search?q=research", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp searchRespEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Thread.ID != th.ID {
		t.Fatalf("expected thread %d via tag match, got %s", th.ID, w.Body.String())
	}
	if len(resp.Data[0].Matches) != 0 {
		t.Errorf("expected empty matches for tag-only hit, got %d", len(resp.Data[0].Matches))
	}
}

func TestSearch_ByPersonaName(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	persona := models.Persona{Name: "Linguist", SystemPrompt: "you are a linguist"}
	if err := db.Create(&persona).Error; err != nil {
		t.Fatalf("create persona: %v", err)
	}
	th := models.Thread{Title: "Untitled", PersonaID: &persona.ID, PersonaName: persona.Name}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search?q=linguist", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp searchRespEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Thread.ID != th.ID {
		t.Fatalf("expected thread %d via persona match, got %s", th.ID, w.Body.String())
	}
	if len(resp.Data[0].Matches) != 0 {
		t.Errorf("expected empty matches for persona-only hit, got %d", len(resp.Data[0].Matches))
	}
}

func TestSearch_ByMessageBodyKeepsSnippet(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Casual chat"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	addMessage(t, db, th.ID, "user", "Let's discuss quokka behavior today.")

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search?q=quokka", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp searchRespEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 || resp.Data[0].Thread.ID != th.ID {
		t.Fatalf("expected message-body match for thread %d, got %s", th.ID, w.Body.String())
	}
	if len(resp.Data[0].Matches) != 1 {
		t.Fatalf("expected exactly 1 message match, got %d", len(resp.Data[0].Matches))
	}
	if !strings.Contains(resp.Data[0].Matches[0].Snippet, "<mark>quokka</mark>") {
		t.Errorf("expected highlighted snippet, got %q", resp.Data[0].Matches[0].Snippet)
	}
}

func TestSearch_TitleAndBodyDeduped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Quokka enthusiasts"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	addMessage(t, db, th.ID, "user", "We love quokka photos.")

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search?q=quokka", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp searchRespEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected single de-duplicated result, got %d (%s)", len(resp.Data), w.Body.String())
	}
	if resp.Data[0].Thread.ID != th.ID {
		t.Errorf("expected thread %d, got %d", th.ID, resp.Data[0].Thread.ID)
	}
	if len(resp.Data[0].Matches) != 1 {
		t.Errorf("expected message match preserved on dedup, got %d matches", len(resp.Data[0].Matches))
	}
}

func TestGlobalSearch_ByTagAndPersona(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Thread 1 matched by tag name.
	t1 := models.Thread{Title: "Notes about cooking"}
	if err := db.Create(&t1).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	tag := models.Tag{Name: "Czechia", Color: "#112233"}
	if err := db.Create(&tag).Error; err != nil {
		t.Fatalf("create tag: %v", err)
	}
	if err := db.Exec("INSERT INTO thread_tags (thread_id, tag_id) VALUES (?, ?)", t1.ID, tag.ID).Error; err != nil {
		t.Fatalf("link thread_tags: %v", err)
	}

	// Thread 2 matched by persona name.
	persona := models.Persona{Name: "Czechia tour guide", SystemPrompt: "p"}
	if err := db.Create(&persona).Error; err != nil {
		t.Fatalf("create persona: %v", err)
	}
	t2 := models.Thread{Title: "Trip plans", PersonaID: &persona.ID, PersonaName: persona.Name}
	if err := db.Create(&t2).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/global?q=czechia", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp globalSearchRespEnvelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	got := map[int64]bool{}
	for _, th := range resp.Data.Threads {
		got[th.ID] = true
	}
	if !got[t1.ID] {
		t.Errorf("expected thread %d (tag match) in global search threads, got %s", t1.ID, w.Body.String())
	}
	if !got[t2.ID] {
		t.Errorf("expected thread %d (persona match) in global search threads, got %s", t2.ID, w.Body.String())
	}
}
