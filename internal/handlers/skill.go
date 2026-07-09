package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"botka/internal/models"
	"botka/internal/skills"
)

// SkillScanFunc discovers skills on disk. It mirrors skills.Scan and is
// injected so tests can exercise the rescan endpoint without touching $HOME.
type SkillScanFunc func(homeDir, projectsDir string) ([]skills.Discovered, error)

// SkillSyncFunc reconciles discovered skills with the registry. It mirrors
// skills.SyncToDatabase.
type SkillSyncFunc func(db *gorm.DB, discovered []skills.Discovered) error

// SkillHandler serves the global skill registry and per-thread skill overrides.
type SkillHandler struct {
	db          *gorm.DB
	homeDir     string
	projectsDir string
	scanFn      SkillScanFunc
	syncFn      SkillSyncFunc
}

// NewSkillHandler creates a SkillHandler that rescans skills under homeDir and
// projectsDir using the given scan and sync functions.
func NewSkillHandler(
	db *gorm.DB, homeDir, projectsDir string, scanFn SkillScanFunc, syncFn SkillSyncFunc,
) *SkillHandler {
	return &SkillHandler{
		db:          db,
		homeDir:     homeDir,
		projectsDir: projectsDir,
		scanFn:      scanFn,
		syncFn:      syncFn,
	}
}

// RegisterSkillRoutes attaches skill registry and per-thread skill endpoints.
func RegisterSkillRoutes(rg *gin.RouterGroup, h *SkillHandler) {
	rg.GET("/skills", h.List)
	rg.PATCH("/skills/:name", h.Update)
	rg.POST("/skills/rescan", h.Rescan)
	rg.GET("/threads/:id/skills", h.ListThreadSkills)
	rg.PUT("/threads/:id/skills", h.SetThreadSkills)
}

// List returns the whole registry, active skills first, ordered by name.
func (h *SkillHandler) List(c *gin.Context) {
	registry, err := h.loadRegistry()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list skills")
		return
	}
	respondList(c, registry, int64(len(registry)))
}

// loadRegistry reads every registry row, sorting active skills ahead of the
// ones that disappeared from disk.
func (h *SkillHandler) loadRegistry() ([]models.Skill, error) {
	var registry []models.Skill
	if err := h.db.Order("active DESC, name ASC").Find(&registry).Error; err != nil {
		return nil, err
	}
	if registry == nil {
		registry = []models.Skill{}
	}
	return registry, nil
}

type updateSkillRequest struct {
	DefaultEnabled *bool `json:"default_enabled"`
}

// Update sets a skill's default_enabled flag. The change applies to new chats
// and to existing chats that never overrode this skill.
func (h *SkillHandler) Update(c *gin.Context) {
	name := c.Param("name")

	var req updateSkillRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.DefaultEnabled == nil {
		respondError(c, http.StatusBadRequest, "default_enabled is required")
		return
	}

	var skill models.Skill
	if err := h.db.Where("name = ?", name).First(&skill).Error; err != nil {
		respondError(c, http.StatusNotFound, "skill not found")
		return
	}

	if err := h.db.Model(&skill).Update("default_enabled", *req.DefaultEnabled).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update skill")
		return
	}

	respondOK(c, skill)
}

// Rescan re-reads the skill directories from disk and syncs the registry,
// then returns the refreshed registry. User-set default_enabled flags and
// per-thread overrides survive the rescan.
func (h *SkillHandler) Rescan(c *gin.Context) {
	discovered, err := h.scanFn(h.homeDir, h.projectsDir)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to scan skills")
		return
	}
	if err := h.syncFn(h.db, discovered); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to sync skills")
		return
	}

	registry, err := h.loadRegistry()
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list skills")
		return
	}
	respondList(c, registry, int64(len(registry)))
}

// ListThreadSkills returns every active skill with its effective state for the
// thread: the override when one exists, otherwise the skill's default.
func (h *SkillHandler) ListThreadSkills(c *gin.Context) {
	threadID, ok := h.requireThread(c)
	if !ok {
		return
	}

	effective, err := models.ResolveThreadSkills(h.db, threadID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list thread skills")
		return
	}
	respondOK(c, effective)
}

type setThreadSkillsRequest struct {
	EnabledSkills []string `json:"enabled_skills"`
}

// SetThreadSkills replaces the thread's skill selection. The request lists the
// skills that should be ON; every other active skill is turned OFF.
//
// Only states that differ from the skill's current default are persisted as
// overrides — a skill matching its default has its override removed, so later
// changes to that default keep propagating to this thread.
func (h *SkillHandler) SetThreadSkills(c *gin.Context) {
	threadID, ok := h.requireThread(c)
	if !ok {
		return
	}

	var req setThreadSkillsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	var registry []models.Skill
	if err := h.db.Where("active = ?", true).Find(&registry).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to load skills")
		return
	}

	wanted := make(map[string]bool, len(req.EnabledSkills))
	for _, name := range req.EnabledSkills {
		wanted[name] = true
	}

	if err := h.db.Transaction(func(tx *gorm.DB) error {
		return applySkillOverrides(tx, threadID, registry, wanted)
	}); err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update thread skills")
		return
	}

	effective, err := models.ResolveThreadSkills(h.db, threadID)
	if err != nil {
		respondError(c, http.StatusInternalServerError, "failed to list thread skills")
		return
	}
	respondOK(c, effective)
}

// applySkillOverrides writes one override row per skill whose desired state
// differs from its default and deletes the rows for skills that match it.
func applySkillOverrides(tx *gorm.DB, threadID int64, registry []models.Skill, wanted map[string]bool) error {
	for _, skill := range registry {
		enabled := wanted[skill.Name]
		if enabled == skill.DefaultEnabled {
			err := tx.Where("thread_id = ? AND skill_name = ?", threadID, skill.Name).
				Delete(&models.ThreadSkill{}).Error
			if err != nil {
				return err
			}
			continue
		}

		override := models.ThreadSkill{ThreadID: threadID, SkillName: skill.Name, Enabled: enabled}
		// GORM's Save() only UPDATEs when a composite primary key is fully
		// populated, so upsert explicitly.
		err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "thread_id"}, {Name: "skill_name"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled"}),
		}).Create(&override).Error
		if err != nil {
			return err
		}
	}
	return nil
}

// requireThread parses the :id path param and verifies the thread exists,
// writing the error response itself and returning ok=false on failure.
func (h *SkillHandler) requireThread(c *gin.Context) (int64, bool) {
	threadID, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return 0, false
	}

	var thread models.Thread
	if err := h.db.First(&thread, threadID).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return 0, false
	}
	return threadID, true
}
