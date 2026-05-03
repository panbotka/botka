package handlers

import (
	"testing"

	"botka/internal/models"

	"gorm.io/gorm"
)

func TestGetActivePath_EmptyThread(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	messages, forkPoints, err := getActivePath(db, thread.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(messages))
	}
	if len(forkPoints) != 0 {
		t.Errorf("expected 0 fork points, got %d", len(forkPoints))
	}
}

func TestGetActivePath_LinearThread(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)

	msg1 := createMessage(t, db, thread.ID, nil, "user", "hello")
	msg2 := createMessage(t, db, thread.ID, &msg1.ID, "assistant", "hi there")
	msg3 := createMessage(t, db, thread.ID, &msg2.ID, "user", "how are you")

	messages, forkPoints, err := getActivePath(db, thread.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(messages))
	}
	if messages[0].ID != msg1.ID || messages[1].ID != msg2.ID || messages[2].ID != msg3.ID {
		t.Error("messages not in expected order")
	}
	if len(forkPoints) != 0 {
		t.Errorf("expected 0 fork points, got %d", len(forkPoints))
	}
}

func TestGetActivePath_ForkWithSelection(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)

	// Create a fork: msg1 has two children (msg2a and msg2b)
	msg1 := createMessage(t, db, thread.ID, nil, "user", "hello")
	msg2a := createMessage(t, db, thread.ID, &msg1.ID, "assistant", "response A")
	msg2b := createMessage(t, db, thread.ID, &msg1.ID, "assistant", "response B")

	// Select msg2a as the active branch
	db.Create(&models.BranchSelection{
		ThreadID:        thread.ID,
		ForkMessageID:   msg1.ID,
		SelectedChildID: msg2a.ID,
	})

	messages, forkPoints, err := getActivePath(db, thread.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages in active path, got %d", len(messages))
	}
	if messages[0].ID != msg1.ID {
		t.Errorf("expected first message to be msg1")
	}
	if messages[1].ID != msg2a.ID {
		t.Errorf("expected second message to be msg2a (selected branch), got msg %d", messages[1].ID)
	}

	// Verify fork point exists
	fp, ok := forkPoints[msg1.ID]
	if !ok {
		t.Fatal("expected fork point at msg1")
	}
	if len(fp.Children) != 2 {
		t.Errorf("expected 2 children at fork point, got %d", len(fp.Children))
	}

	_ = msg2b // used in fork
}

func TestGetActivePath_DefaultsToLastChild(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)

	msg1 := createMessage(t, db, thread.ID, nil, "user", "hello")
	createMessage(t, db, thread.ID, &msg1.ID, "assistant", "response A")
	msg2b := createMessage(t, db, thread.ID, &msg1.ID, "assistant", "response B")

	// No branch selection — should default to last child
	messages, _, err := getActivePath(db, thread.ID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(messages))
	}
	if messages[1].ID != msg2b.ID {
		t.Errorf("expected default to last child (msg2b), got msg %d", messages[1].ID)
	}
}

// createMessage is a test helper to create a message in the database.
func createMessage(t *testing.T, db *gorm.DB, threadID int64, parentID *int64, role, content string) models.Message {
	t.Helper()
	msg := models.Message{
		ThreadID: threadID,
		ParentID: parentID,
		Role:     role,
		Content:  content,
	}
	if err := db.Create(&msg).Error; err != nil {
		t.Fatalf("create message: %v", err)
	}
	return msg
}

