package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"botka/internal/models"
)

type messageSearchEnvelope struct {
	Data  []MessageSearchHit `json:"data"`
	Total int64              `json:"total"`
}

func decodeMessageSearch(t *testing.T, body []byte) messageSearchEnvelope {
	t.Helper()
	var resp messageSearchEnvelope
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal: %v: %s", err, body)
	}
	return resp
}

// addMessageWithTime is like addMessage but lets the test pin created_at,
// which lets us verify rank/created_at tiebreaker ordering deterministically.
func addMessageWithTime(t *testing.T, threadID int64, role, content string, ts time.Time) models.Message {
	t.Helper()
	m := models.Message{
		ThreadID:  threadID,
		Role:      role,
		Content:   content,
		CreatedAt: ts,
	}
	if err := sharedDB.Create(&m).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	// GORM's CreatedAt autoset overrides our value on insert; force it.
	if err := sharedDB.Model(&models.Message{}).
		Where("id = ?", m.ID).
		Update("created_at", ts).Error; err != nil {
		t.Fatalf("set created_at: %v", err)
	}
	return m
}

func TestSearchMessages_EmptyQuery(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if resp.Total != 0 || len(resp.Data) != 0 {
		t.Fatalf("expected empty result for empty query, got %+v", resp)
	}
}

func TestSearchMessages_BasicHit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Quokka chat"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	addMessage(t, db, th.ID, "user", "Let us discuss quokka behavior today.")

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=quokka", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if resp.Total != 1 {
		t.Fatalf("expected total 1, got %d (%s)", resp.Total, w.Body.String())
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(resp.Data))
	}
	hit := resp.Data[0]
	if hit.ThreadID != th.ID {
		t.Errorf("thread id: want %d got %d", th.ID, hit.ThreadID)
	}
	if hit.ThreadTitle != "Quokka chat" {
		t.Errorf("thread title: want %q got %q", "Quokka chat", hit.ThreadTitle)
	}
	if hit.Role != "user" {
		t.Errorf("role: want user got %q", hit.Role)
	}
	if !strings.Contains(hit.ContentSnippet, "<mark>quokka</mark>") {
		t.Errorf("expected <mark>quokka</mark> in snippet, got %q", hit.ContentSnippet)
	}
	if hit.Rank <= 0 {
		t.Errorf("expected positive rank, got %v", hit.Rank)
	}
}

// TestSearchMessages_DiacriticInsensitive guards the migration-036 behavior:
// a query typed without diacritics must match accented Czech content, and an
// accented query must match too. Regression for "search finds nothing" when
// the user types e.g. "zaluzie" looking for messages about "žaluzie".
func TestSearchMessages_DiacriticInsensitive(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Okna"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	addMessage(t, db, th.ID, "assistant", "Máš horizontální žaluzie do okenního křídla.")

	r := searchRouter(db)

	for _, q := range []string{"zaluzie", "žaluzie", "ZALUZIE"} {
		w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q="+url.QueryEscape(q), "")
		if w.Code != http.StatusOK {
			t.Fatalf("q=%q: expected 200, got %d: %s", q, w.Code, w.Body.String())
		}
		resp := decodeMessageSearch(t, w.Body.Bytes())
		if resp.Total != 1 {
			t.Fatalf("q=%q: expected total 1, got %d (%s)", q, resp.Total, w.Body.String())
		}
	}
}

func TestSearchMessages_FiltersDeletedMessages(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Soft delete chat"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	visible := addMessage(t, db, th.ID, "user", "visible quokka mention")
	deleted := addMessage(t, db, th.ID, "assistant", "deleted quokka mention")

	// Soft-delete one message via GORM.
	if err := db.Delete(&models.Message{}, deleted.ID).Error; err != nil {
		t.Fatalf("soft delete: %v", err)
	}

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=quokka", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if resp.Total != 1 {
		t.Fatalf("expected 1 hit (deleted excluded), got %d (%s)", resp.Total, w.Body.String())
	}
	if resp.Data[0].MessageID != visible.ID {
		t.Errorf("expected visible message %d, got %d", visible.ID, resp.Data[0].MessageID)
	}
}

func TestSearchMessages_ThreadFilter(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	t1 := models.Thread{Title: "Thread one"}
	t2 := models.Thread{Title: "Thread two"}
	if err := db.Create(&t1).Error; err != nil {
		t.Fatalf("create thread1: %v", err)
	}
	if err := db.Create(&t2).Error; err != nil {
		t.Fatalf("create thread2: %v", err)
	}
	addMessage(t, db, t1.ID, "user", "shared keyword wallaby")
	addMessage(t, db, t2.ID, "user", "shared keyword wallaby")

	r := searchRouter(db)
	url := fmt.Sprintf("/api/v1/search/messages?q=wallaby&thread_id=%d", t1.ID)
	w := doRequest(r, http.MethodGet, url, "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if resp.Total != 1 {
		t.Fatalf("expected 1 result for thread filter, got %d (%s)", resp.Total, w.Body.String())
	}
	if resp.Data[0].ThreadID != t1.ID {
		t.Errorf("expected thread %d, got %d", t1.ID, resp.Data[0].ThreadID)
	}
}

