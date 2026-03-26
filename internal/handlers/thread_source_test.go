package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func threadSourceRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewThreadSourceHandler(db)
	v1 := r.Group("/api/v1")
	RegisterThreadSourceRoutes(v1, h)
	return r
}

func TestThreadSource_ListEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	w := doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].([]interface{})
	if len(data) != 0 {
		t.Errorf("expected empty list, got %d items", len(data))
	}
}

func TestThreadSource_CreateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"url":"https://example.com/docs","label":"Example Docs"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["url"] != "https://example.com/docs" {
		t.Errorf("expected url, got %v", data["url"])
	}
	if data["label"] != "Example Docs" {
		t.Errorf("expected label, got %v", data["label"])
	}
}

func TestThreadSource_CreateMissingURL(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"label":"No URL"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestThreadSource_UpdateSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	// Create first
	body := `{"url":"https://old.com","label":"Old"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	sourceID := int64(createResp["data"].(map[string]interface{})["id"].(float64))

	// Update
	body = `{"url":"https://new.com","label":"New"}`
	w = doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/sources/%d", th.ID, sourceID), body)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	data := resp["data"].(map[string]interface{})
	if data["url"] != "https://new.com" {
		t.Errorf("expected updated url, got %v", data["url"])
	}
}

func TestThreadSource_UpdateNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"url":"https://test.com","label":"Test"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/sources/99999", th.ID), body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestThreadSource_DeleteSuccess(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	body := `{"url":"https://delete.me","label":"Bye"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	sourceID := int64(createResp["data"].(map[string]interface{})["id"].(float64))

	w = doRequest(r, http.MethodDelete, fmt.Sprintf("/api/v1/threads/%d/sources/%d", th.ID, sourceID), "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestThreadSource_Reorder(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)
	// Create 3 sources
	var ids []int64
	for _, u := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		body := fmt.Sprintf(`{"url":"%s"}`, u)
		w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
		if w.Code != http.StatusCreated {
			t.Fatalf("create failed: %d", w.Code)
		}
		var resp map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &resp)
		ids = append(ids, int64(resp["data"].(map[string]interface{})["id"].(float64)))
	}

	// Reorder: reverse
	reorderBody := fmt.Sprintf(`{"ids":[%d,%d,%d]}`, ids[2], ids[0], ids[1])
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/sources/reorder", th.ID), reorderBody)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// List and verify order
	w = doRequest(r, http.MethodGet, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), "")
	var listResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	data := listResp["data"].([]interface{})
	firstURL := data[0].(map[string]interface{})["url"].(string)
	if firstURL != "https://c.com" {
		t.Errorf("expected first source to be c.com after reorder, got %s", firstURL)
	}
}

func TestThreadSource_AutoPosition(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	th := createTestThread(t, db)

	r := threadSourceRouter(db)

	// Create two sources and verify positions auto-increment
	body := `{"url":"https://first.com"}`
	w := doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var resp1 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp1)
	pos1 := int(resp1["data"].(map[string]interface{})["position"].(float64))

	body = `{"url":"https://second.com"}`
	w = doRequest(r, http.MethodPost, fmt.Sprintf("/api/v1/threads/%d/sources", th.ID), body)
	if w.Code != http.StatusCreated {
		t.Fatalf("create failed: %d", w.Code)
	}
	var resp2 map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp2)
	pos2 := int(resp2["data"].(map[string]interface{})["position"].(float64))

	if pos2 <= pos1 {
		t.Errorf("expected second position (%d) > first position (%d)", pos2, pos1)
	}
}