func TestSoftDeleteMessageBranch(t *testing.T) {
	// Regenerate uses softDeleteMessageBranch to clear the last assistant
	// message and its descendants. Verify that:
	//   - the assistant message + its descendants get deleted_at set;
	//   - the user message that came before is untouched;
	//   - attachments tied to the deleted messages are also soft-deleted;
	//   - GORM Find auto-filters the soft-deleted rows;
	//   - .Unscoped() still sees them with deleted_at populated.
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	user := createMessage(t, db, thread.ID, nil, "user", "hi")
	assistant := createMessage(t, db, thread.ID, &user.ID, "assistant", "hello")
	follow := createMessage(t, db, thread.ID, &assistant.ID, "user", "thanks")
	att := models.Attachment{
		MessageID:    assistant.ID,
		StoredName:   "stored.png",
		OriginalName: "orig.png",
		MimeType:     "image/png",
		Size:         1,
	}
	db.Create(&att)

	if err := softDeleteMessageBranch(db, thread.ID, assistant.ID); err != nil {
		t.Fatalf("softDeleteMessageBranch: %v", err)
	}

	// Live (auto-filtered) view: only the original user message survives.
	var live []models.Message
	db.Where("thread_id = ?", thread.ID).Order("id ASC").Find(&live)
	if len(live) != 1 || live[0].ID != user.ID {
		t.Fatalf("expected only the user message live, got %d (ids %v)", len(live), live)
	}

	// Unscoped: assistant + descendant follow-up still in DB with deleted_at set.
	var all []models.Message
	db.Unscoped().Where("thread_id = ?", thread.ID).Order("id ASC").Find(&all)
	if len(all) != 3 {
		t.Fatalf("expected 3 rows in DB, got %d", len(all))
	}
	for _, m := range all {
		switch m.ID {
		case user.ID:
			if m.DeletedAt.Valid {
				t.Errorf("user message should not be soft-deleted")
			}
		case assistant.ID, follow.ID:
			if !m.DeletedAt.Valid {
				t.Errorf("message %d should have deleted_at set", m.ID)
			}
		default:
			t.Errorf("unexpected message id %d", m.ID)
		}
	}

	// Attachment on the deleted assistant message is soft-deleted too.
	var liveAtt []models.Attachment
	db.Where("message_id = ?", assistant.ID).Find(&liveAtt)
	if len(liveAtt) != 0 {
		t.Errorf("expected attachment hidden after branch soft-delete, got %d", len(liveAtt))
	}
	var allAtt []models.Attachment
	db.Unscoped().Where("message_id = ?", assistant.ID).Find(&allAtt)
	if len(allAtt) != 1 || !allAtt[0].DeletedAt.Valid {
		t.Errorf("expected attachment still present with deleted_at set, got %d", len(allAtt))
	}

	// getActivePath now skips the soft-deleted branch.
	path, _, err := getActivePath(db, thread.ID)
	if err != nil {
		t.Fatalf("getActivePath: %v", err)
	}
	if len(path) != 1 || path[0].ID != user.ID {
		t.Fatalf("expected getActivePath to return only the user message, got %d entries", len(path))
	}
}

func TestSoftDeleteMessageBranch_Idempotent(t *testing.T) {
	// Calling Regenerate's soft-delete twice on the same branch must not error
	// and must leave the deleted_at timestamps untouched on the second call.
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := createTestThread(t, db)
	user := createMessage(t, db, thread.ID, nil, "user", "hi")
	assistant := createMessage(t, db, thread.ID, &user.ID, "assistant", "hello")

	if err := softDeleteMessageBranch(db, thread.ID, assistant.ID); err != nil {
		t.Fatalf("first soft-delete: %v", err)
	}
	var first models.Message
	if err := db.Unscoped().First(&first, assistant.ID).Error; err != nil {
		t.Fatalf("load deleted assistant: %v", err)
	}
	firstAt := first.DeletedAt

	if err := softDeleteMessageBranch(db, thread.ID, assistant.ID); err != nil {
		t.Fatalf("second soft-delete: %v", err)
	}
	var second models.Message
	if err := db.Unscoped().First(&second, assistant.ID).Error; err != nil {
		t.Fatalf("reload deleted assistant: %v", err)
	}
	if !second.DeletedAt.Valid || second.DeletedAt.Time != firstAt.Time {
		t.Errorf("deleted_at should not change on a second soft-delete; was %v, now %v", firstAt.Time, second.DeletedAt.Time)
	}
}

func TestIsTransientError(t *testing.T) {
	tests := []struct {
		msg  string
		want bool
	}{
		{"", true},
		{"Error in input stream", true},
		{"error in input stream", true},
		{"ERROR IN INPUT STREAM", true},
		{"something: Error in input stream", true},
		{"claude process exited unexpectedly", true},
		{"claude process exited unexpectedly (exit code 1)", true},
		{"claude process exited unexpectedly (killed by signal killed (possible OOM kill))", true},
		{"No conversation found", false},
		{"rate limit exceeded", false},
		{"some other error", false},
	}
	for _, tc := range tests {
		if got := isTransientError(tc.msg); got != tc.want {
			t.Errorf("isTransientError(%q) = %v, want %v", tc.msg, got, tc.want)
		}
	}
}
