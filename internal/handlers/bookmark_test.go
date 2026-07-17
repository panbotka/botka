package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"botka/internal/models"
)

func newBookmarkRouter(h *BookmarkHandler) *gin.Engine {
	r := gin.New()
	v1 := r.Group("/api/v1")
	RegisterBookmarkRoutes(v1, h)
	return r
}

func TestBookmarkHandler_CreateAndList(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	router := newBookmarkRouter(NewBookmarkHandler(db))

	// A page whose title and favicon we can extract.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = io.WriteString(w, `<html><head><title>Example</title><link rel="icon" href="/fav.png"></head><body>x</body></html>`)
	}))
	defer srv.Close()

	w := doRequest(router, http.MethodPost, "/api/v1/bookmarks", fmt.Sprintf(`{"url":%q}`, srv.URL))
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		Data models.Bookmark `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	if created.Data.Title != "Example" {
		t.Errorf("title = %q, want Example", created.Data.Title)
	}
	if want := srv.URL + "/fav.png"; created.Data.FaviconURL != want {
		t.Errorf("favicon = %q, want %q", created.Data.FaviconURL, want)
	}
	if created.Data.URL != srv.URL {
		t.Errorf("url = %q, want %q", created.Data.URL, srv.URL)
	}

	// List returns the created bookmark.
	w = doRequest(router, http.MethodGet, "/api/v1/bookmarks", "")
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d", w.Code)
	}
	var list struct {
		Data []models.Bookmark `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(list.Data) != 1 {
		t.Fatalf("list len = %d, want 1", len(list.Data))
	}
}

func TestBookmarkHandler_CreateNormalizesAndFallsBack(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	router := newBookmarkRouter(NewBookmarkHandler(db))

	// A bare, unreachable host: create must still succeed with a normalized URL,
	// hostname title, and a default favicon.
	w := doRequest(router, http.MethodPost, "/api/v1/bookmarks", `{"url":"nonexistent.invalid"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", w.Code, w.Body.String())
	}
	var created struct {
		Data models.Bookmark `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if created.Data.URL != "https://nonexistent.invalid" {
		t.Errorf("url = %q, want https://nonexistent.invalid", created.Data.URL)
	}
	if created.Data.Title != "nonexistent.invalid" {
		t.Errorf("title = %q, want hostname fallback", created.Data.Title)
	}
	if created.Data.FaviconURL != "https://nonexistent.invalid/favicon.ico" {
		t.Errorf("favicon = %q, want /favicon.ico fallback", created.Data.FaviconURL)
	}
}

func TestBookmarkHandler_CreateRejectsEmptyURL(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	router := newBookmarkRouter(NewBookmarkHandler(db))

	w := doRequest(router, http.MethodPost, "/api/v1/bookmarks", `{"url":"   "}`)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestBookmarkHandler_ListOrdersBySortOrder(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	router := newBookmarkRouter(NewBookmarkHandler(db))

	// Insert out of order; List must return sorted by sort_order.
	db.Create(&models.Bookmark{URL: "https://b.example", Title: "B", SortOrder: 2})
	db.Create(&models.Bookmark{URL: "https://a.example", Title: "A", SortOrder: 1})

	w := doRequest(router, http.MethodGet, "/api/v1/bookmarks", "")
	var list struct {
		Data []models.Bookmark `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(list.Data) != 2 || list.Data[0].Title != "A" || list.Data[1].Title != "B" {
		t.Fatalf("unexpected order: %+v", list.Data)
	}
}

func TestBookmarkHandler_Delete(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	router := newBookmarkRouter(NewBookmarkHandler(db))

	bm := models.Bookmark{URL: "https://x.example", Title: "X"}
	if err := db.Create(&bm).Error; err != nil {
		t.Fatalf("create: %v", err)
	}

	w := doRequest(router, http.MethodDelete, fmt.Sprintf("/api/v1/bookmarks/%d", bm.ID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("delete status = %d", w.Code)
	}

	var count int64
	db.Model(&models.Bookmark{}).Count(&count)
	if count != 0 {
		t.Errorf("count after delete = %d, want 0", count)
	}

	// Deleting again yields 404.
	w = doRequest(router, http.MethodDelete, fmt.Sprintf("/api/v1/bookmarks/%d", bm.ID), "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("second delete status = %d, want 404", w.Code)
	}
}
