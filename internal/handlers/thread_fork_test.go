package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"botka/internal/models"
)

func TestThread_Fork_CopiesSettingsAndMessages(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	model := "sonnet"
	src := models.Thread{
		Title:         "original",
		Model:         &model,
		SystemPrompt:  "You are helpful.",
		CustomContext: "ref",
		Color:         "amber",
	}
	if err := db.Create(&src).Error; err != nil {
		t.Fatalf("create source thread: %v", err)
	}

	// Build a 3-message linear chain: m1 (root) -> m2 -> m3, plus an
	// out-of-path branch m2b that must NOT appear in the fork (we fork at m2).
	m1 := models.Message{ThreadID: src.ID, Role: "user", Content: "hi"}
	db.Create(&m1)
	m2 := models.Message{ThreadID: src.ID, Role: "assistant", Content: "hello", ParentID: &m1.ID}
	db.Create(&m2)
	m3 := models.Message{ThreadID: src.ID, Role: "user", Content: "follow-up", ParentID: &m2.ID}
	db.Create(&m3)
	m2b := models.Message{ThreadID: src.ID, Role: "assistant", Content: "alt branch", ParentID: &m1.ID}
	db.Create(&m2b)

	// Attach a file to m2 — the fork must copy the row but reuse StoredName.
	att := models.Attachment{
		MessageID:    m2.ID,
		StoredName:   "shared.png",
		OriginalName: "image.png",
		MimeType:     "image/png",
		Size:         42,
	}
	db.Create(&att)

	// Source has a URL source — should be copied.
	db.Create(&models.ThreadSource{ThreadID: src.ID, URL: "https://example.com", Label: "ex", Position: 0})

	// Set claude_session_id on source — must NOT be copied.
	sessID := "sess-123"
	db.Model(&src).Update("claude_session_id", sessID)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"from_message_id":%d}`, m2.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("missing data in response: %s", w.Body.String())
	}
	newID := int64(data["id"].(float64))
	if newID == src.ID {
		t.Fatal("fork returned source thread id")
	}

	// Default title is "<source> (fork)".
	var newThread models.Thread
	if err := db.First(&newThread, newID).Error; err != nil {
		t.Fatalf("load new thread: %v", err)
	}
	if newThread.Title != "original (fork)" {
		t.Errorf("title: got %q want %q", newThread.Title, "original (fork)")
	}
	if newThread.SystemPrompt != src.SystemPrompt {
		t.Errorf("system_prompt not copied")
	}
	if newThread.CustomContext != src.CustomContext {
		t.Errorf("custom_context not copied")
	}
	if newThread.Color != src.Color {
		t.Errorf("color not copied")
	}
	if newThread.ClaudeSessionID != nil {
		t.Errorf("claude_session_id should NOT be copied, got %v", *newThread.ClaudeSessionID)
	}
	if newThread.ParentThreadID == nil || *newThread.ParentThreadID != src.ID {
		t.Errorf("parent_thread_id wrong: %v", newThread.ParentThreadID)
	}
	if newThread.ForkedFromMessageID == nil || *newThread.ForkedFromMessageID != m2.ID {
		t.Errorf("forked_from_message_id wrong: %v", newThread.ForkedFromMessageID)
	}

	// Exactly two messages copied (m1, m2). m3 is after the fork point; m2b is
	// on a different branch.
	var newMsgs []models.Message
	db.Where("thread_id = ?", newID).Order("created_at ASC").Find(&newMsgs)
	if len(newMsgs) != 2 {
		t.Fatalf("expected 2 messages in fork, got %d", len(newMsgs))
	}
	if newMsgs[0].Content != "hi" || newMsgs[1].Content != "hello" {
		t.Errorf("unexpected message contents: %q, %q", newMsgs[0].Content, newMsgs[1].Content)
	}
	if newMsgs[1].ParentID == nil || *newMsgs[1].ParentID != newMsgs[0].ID {
		t.Errorf("parent chain not rebuilt: %+v", newMsgs[1].ParentID)
	}

	// Attachment copied by reference (same StoredName).
	var newAtts []models.Attachment
	db.Where("message_id = ?", newMsgs[1].ID).Find(&newAtts)
	if len(newAtts) != 1 {
		t.Fatalf("expected 1 attachment in fork, got %d", len(newAtts))
	}
	if newAtts[0].StoredName != "shared.png" {
		t.Errorf("attachment StoredName not preserved: %q", newAtts[0].StoredName)
	}
	if newAtts[0].ID == att.ID {
		t.Errorf("expected a new attachment row, got the original id")
	}

	// URL source copied.
	var newSources []models.ThreadSource
	db.Where("thread_id = ?", newID).Find(&newSources)
	if len(newSources) != 1 || newSources[0].URL != "https://example.com" {
		t.Errorf("URL source not copied: %+v", newSources)
	}
}

func TestThread_Fork_DefaultsAndCustomTitle(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	src := createTestThread(t, db)
	m1 := models.Message{ThreadID: src.ID, Role: "user", Content: "x"}
	db.Create(&m1)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"from_message_id":%d,"new_title":"My branch"}`, m1.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["title"] != "My branch" {
		t.Errorf("custom title not honored: %v", data["title"])
	}
}

