package models

import (
	"sort"
	"time"

	"gorm.io/gorm"
)

// Skill is a Claude Code skill discovered on disk and tracked in the registry.
// The registry is global: skills are keyed by their invocable name (for plugin
// skills that is the namespaced "plugin:skill" form). DefaultEnabled decides
// whether new chats — and chats without an explicit override — may use it.
type Skill struct {
	ID          int64  `json:"id" gorm:"primaryKey"`
	Name        string `json:"name" gorm:"uniqueIndex;not null;size:200"`
	Description string `json:"description" gorm:"not null;default:''"`
	Source      string `json:"source" gorm:"not null;default:'user';size:200"`
	// DefaultEnabled and Active carry no GORM `default:` tag on purpose: GORM
	// omits zero-valued fields from an INSERT when a default is declared,
	// which would silently turn every `false` into the column default. The
	// DDL defaults live in the migration instead.
	DefaultEnabled bool      `json:"default_enabled" gorm:"not null"`
	Active         bool      `json:"active" gorm:"not null"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// TableName returns the database table name for the Skill model.
func (Skill) TableName() string {
	return "skills"
}

// ThreadSkill is a per-thread override of a skill's DefaultEnabled flag.
// A row exists only when the thread's desired state differs from the skill's
// current default, so changing a default keeps propagating to threads that
// never expressed a preference.
type ThreadSkill struct {
	ThreadID  int64  `json:"thread_id" gorm:"primaryKey"`
	SkillName string `json:"skill_name" gorm:"primaryKey;size:200"`
	Enabled   bool   `json:"enabled" gorm:"not null"`
}

// TableName returns the database table name for the ThreadSkill model.
func (ThreadSkill) TableName() string {
	return "thread_skills"
}

// EffectiveSkill is a skill together with its resolved state for one thread.
type EffectiveSkill struct {
	Name           string `json:"name"`
	Description    string `json:"description"`
	Source         string `json:"source"`
	DefaultEnabled bool   `json:"default_enabled"`
	Enabled        bool   `json:"enabled"`
	Overridden     bool   `json:"overridden"`
}

// ResolveThreadSkills returns every active skill with the state it has for the
// given thread: the thread_skills override when one exists, otherwise the
// skill's current DefaultEnabled. Results are ordered by name.
func ResolveThreadSkills(db *gorm.DB, threadID int64) ([]EffectiveSkill, error) {
	var registry []Skill
	if err := db.Where("active = ?", true).Order("name ASC").Find(&registry).Error; err != nil {
		return nil, err
	}

	var overrides []ThreadSkill
	if err := db.Where("thread_id = ?", threadID).Find(&overrides).Error; err != nil {
		return nil, err
	}
	byName := make(map[string]bool, len(overrides))
	for _, o := range overrides {
		byName[o.SkillName] = o.Enabled
	}

	result := make([]EffectiveSkill, 0, len(registry))
	for _, s := range registry {
		enabled, overridden := byName[s.Name]
		if !overridden {
			enabled = s.DefaultEnabled
		}
		result = append(result, EffectiveSkill{
			Name:           s.Name,
			Description:    s.Description,
			Source:         s.Source,
			DefaultEnabled: s.DefaultEnabled,
			Enabled:        enabled,
			Overridden:     overridden,
		})
	}
	return result, nil
}

// DisabledSkillsForThread returns the sorted names of active skills that are
// OFF for the given thread. Claude Code has no positive allowlist for skills,
// so enforcement is subtractive: every returned name becomes a
// --disallowedTools "Skill(<name>)" entry on the spawned subprocess.
func DisabledSkillsForThread(db *gorm.DB, threadID int64) ([]string, error) {
	effective, err := ResolveThreadSkills(db, threadID)
	if err != nil {
		return nil, err
	}

	var disabled []string
	for _, s := range effective {
		if !s.Enabled {
			disabled = append(disabled, s.Name)
		}
	}
	sort.Strings(disabled)
	return disabled, nil
}
