package handlers

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

func fileRouter(db *gorm.DB, uploadDir string) *gin.Engine {
	r := gin.New()
	h := NewFileHandler(db, uploadDir)
	v1 := r.Group("/api/v1")
	RegisterFileRoutes(v1, h)
	return r
}

func createTestAttachment(t *testing.T, db *gorm.DB, uploadDir string) models.Attachment {
	t.Helper()

	thread := createTestThread(t, db)
	msg := models.Message{ThreadID: thread.ID, Role: "user", Content: "test message"}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create test message: %v", err)
	}

	attachment := models.Attachment{
		MessageID:    msg.ID,
		StoredName:   "stored-file.txt",
		OriginalName: "my file.txt",
		MimeType:     "text/plain",
		Size:         5,
	}
	if err := db.Create(&attachment).Error; err != nil {
		t.Fatalf("create test attachment: %v", err)
	}

	if err := os.WriteFile(filepath.Join(uploadDir, "stored-file.txt"), []byte("hello"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	return attachment
}

func TestFile_ServeFileSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	uploadDir := t.TempDir()
	att := createTestAttachment(t, db, uploadDir)

	r := fileRouter(db, uploadDir)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/files/%d", att.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "text/plain" {
		t.Errorf("expected Content-Type=text/plain, got %s", ct)
	}

	cd := w.Header().Get("Content-Disposition")
	if cd != `inline; filename="my file.txt"` {
		t.Errorf("expected inline disposition, got %s", cd)
	}

	if w.Body.String() != "hello" {
		t.Errorf("expected body=hello, got %s", w.Body.String())
	}
}

func TestFile_ServeFileNotInDB(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	uploadDir := t.TempDir()

	r := fileRouter(db, uploadDir)
	w := doRequest(r, http.MethodGet, "/api/v1/files/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFile_ServeFileNotOnDisk(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	uploadDir := t.TempDir()

	// Create attachment in DB but do NOT write the file to disk
	thread := createTestThread(t, db)
	msg := models.Message{ThreadID: thread.ID, Role: "user", Content: "test"}
	db.Create(&msg)
	att := models.Attachment{
		MessageID:    msg.ID,
		StoredName:   "missing-file.txt",
		OriginalName: "missing.txt",
		MimeType:     "text/plain",
		Size:         5,
	}
	db.Create(&att)

	r := fileRouter(db, uploadDir)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/files/%d", att.ID), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestFile_DownloadFileSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	uploadDir := t.TempDir()
	att := createTestAttachment(t, db, uploadDir)

	r := fileRouter(db, uploadDir)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/files/%d/download", att.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/octet-stream" {
		t.Errorf("expected Content-Type=application/octet-stream, got %s", ct)
	}

	cd := w.Header().Get("Content-Disposition")
	if cd != `attachment; filename="my file.txt"` {
		t.Errorf("expected attachment disposition, got %s", cd)
	}
}

func TestFile_SanitizeFilenameRemovesQuotes(t *testing.T) {
	result := sanitizeFilename(`my"file"name.txt`)
	expected := "myfilename.txt"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFile_SanitizeFilenameRemovesBackslashes(t *testing.T) {
	result := sanitizeFilename(`my\file\name.txt`)
	expected := "myfilename.txt"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}
