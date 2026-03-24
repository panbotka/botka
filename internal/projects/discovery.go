// Package projects provides project auto-discovery by scanning directories for git repositories.
package projects

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"botka/internal/models"

	"gorm.io/gorm"
)

// DiscoveredProject represents a git repository found during directory scanning.
type DiscoveredProject struct {
	Name string
	Path string
}

// Scan walks the top-level entries of projectsDir and returns directories that contain
// a .git/ subdirectory. It skips hidden directories.
// Directories that cannot be read due to permission errors are logged and skipped.
func Scan(projectsDir string) ([]DiscoveredProject, error) {
	absDir, err := filepath.Abs(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("resolving absolute path for %q: %w", projectsDir, err)
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("reading directory %q: %w", absDir, err)
	}

	var discovered []DiscoveredProject

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}

		entryPath := filepath.Join(absDir, name)
		gitPath := filepath.Join(entryPath, ".git")

		info, err := os.Stat(gitPath)
		if err != nil {
			if os.IsPermission(err) {
				log.Printf("warning: skipping %q: permission denied", entryPath)
			}
			continue
		}

		if !info.IsDir() {
			continue
		}

		discovered = append(discovered, DiscoveredProject{
			Name: name,
			Path: entryPath,
		})
	}

	return discovered, nil
}

// SyncToDatabase upserts discovered projects into the database and marks projects
// no longer found on disk as inactive. Existing branch_strategy, verification_command,
// claude_md, and sort_order values are preserved for known projects.
// This function is idempotent.
func SyncToDatabase(db *gorm.DB, discovered []DiscoveredProject) error {
	discoveredPaths := make(map[string]struct{}, len(discovered))

	for _, dp := range discovered {
		discoveredPaths[dp.Path] = struct{}{}

		if err := upsertProject(db, dp); err != nil {
			return err
		}
	}

	return deactivateMissing(db, discoveredPaths)
}

// upsertProject creates a new project record or updates an existing one by path.
// New projects get branch_strategy "main" and active=true. Existing projects get
// their name updated and active set to true, preserving other fields.
func upsertProject(db *gorm.DB, dp DiscoveredProject) error {
	var existing models.Project
	result := db.Where("path = ?", dp.Path).First(&existing)

	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		project := models.Project{
			Name:           dp.Name,
			Path:           dp.Path,
			BranchStrategy: "main",
			Active:         true,
		}
		if err := db.Create(&project).Error; err != nil {
			return fmt.Errorf("creating project %q: %w", dp.Name, err)
		}
		return nil
	}

	if result.Error != nil {
		return fmt.Errorf("querying project by path %q: %w", dp.Path, result.Error)
	}

	if err := db.Model(&existing).Updates(map[string]any{
		"name":   dp.Name,
		"active": true,
	}).Error; err != nil {
		return fmt.Errorf("updating project %q: %w", dp.Name, err)
	}

	return nil
}

// deactivateMissing sets active=false on projects whose paths are not in the given set.
func deactivateMissing(db *gorm.DB, discoveredPaths map[string]struct{}) error {
	var allProjects []models.Project
	if err := db.Find(&allProjects).Error; err != nil {
		return fmt.Errorf("listing all projects: %w", err)
	}

	for _, project := range allProjects {
		if _, found := discoveredPaths[project.Path]; !found && project.Active {
			if err := db.Model(&project).Update("active", false).Error; err != nil {
				return fmt.Errorf("deactivating project %q: %w", project.Name, err)
			}
		}
	}

	return nil
}
