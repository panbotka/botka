package handlers

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/textproto"
	"strings"
	"testing"
)

// buildFileHeader constructs a real *multipart.FileHeader backed by a parsed
// multipart form, so that subsequent fh.Open() calls work normally.
func buildFileHeader(t *testing.T, filename, contentType string, data []byte) *multipart.FileHeader {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="file"; filename=%q`, filename))
	h.Set("Content-Type", contentType)
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := part.Write(data); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	r := multipart.NewReader(&buf, w.Boundary())
	form, err := r.ReadForm(32 << 20)
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	files := form.File["file"]
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}
	return files[0]
}

// TestSaveUploadedFile_SizeLimit exercises the upload size boundary by
// temporarily shrinking maxUploadSize. It verifies that a file just over the
// limit is rejected and a file just under the limit passes the size check.
func TestSaveUploadedFile_SizeLimit(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	msg := createMessage(t, db, thread.ID, nil, "user", "test")

	origLimit := maxUploadSize
	maxUploadSize = 100
	t.Cleanup(func() { maxUploadSize = origLimit })

	h := &ChatHandler{db: db, uploadDir: t.TempDir()}

	over := buildFileHeader(t, "big.png", "image/png", bytes.Repeat([]byte("a"), 101))
	if _, err := h.saveUploadedFile(msg.ID, over); err == nil {
		t.Fatal("expected error for oversized file, got nil")
	} else if !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected 'too large' error, got: %v", err)
	}

	under := buildFileHeader(t, "small.png", "image/png", bytes.Repeat([]byte("b"), 99))
	att, err := h.saveUploadedFile(msg.ID, under)
	if err != nil {
		t.Fatalf("expected success for under-limit file, got: %v", err)
	}
	if att == nil || att.Size != 99 {
		t.Fatalf("expected 99-byte attachment, got %+v", att)
	}
}

// TestSaveUploadedFile_ProductionLimit pins the production constant to its
// configured value. If the limit is ever changed, this test must be updated
// alongside the frontend constant in ChatInput.tsx to keep them in sync.
func TestSaveUploadedFile_ProductionLimit(t *testing.T) {
	const expected int64 = 500 << 20 // 500 MB
	if maxUploadSize != expected {
		t.Errorf("maxUploadSize = %d, want %d (keep in sync with frontend MAX_FILE_SIZE)", maxUploadSize, expected)
	}
}
