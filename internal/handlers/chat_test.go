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
