package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// maxFolderDepth caps the nesting depth of thread folders. Root folders count
// as depth 1, so 5 means at most a folder five levels below the root.
const maxFolderDepth = 5

// FolderHandler handles HTTP requests for thread folder resources.
type FolderHandler struct {
	db *gorm.DB
}

// NewFolderHandler creates a new FolderHandler with the given database connection.
func NewFolderHandler(db *gorm.DB) *FolderHandler {
	return &FolderHandler{db: db}
}

// RegisterFolderRoutes attaches folder endpoints to the given router group.
func RegisterFolderRoutes(rg *gin.RouterGroup, h *FolderHandler) {
	rg.GET("/folders", h.List)
	rg.POST("/folders", h.Create)
	rg.PATCH("/folders/:id", h.Update)
	rg.DELETE("/folders/:id", h.Delete)
}

// folderNode is the API shape for a folder entry in the tree response.
type folderNode struct {
	ID          int64        `json:"id"`
	Name        string       `json:"name"`
	ParentID    *int64       `json:"parent_id"`
	Position    int          `json:"position"`
	ThreadCount int          `json:"thread_count"`
	Children    []folderNode `json:"children"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// List returns the full folder tree along with the per-folder thread count
// (direct children only, matching user intuition for the badge).
func (h *FolderHandler) List(c *gin.Context) {
	var folders []models.ThreadFolder
	if err := h.db.Order("position ASC, id ASC").Find(&folders).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list folders")
		return
	}

	type countRow struct {
		FolderID int64
		Count    int
	}
	var counts []countRow
	h.db.Raw(`SELECT folder_id, COUNT(*) AS count FROM threads
		WHERE folder_id IS NOT NULL GROUP BY folder_id`).Scan(&counts)
	countByFolder := make(map[int64]int, len(counts))
	for _, r := range counts {
		countByFolder[r.FolderID] = r.Count
	}

	// Recursively build the tree. The source query is ordered by position so
	// children are attached in display order.
	var build func(parent *int64) []folderNode
	build = func(parent *int64) []folderNode {
		out := []folderNode{}
		for _, f := range folders {
			matches := (parent == nil && f.ParentID == nil) ||
				(parent != nil && f.ParentID != nil && *f.ParentID == *parent)
			if !matches {
				continue
			}
			out = append(out, folderNode{
				ID:          f.ID,
				Name:        f.Name,
				ParentID:    f.ParentID,
				Position:    f.Position,
				ThreadCount: countByFolder[f.ID],
				Children:    build(&f.ID),
				CreatedAt:   f.CreatedAt,
				UpdatedAt:   f.UpdatedAt,
			})
		}
		return out
	}

	respondOK(c, build(nil))
}

type createFolderRequest struct {
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
}

// Create makes a new folder, optionally nested under ParentID.
func (h *FolderHandler) Create(c *gin.Context) {
	var req createFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	name := strings.TrimSpace(req.Name)
	if name == "" || len(name) > 255 {
		respondError(c, http.StatusBadRequest, "name is required (max 255 chars)")
		return
	}

	if req.ParentID != nil {
		var parent models.ThreadFolder
		if err := h.db.First(&parent, *req.ParentID).Error; err != nil {
			respondError(c, http.StatusBadRequest, "parent folder not found")
			return
		}
		depth, err := folderDepth(h.db, *req.ParentID)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "failed to compute depth")
			return
		}
		if depth+1 > maxFolderDepth {
			respondError(c, http.StatusBadRequest, "maximum folder nesting depth exceeded")
			return
		}
	}

	var maxPos *int
	h.db.Raw(`SELECT MAX(position) FROM thread_folders WHERE parent_id IS NOT DISTINCT FROM ?`,
		req.ParentID).Scan(&maxPos)
	pos := 0
	if maxPos != nil {
		pos = *maxPos + 1
	}

	folder := models.ThreadFolder{
		Name:     name,
		ParentID: req.ParentID,
		Position: pos,
	}
	if err := h.db.Create(&folder).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to create folder")
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": folder})
}

type updateFolderRequest struct {
	Name     *string `json:"name,omitempty"`
	ParentID *int64  `json:"parent_id,omitempty"`
	// ClearParent forces parent_id to NULL when true. Distinguishes "move to
	// root" from "leave parent_id unchanged" — JSON null on ParentID alone
	// can't be distinguished from omitted in Go's json package without a
	// custom unmarshaler.
	ClearParent bool    `json:"clear_parent,omitempty"`
	Position    *int    `json:"position,omitempty"`
	SiblingIDs  []int64 `json:"sibling_ids,omitempty"`
}

// Update renames, moves, or reorders a folder. Reordering uses SiblingIDs:
// callers send the desired ordered list of children for a given parent and
// the backend assigns sequential positions.
func (h *FolderHandler) Update(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid folder id")
		return
	}

	var folder models.ThreadFolder
	if err := h.db.First(&folder, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "folder not found")
		return
	}

	var req updateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request")
		return
	}

	updates := map[string]interface{}{}

	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || len(name) > 255 {
			respondError(c, http.StatusBadRequest, "name is required (max 255 chars)")
			return
		}
		updates["name"] = name
	}

	if req.ClearParent || req.ParentID != nil {
		var newParent *int64
		if !req.ClearParent {
			newParent = req.ParentID
		}
		if newParent != nil {
			if *newParent == folder.ID {
				respondError(c, http.StatusBadRequest, "folder cannot be its own parent")
				return
			}
			var parent models.ThreadFolder
			if err := h.db.First(&parent, *newParent).Error; err != nil {
				respondError(c, http.StatusBadRequest, "parent folder not found")
				return
			}
			ancestor, err := isDescendant(h.db, folder.ID, *newParent)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "failed to verify hierarchy")
				return
			}
			if ancestor {
				respondError(c, http.StatusBadRequest, "cannot move a folder under one of its descendants")
				return
			}
			depth, err := folderDepth(h.db, *newParent)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "failed to compute depth")
				return
			}
			subtreeDepth, err := subtreeMaxDepth(h.db, folder.ID)
			if err != nil {
				respondError(c, http.StatusInternalServerError, "failed to compute subtree depth")
				return
			}
			if depth+subtreeDepth > maxFolderDepth {
				respondError(c, http.StatusBadRequest, "maximum folder nesting depth exceeded")
				return
			}
		}
		updates["parent_id"] = newParent
		// When the parent changes, place the moved folder at the end of the
		// new parent's child list unless an explicit position arrives in the
		// same request via SiblingIDs.
		if len(req.SiblingIDs) == 0 && req.Position == nil {
			var maxPos *int
			h.db.Raw(`SELECT MAX(position) FROM thread_folders
				WHERE parent_id IS NOT DISTINCT FROM ? AND id <> ?`,
				newParent, folder.ID).Scan(&maxPos)
			pos := 0
			if maxPos != nil {
				pos = *maxPos + 1
			}
			updates["position"] = pos
		}
	}

	if req.Position != nil && len(req.SiblingIDs) == 0 {
		updates["position"] = *req.Position
	}

	if len(updates) > 0 {
		updates["updated_at"] = time.Now()
		if err := h.db.Model(&models.ThreadFolder{}).Where("id = ?", id).
			Updates(updates).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to update folder")
			return
		}
	}

	if len(req.SiblingIDs) > 0 {
		if err := h.applySiblingOrder(req.SiblingIDs); err != nil {
			respondError(c, http.StatusInternalServerError, err.Error())
			return
		}
	}

	respondOK(c, gin.H{"status": "ok"})
}

// applySiblingOrder writes sequential positions for the given folder IDs in
// the order they appear. All IDs must share the same parent — the caller is
// expected to enforce this when constructing the list.
func (h *FolderHandler) applySiblingOrder(ids []int64) error {
	tx := h.db.Begin()
	for i, fid := range ids {
		if err := tx.Model(&models.ThreadFolder{}).Where("id = ?", fid).
			Updates(map[string]interface{}{
				"position":   i,
				"updated_at": time.Now(),
			}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit().Error
}

// Delete removes an empty folder (no child folders, no threads). Non-empty
// folders return 409 with a useful message so the UI can prompt the user.
func (h *FolderHandler) Delete(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid folder id")
		return
	}

	var folder models.ThreadFolder
	if err := h.db.First(&folder, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "folder not found")
		return
	}

	var childCount int64
	h.db.Model(&models.ThreadFolder{}).Where("parent_id = ?", id).Count(&childCount)
	var threadCount int64
	h.db.Model(&models.Thread{}).Where("folder_id = ?", id).Count(&threadCount)
	if childCount > 0 || threadCount > 0 {
		respondError(c, http.StatusConflict,
			"folder is not empty: move or delete its contents first")
		return
	}

	if err := h.db.Delete(&models.ThreadFolder{}, id).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to delete folder")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}

// folderDepth returns the depth of the given folder (root = 1).
func folderDepth(db *gorm.DB, id int64) (int, error) {
	var depth int
	err := db.Raw(`WITH RECURSIVE chain AS (
		SELECT id, parent_id, 1 AS depth FROM thread_folders WHERE id = ?
		UNION ALL
		SELECT f.id, f.parent_id, c.depth + 1 FROM thread_folders f
		JOIN chain c ON f.id = c.parent_id
	)
	SELECT MAX(depth) FROM chain`, id).Scan(&depth).Error
	return depth, err
}

// subtreeMaxDepth returns the depth of the deepest descendant relative to the
// root of the subtree starting at id (the root itself counts as 1).
func subtreeMaxDepth(db *gorm.DB, id int64) (int, error) {
	var depth int
	err := db.Raw(`WITH RECURSIVE descendants AS (
		SELECT id, 1 AS depth FROM thread_folders WHERE id = ?
		UNION ALL
		SELECT f.id, d.depth + 1 FROM thread_folders f
		JOIN descendants d ON f.parent_id = d.id
	)
	SELECT COALESCE(MAX(depth), 1) FROM descendants`, id).Scan(&depth).Error
	if depth == 0 {
		depth = 1
	}
	return depth, err
}

// isDescendant returns true if candidate is folderID itself or a descendant
// of folderID. Used to reject moves that would create a cycle.
func isDescendant(db *gorm.DB, folderID, candidate int64) (bool, error) {
	if folderID == candidate {
		return true, nil
	}
	var found int64
	err := db.Raw(`WITH RECURSIVE descendants AS (
		SELECT id FROM thread_folders WHERE id = ?
		UNION ALL
		SELECT f.id FROM thread_folders f
		JOIN descendants d ON f.parent_id = d.id
	)
	SELECT COUNT(*) FROM descendants WHERE id = ?`, folderID, candidate).Scan(&found).Error
	return found > 0, err
}
