package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"botka/internal/models"
)

func TestBuildPrefixTSQuery(t *testing.T) {
	cases := map[string]string{
		"":                  "",
		"   ":               "",
		"zalu":              "zalu:*",
		"horiz zalu":        "horiz:* & zalu:*",
		"  spaced   words ": "spaced:* & words:*",
		"žaluzie":           "žaluzie:*", // accents preserved; unaccented in SQL
		"a:* & b | c":       "a:* & b:* & c:*",
		"!!!":               "",
		"foo!bar":           "foobar:*",
	}
	for in, want := range cases {
		if got := buildPrefixTSQuery(in); got != want {
			t.Errorf("buildPrefixTSQuery(%q) = %q, want %q", in, got, want)
		}
	}
}

type unifiedSearchEnvelope struct {
	Data UnifiedSearchResult `json:"data"`
}

func decodeUnifiedSearch(t *testing.T, body []byte) UnifiedSearchResult {
	t.Helper()
	var env unifiedSearchEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode unified search: %v (%s)", err, string(body))
	}
	return env.Data
}

func unifiedSearch(t *testing.T, r *gin.Engine, q string) UnifiedSearchResult {
	t.Helper()
	w := doRequest(r, http.MethodGet, "/api/v1/search/all?q="+url.QueryEscape(q), "")
	if w.Code != http.StatusOK {
		t.Fatalf("q=%q: expected 200, got %d: %s", q, w.Code, w.Body.String())
	}
	return decodeUnifiedSearch(t, w.Body.Bytes())
}

// TestSearchAll_ThreadTitlePriority verifies that a thread whose title matches
// ranks above a thread that matches only in its message body, and that prefix +
// diacritic folding both apply.
func TestSearchAll_ThreadTitlePriority(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Title match (no relevant body).
	titleThread := models.Thread{Title: "Žaluzie v ložnici"}
	if err := db.Create(&titleThread).Error; err != nil {
		t.Fatalf("create title thread: %v", err)
	}
	addMessage(t, db, titleThread.ID, "user", "Dobrý den, mám dotaz.")

	// Body-only match (unrelated title).
	bodyThread := models.Thread{Title: "Obecný dotaz"}
	if err := db.Create(&bodyThread).Error; err != nil {
		t.Fatalf("create body thread: %v", err)
	}
	addMessage(t, db, bodyThread.ID, "assistant", "Máš horizontální žaluzie do okna.")

	r := searchRouter(db)

	// Prefix, no diacritics.
	res := unifiedSearch(t, r, "zalu")
	if len(res.Threads) != 2 {
		t.Fatalf("expected 2 thread hits, got %d (%+v)", len(res.Threads), res.Threads)
	}
	if res.Threads[0].ThreadID != titleThread.ID {
		t.Errorf("title-match thread should rank first; got order %d, %d",
			res.Threads[0].ThreadID, res.Threads[1].ThreadID)
	}
	if !res.Threads[0].MatchedTitle {
		t.Errorf("first hit should have matched_title=true")
	}
	// Body-only hit should carry a snippet from the matching message.
	if res.Threads[1].MessageID == nil || res.Threads[1].ContentSnippet == "" {
		t.Errorf("body-only hit should include a message snippet, got %+v", res.Threads[1])
	}
}

// TestSearchAll_TaskFound verifies tasks are searchable with prefix + diacritic
// folding and that title matches outrank spec-only matches.
func TestSearchAll_TaskFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := models.Project{
		Name:           "proj",
		Path:           "/tmp/unified-" + uuid.New().String()[:8],
		BranchStrategy: "main",
		Active:         true,
	}
	if err := db.Create(&proj).Error; err != nil {
		t.Fatalf("create project: %v", err)
	}

	titleTask := models.Task{Title: "Opravit žaluzie", Spec: "nic", ProjectID: proj.ID, Status: models.TaskStatusQueued}
	specTask := models.Task{Title: "Jiný úkol", Spec: "souvisí se žaluziemi nepřímo", ProjectID: proj.ID, Status: models.TaskStatusQueued}
	if err := db.Create(&titleTask).Error; err != nil {
		t.Fatalf("create title task: %v", err)
	}
	if err := db.Create(&specTask).Error; err != nil {
		t.Fatalf("create spec task: %v", err)
	}

	r := searchRouter(db)
	res := unifiedSearch(t, r, "zaluz")
	if len(res.Tasks) != 2 {
		t.Fatalf("expected 2 task hits, got %d (%+v)", len(res.Tasks), res.Tasks)
	}
	if res.Tasks[0].TaskID != titleTask.ID.String() {
		t.Errorf("title-match task should rank first; got %s then %s",
			res.Tasks[0].TaskID, res.Tasks[1].TaskID)
	}
}

// TestSearchAll_EmptyQuery returns empty sections without error.
func TestSearchAll_EmptyQuery(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	r := searchRouter(db)
	res := unifiedSearch(t, r, "   ")
	if len(res.Threads) != 0 || len(res.Tasks) != 0 {
		t.Errorf("expected empty result, got %+v", res)
	}
}