func TestThread_Fork_TagsCopied(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	src := createTestThread(t, db)
	tag := models.Tag{Name: "research"}
	db.Create(&tag)
	db.Exec("INSERT INTO thread_tags (thread_id, tag_id) VALUES (?, ?)", src.ID, tag.ID)
	m1 := models.Message{ThreadID: src.ID, Role: "user", Content: "x"}
	db.Create(&m1)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"from_message_id":%d}`, m1.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("status: %d body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	newID := int64(data["id"].(float64))

	var count int64
	db.Raw("SELECT COUNT(*) FROM thread_tags WHERE thread_id = ?", newID).Scan(&count)
	if count != 1 {
		t.Errorf("expected 1 tag assignment on fork, got %d", count)
	}
}

func TestThread_Fork_RejectsForeignMessage(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	src := createTestThread(t, db)
	other := createTestThread(t, db)
	stranger := models.Message{ThreadID: other.ID, Role: "user", Content: "elsewhere"}
	db.Create(&stranger)

	r := threadRouter(db)
	body := fmt.Sprintf(`{"from_message_id":%d}`, stranger.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for foreign message, got %d: %s", w.Code, w.Body.String())
	}
}

func TestThread_Fork_RejectsMissingFromMessageID(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	src := createTestThread(t, db)
	r := threadRouter(db)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), `{}`)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing from_message_id, got %d", w.Code)
	}
}

func TestThread_Fork_RejectsTooManyMessages(t *testing.T) {
	if testing.Short() {
		t.Skip("skipped in short mode")
	}
	db := setupTestDB(t)
	cleanTables(t, db)

	src := createTestThread(t, db)
	// Create maxForkMessageCount+1 messages in a chain.
	var prev *int64
	var lastID int64
	for i := 0; i <= maxForkMessageCount; i++ {
		m := models.Message{ThreadID: src.ID, Role: "user", Content: fmt.Sprintf("m%d", i)}
		if prev != nil {
			p := *prev
			m.ParentID = &p
		}
		if err := db.Create(&m).Error; err != nil {
			t.Fatalf("create message %d: %v", i, err)
		}
		lastID = m.ID
		id := m.ID
		prev = &id
	}

	r := threadRouter(db)
	body := fmt.Sprintf(`{"from_message_id":%d}`, lastID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), body)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for too-many-messages, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "limit") {
		t.Errorf("error should mention the limit, got %s", w.Body.String())
	}
}

func TestThread_ListForks(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	src := createTestThread(t, db)
	m1 := models.Message{ThreadID: src.ID, Role: "user", Content: "x"}
	db.Create(&m1)

	r := threadRouter(db)
	for i := 0; i < 2; i++ {
		body := fmt.Sprintf(`{"from_message_id":%d}`, m1.ID)
		w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/fork", src.ID), body)
		if w.Code != http.StatusCreated {
			t.Fatalf("fork %d: status %d: %s", i, w.Code, w.Body.String())
		}
	}

	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/forks", src.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("list forks: status %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if int(resp["total"].(float64)) != 2 {
		t.Errorf("expected total=2, got %v", resp["total"])
	}
}
