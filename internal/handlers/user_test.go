package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// userRouter creates a test router with an admin user injected into context,
// bypassing the AdminOnly middleware that RegisterUserRoutes normally applies.
func userRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		admin := &models.User{ID: 999, Username: "admin", Role: models.RoleAdmin}
		c.Set("user", admin)
		c.Next()
	})
	h := NewUserHandler(db)
	v1 := r.Group("/api/v1")
	RegisterUserRoutes(v1, h)
	return r
}

func TestUser_Create_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/users", `{"username":"testuser","password":"password123"}`)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			ID       int64           `json:"id"`
			Username string          `json:"username"`
			Role     models.UserRole `json:"role"`
		} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Data.Username != "testuser" {
		t.Errorf("expected username=testuser, got %s", resp.Data.Username)
	}
	if resp.Data.Role != models.RoleExternal {
		t.Errorf("expected role=external, got %s", resp.Data.Role)
	}
}

func TestUser_Create_MissingFields(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/users", `{}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_Create_ShortPassword(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)
	w := doRequest(r, http.MethodPost, "/api/v1/users", `{"username":"testuser","password":"short"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_Create_DuplicateUsername(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)

	// Create first user.
	w := doRequest(r, http.MethodPost, "/api/v1/users", `{"username":"dupuser","password":"password123"}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("first create: expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Create duplicate.
	w = doRequest(r, http.MethodPost, "/api/v1/users", `{"username":"dupuser","password":"password456"}`)
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_List_Empty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/users", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []interface{} `json:"data"`
		Total float64       `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 0 {
		t.Errorf("expected empty list, got %d items", len(resp.Data))
	}
}

func TestUser_List_WithUsers(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create an admin user and an external user directly in the DB.
	adminUser := models.User{Username: "admin1", PasswordHash: "hash", Role: models.RoleAdmin}
	db.Create(&adminUser)

	extUser := models.User{Username: "ext1", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	// Grant the external user access to a thread to verify thread_count.
	th := createTestThread(t, db)
	db.Create(&models.ThreadAccess{UserID: extUser.ID, ThreadID: th.ID})

	r := userRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/users", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []map[string]interface{} `json:"data"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 2 {
		t.Fatalf("expected 2 users, got %d", len(resp.Data))
	}

	// Find the external user and check thread_count.
	for _, u := range resp.Data {
		if u["username"] == "ext1" {
			tc := u["thread_count"].(float64)
			if tc != 1 {
				t.Errorf("expected thread_count=1 for ext1, got %.0f", tc)
			}
		}
	}
}

func TestUser_Delete_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create an external user to delete.
	extUser := models.User{Username: "deleteMe", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", extUser.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify user is gone.
	var check models.User
	if err := db.First(&check, extUser.ID).Error; err == nil {
		t.Error("expected user to be deleted")
	}
}

func TestUser_Delete_Self(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// The userRouter injects admin with ID=999 into context.
	// Trying to delete ID 999 should be rejected.
	r := userRouter(db)
	w := doRequest(r, http.MethodDelete, "/api/v1/users/999", "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_Delete_AdminUser(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	// Create another admin user.
	adminUser := models.User{Username: "otheradmin", PasswordHash: "hash", Role: models.RoleAdmin}
	db.Create(&adminUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d", adminUser.ID), "")

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_Delete_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)
	w := doRequest(r, http.MethodDelete, "/api/v1/users/99999", "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_ResetPassword_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "resetme", PasswordHash: "oldhash", Role: models.RoleExternal}
	db.Create(&extUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/users/%d/password", extUser.ID), `{"password":"newpassword123"}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify password hash changed.
	var updated models.User
	db.First(&updated, extUser.ID)
	if updated.PasswordHash == "oldhash" {
		t.Error("expected password hash to be updated")
	}
}

func TestUser_ResetPassword_ShortPassword(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "resetshort", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/users/%d/password", extUser.ID), `{"password":"short"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_ResetPassword_AdminUser(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	adminUser := models.User{Username: "adminreset", PasswordHash: "hash", Role: models.RoleAdmin}
	db.Create(&adminUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/users/%d/password", adminUser.ID), `{"password":"newpassword123"}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_ResetPassword_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := userRouter(db)
	w := doRequest(r, http.MethodPut, "/api/v1/users/99999/password", `{"password":"newpassword123"}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_GrantThread_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "grantuser", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	th := createTestThread(t, db)

	r := userRouter(db)
	body := fmt.Sprintf(`{"thread_id":%d}`, th.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/threads", extUser.ID), body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	// Verify access record exists.
	var count int64
	db.Model(&models.ThreadAccess{}).Where("user_id = ? AND thread_id = ?", extUser.ID, th.ID).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 access record, got %d", count)
	}
}

func TestUser_GrantThread_NotExternal(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	adminUser := models.User{Username: "grantadmin", PasswordHash: "hash", Role: models.RoleAdmin}
	db.Create(&adminUser)

	th := createTestThread(t, db)

	r := userRouter(db)
	body := fmt.Sprintf(`{"thread_id":%d}`, th.ID)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/threads", adminUser.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_GrantThread_ThreadNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "grantbad", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/users/%d/threads", extUser.ID), `{"thread_id":99999}`)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_RevokeThread_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "revokeuser", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	th := createTestThread(t, db)
	db.Create(&models.ThreadAccess{UserID: extUser.ID, ThreadID: th.ID})

	r := userRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d/threads/%d", extUser.ID, th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify access record is gone.
	var count int64
	db.Model(&models.ThreadAccess{}).Where("user_id = ? AND thread_id = ?", extUser.ID, th.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 access records, got %d", count)
	}
}

func TestUser_RevokeThread_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "revokenotfound", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	r := userRouter(db)
	w := doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/users/%d/threads/99999", extUser.ID), "")

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestUser_ListThreads(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	extUser := models.User{Username: "listthreads", PasswordHash: "hash", Role: models.RoleExternal}
	db.Create(&extUser)

	th := createTestThread(t, db)
	db.Create(&models.ThreadAccess{UserID: extUser.ID, ThreadID: th.ID})

	r := userRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/users/%d/threads", extUser.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data  []map[string]interface{} `json:"data"`
		Total float64                  `json:"total"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 thread, got %d", len(resp.Data))
	}
	if resp.Data[0]["thread_title"] != "test thread" {
		t.Errorf("expected thread_title=test thread, got %v", resp.Data[0]["thread_title"])
	}
}
