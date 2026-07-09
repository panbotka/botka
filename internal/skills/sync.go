package skills

import (
	"fmt"

	"gorm.io/gorm"

	"botka/internal/models"
)

// SyncToDatabase reconciles the skill registry with what was found on disk.
//
// Newly discovered skills are inserted with default_enabled = true so existing
// behavior is preserved: a chat that never touched its skill settings keeps
// every skill available. Known skills have their description, source, and
// active flag refreshed but keep their default_enabled — that flag is
// user-owned, not disk-owned.
//
// Skills that vanished from disk are deactivated rather than deleted, which
// keeps per-thread overrides intact if the skill comes back (a plugin being
// upgraded, a project temporarily missing). SyncToDatabase is idempotent.
func SyncToDatabase(db *gorm.DB, discovered []Discovered) error {
	present := make(map[string]struct{}, len(discovered))
	for _, d := range discovered {
		present[d.Name] = struct{}{}
		if err := upsertSkill(db, d); err != nil {
			return fmt.Errorf("upsert skill %q: %w", d.Name, err)
		}
	}
	return deactivateMissing(db, present)
}

// upsertSkill inserts a newly discovered skill or refreshes the disk-derived
// fields of an existing one, never touching its default_enabled flag.
func upsertSkill(db *gorm.DB, d Discovered) error {
	var existing models.Skill
	err := db.Where("name = ?", d.Name).First(&existing).Error
	if err == gorm.ErrRecordNotFound {
		return db.Create(&models.Skill{
			Name:           d.Name,
			Description:    d.Description,
			Source:         d.Source,
			DefaultEnabled: true,
			Active:         true,
		}).Error
	}
	if err != nil {
		return err
	}

	return db.Model(&existing).Updates(map[string]interface{}{
		"description": d.Description,
		"source":      d.Source,
		"active":      true,
	}).Error
}

// deactivateMissing flags every active registry entry whose name is absent
// from present as inactive. Overrides in thread_skills are left untouched.
func deactivateMissing(db *gorm.DB, present map[string]struct{}) error {
	var active []models.Skill
	if err := db.Where("active = ?", true).Find(&active).Error; err != nil {
		return fmt.Errorf("load active skills: %w", err)
	}

	var stale []string
	for _, s := range active {
		if _, ok := present[s.Name]; !ok {
			stale = append(stale, s.Name)
		}
	}
	if len(stale) == 0 {
		return nil
	}

	if err := db.Model(&models.Skill{}).Where("name IN ?", stale).
		Update("active", false).Error; err != nil {
		return fmt.Errorf("deactivate missing skills: %w", err)
	}
	return nil
}
