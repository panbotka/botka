package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"botka/internal/models"
)

// updateProjectArgs holds the arguments for the update_project tool.
type updateProjectArgs struct {
	ProjectName         string  `json:"project_name"`
	DevCommand          *string `json:"dev_command"`
	DeployCommand       *string `json:"deploy_command"`
	DevPort             *int    `json:"dev_port"`
	DeployPort          *int    `json:"deploy_port"`
	VerificationCommand *string `json:"verification_command"`
	BranchStrategy      *string `json:"branch_strategy"`
}

// handleUpdateProject updates a project's configuration fields.
func (s *Server) handleUpdateProject(raw json.RawMessage) (interface{}, error) {
	var args updateProjectArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ProjectName == "" {
		return nil, errors.New("project_name is required")
	}

	project, err := s.findProjectByName(args.ProjectName)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{}
	var changed []string

	if args.DevCommand != nil {
		if *args.DevCommand == "" {
			updates["dev_command"] = nil
		} else {
			updates["dev_command"] = *args.DevCommand
		}
		changed = append(changed, "dev_command")
	}
	if args.DeployCommand != nil {
		if *args.DeployCommand == "" {
			updates["deploy_command"] = nil
		} else {
			updates["deploy_command"] = *args.DeployCommand
		}
		changed = append(changed, "deploy_command")
	}
	if args.VerificationCommand != nil {
		if *args.VerificationCommand == "" {
			updates["verification_command"] = nil
		} else {
			updates["verification_command"] = *args.VerificationCommand
		}
		changed = append(changed, "verification_command")
	}
	if args.DevPort != nil {
		if *args.DevPort == 0 {
			updates["dev_port"] = nil
		} else {
			updates["dev_port"] = *args.DevPort
		}
		changed = append(changed, "dev_port")
	}
	if args.DeployPort != nil {
		if *args.DeployPort == 0 {
			updates["deploy_port"] = nil
		} else {
			updates["deploy_port"] = *args.DeployPort
		}
		changed = append(changed, "deploy_port")
	}
	if args.BranchStrategy != nil {
		if *args.BranchStrategy != "main" && *args.BranchStrategy != "feature_branch" {
			return nil, errors.New("branch_strategy must be \"main\" or \"feature_branch\"")
		}
		updates["branch_strategy"] = *args.BranchStrategy
		changed = append(changed, "branch_strategy")
	}

	if len(updates) == 0 {
		return "No changes specified.", nil
	}

	if err := s.db.Model(&project).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	// Reload the project to return updated values.
	if err := s.db.First(&project, "id = ?", project.ID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload project: %w", err)
	}

	return formatProject(&project, changed), nil
}

// formatProject formats a project for tool output.
func formatProject(p *models.Project, changed []string) string {
	result := map[string]interface{}{
		"id":              p.ID,
		"name":            p.Name,
		"path":            p.Path,
		"branch_strategy": p.BranchStrategy,
		"active":          p.Active,
		"updated_fields":  changed,
	}
	if p.DevCommand != nil {
		result["dev_command"] = *p.DevCommand
	}
	if p.DeployCommand != nil {
		result["deploy_command"] = *p.DeployCommand
	}
	if p.VerificationCommand != nil {
		result["verification_command"] = *p.VerificationCommand
	}
	if p.DevPort != nil {
		result["dev_port"] = *p.DevPort
	}
	if p.DeployPort != nil {
		result["deploy_port"] = *p.DeployPort
	}
	return mustJSON(result)
}

// findProjectByNameAll looks up a project by case-insensitive name (including inactive).
func (s *Server) findProjectByNameAll(name string) (models.Project, error) {
	var project models.Project
	err := s.db.Where("LOWER(name) = LOWER(?)", name).First(&project).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return project, fmt.Errorf("project %q not found", name)
	}
	if err != nil {
		return project, fmt.Errorf("failed to look up project: %w", err)
	}
	return project, nil
}

// mustJSON marshals v to JSON, panicking on error (should never happen with map types).
func mustJSON(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("{\"error\": \"marshal failed: %s\"}", err)
	}
	return string(data)
}