func TestSearchMessages_LimitAndOffset(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Pagination chat"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	base := time.Now().Add(-time.Hour).UTC()
	for i := 0; i < 5; i++ {
		addMessageWithTime(t, th.ID, "user", "wombat mention", base.Add(time.Duration(i)*time.Minute))
	}

	r := searchRouter(db)

	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=wombat&limit=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	first := decodeMessageSearch(t, w.Body.Bytes())
	if first.Total != 5 {
		t.Errorf("expected total 5, got %d", first.Total)
	}
	if len(first.Data) != 2 {
		t.Fatalf("expected 2 hits with limit=2, got %d", len(first.Data))
	}

	w = doRequest(r, http.MethodGet, "/api/v1/search/messages?q=wombat&limit=2&offset=2", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	second := decodeMessageSearch(t, w.Body.Bytes())
	if second.Total != 5 {
		t.Errorf("expected total 5 on second page, got %d", second.Total)
	}
	if len(second.Data) != 2 {
		t.Fatalf("expected 2 hits with offset=2, got %d", len(second.Data))
	}

	// Pages should not overlap.
	for _, a := range first.Data {
		for _, b := range second.Data {
			if a.MessageID == b.MessageID {
				t.Errorf("overlapping ids between pages: %d", a.MessageID)
			}
		}
	}
}

func TestSearchMessages_LimitCap(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := searchRouter(db)
	// limit=999 should be silently capped to messageSearchMaxLimit (100).
	// We check by parsing without inserting any data — empty result, no error.
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=foo&limit=999", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for clamped limit, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSearchMessages_InvalidLimit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=foo&limit=abc", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for non-numeric limit, got %d", w.Code)
	}
}

func TestSearchMessages_InvalidThreadID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=foo&thread_id=-1", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for negative thread_id, got %d", w.Code)
	}
}

func TestSearchMessages_HTMLSanitized(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "XSS test"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	// ts_headline's text parser strips HTML-shaped tags (<script>...</script>),
	// so we use bare angle brackets and ampersands to verify the in-Go
	// sanitization layer does its part.
	addMessage(t, db, th.ID, "user", "this is a&b < message about injection here")

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=injection", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(resp.Data))
	}
	snip := resp.Data[0].ContentSnippet
	if !strings.Contains(snip, "<mark>injection</mark>") {
		t.Errorf("expected highlighted match, got %q", snip)
	}
	if !strings.Contains(snip, "&amp;") {
		t.Errorf("expected ampersand to be HTML-escaped, got %q", snip)
	}
	if !strings.Contains(snip, "&lt;") {
		t.Errorf("expected stray '<' to be HTML-escaped, got %q", snip)
	}
	// Only <mark> and </mark> are allowed; any other angle bracket must be
	// HTML-escaped.
	stripped := strings.ReplaceAll(snip, "<mark>", "")
	stripped = strings.ReplaceAll(stripped, "</mark>", "")
	if strings.ContainsAny(stripped, "<>") {
		t.Errorf("snippet contains unescaped angle brackets after stripping <mark>: %q", snip)
	}
}

func TestSearchMessages_NoMatches(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Nope"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	addMessage(t, db, th.ID, "user", "nothing relevant here")

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=quokka", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if resp.Data == nil {
		t.Error("expected non-nil empty data slice for JSON shape")
	}
}

func TestSearchMessages_RankOrdering(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	th := models.Thread{Title: "Rank test"}
	if err := db.Create(&th).Error; err != nil {
		t.Fatalf("create thread: %v", err)
	}
	base := time.Now().Add(-time.Hour).UTC()
	// Two matches: one passing mention, one with the term repeated four times.
	low := addMessageWithTime(t, th.ID, "user",
		"a single quokka here only", base)
	high := addMessageWithTime(t, th.ID, "assistant",
		"quokka quokka quokka quokka all about quokkas",
		base.Add(time.Minute))

	r := searchRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/search/messages?q=quokka", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	resp := decodeMessageSearch(t, w.Body.Bytes())
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 hits, got %d", len(resp.Data))
	}
	if resp.Data[0].MessageID != high.ID {
		t.Errorf("expected higher-rank message %d first, got order %d, %d",
			high.ID, resp.Data[0].MessageID, resp.Data[1].MessageID)
	}
	if resp.Data[1].MessageID != low.ID {
		t.Errorf("expected lower-rank message %d second", low.ID)
	}
	if !(resp.Data[0].Rank >= resp.Data[1].Rank) {
		t.Errorf("expected ranks descending, got %v then %v",
			resp.Data[0].Rank, resp.Data[1].Rank)
	}
}

func TestSanitizeHeadline(t *testing.T) {
	in := "before " + headlineStartSentinel + "matched" + headlineEndSentinel +
		" then <b>evil</b> & friends"
	got := sanitizeHeadline(in)
	want := "before <mark>matched</mark> then &lt;b&gt;evil&lt;/b&gt; &amp; friends"
	if got != want {
		t.Errorf("sanitizeHeadline = %q, want %q", got, want)
	}
}
