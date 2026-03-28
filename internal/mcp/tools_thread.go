package mcp

import (
	"encoding/json"
	"errors"
	"fmt"

	"gorm.io/gorm"

	"botka/internal/models"
)

// listThreadsArgs holds the arguments for the list_threads tool.
type listThreadsArgs struct {
	ProjectName string `json:"project_name"`
	Limit       int    `json:"limit"`
	Offset      int    `json:"offset"`
}

// handleListThreads lists chat threads, optionally filtered by project.
func (s *Server) handleListThreads(raw json.RawMessage) (interface{}, error) {
	var args listThreadsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}

	limit := 20
	if args.Limit > 0 {
		limit = args.Limit
	}
	offset := 0
	if args.Offset > 0 {
		offset = args.Offset
	}

	query := s.db.Preload("Project").Order("updated_at DESC")

	if args.ProjectName != "" {
		project, err := s.findProjectByNameAll(args.ProjectName)
		if err != nil {
			return nil, err
		}
		query = query.Where("project_id = ?", project.ID)
	}

	var threads []models.Thread
	if err := query.Limit(limit).Offset(offset).Find(&threads).Error; err != nil {
		return nil, fmt.Errorf("failed to list threads: %w", err)
	}

	if len(threads) == 0 {
		return "No threads found.", nil
	}

	result := make([]map[string]interface{}, 0, len(threads))
	for i := range threads {
		t := &threads[i]
		entry := map[string]interface{}{
			"id":         t.ID,
			"title":      t.Title,
			"pinned":     t.Pinned,
			"archived":   t.Archived,
			"created_at": t.CreatedAt.Format(timeFmt),
			"updated_at": t.UpdatedAt.Format(timeFmt),
		}
		if t.Model != nil {
			entry["model"] = *t.Model
		}
		if t.ProjectID != nil {
			entry["project_id"] = *t.ProjectID
		}
		if t.Project != nil {
			entry["project_name"] = t.Project.Name
		}
		result = append(result, entry)
	}

	return result, nil
}

// listThreadSourcesArgs holds the arguments for the list_thread_sources tool.
type listThreadSourcesArgs struct {
	ThreadID int64 `json:"thread_id"`
}

// handleListThreadSources lists URL sources attached to a thread.
func (s *Server) handleListThreadSources(raw json.RawMessage) (interface{}, error) {
	var args listThreadSourcesArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}

	var sources []models.ThreadSource
	if err := s.db.Where("thread_id = ?", args.ThreadID).
		Order("position ASC").Find(&sources).Error; err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	if len(sources) == 0 {
		return []interface{}{}, nil
	}

	result := make([]map[string]interface{}, 0, len(sources))
	for i := range sources {
		src := &sources[i]
		result = append(result, map[string]interface{}{
			"id":       src.ID,
			"url":      src.URL,
			"label":    src.Label,
			"position": src.Position,
		})
	}

	return result, nil
}

// addThreadSourceArgs holds the arguments for the add_thread_source tool.
type addThreadSourceArgs struct {
	ThreadID int64  `json:"thread_id"`
	URL      string `json:"url"`
	Label    string `json:"label"`
}

// handleAddThreadSource adds a URL source to a thread.
func (s *Server) handleAddThreadSource(raw json.RawMessage) (interface{}, error) {
	var args addThreadSourceArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	if args.URL == "" {
		return nil, errors.New("url is required")
	}

	// Calculate next position.
	var maxPos int
	s.db.Model(&models.ThreadSource{}).
		Where("thread_id = ?", args.ThreadID).
		Select("COALESCE(MAX(position), -1)").
		Scan(&maxPos)

	source := models.ThreadSource{
		ThreadID: args.ThreadID,
		URL:      args.URL,
		Label:    args.Label,
		Position: maxPos + 1,
	}
	if err := s.db.Create(&source).Error; err != nil {
		return nil, fmt.Errorf("failed to create source: %w", err)
	}

	return map[string]interface{}{
		"id":       source.ID,
		"url":      source.URL,
		"label":    source.Label,
		"position": source.Position,
	}, nil
}

// removeThreadSourceArgs holds the arguments for the remove_thread_source tool.
type removeThreadSourceArgs struct {
	ThreadID int64 `json:"thread_id"`
	SourceID int64 `json:"source_id"`
}

// handleRemoveThreadSource removes a URL source from a thread.
func (s *Server) handleRemoveThreadSource(raw json.RawMessage) (interface{}, error) {
	var args removeThreadSourceArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	if args.SourceID <= 0 {
		return nil, errors.New("source_id is required")
	}

	result := s.db.Where("id = ? AND thread_id = ?", args.SourceID, args.ThreadID).
		Delete(&models.ThreadSource{})
	if result.Error != nil {
		return nil, fmt.Errorf("failed to delete source: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return nil, errors.New("source not found")
	}

	return "Source removed.", nil
}

// updateThreadSourceArgs holds the arguments for the update_thread_source tool.
type updateThreadSourceArgs struct {
	ThreadID int64   `json:"thread_id"`
	SourceID int64   `json:"source_id"`
	URL      *string `json:"url"`
	Label    *string `json:"label"`
}

// handleUpdateThreadSource updates a thread source's URL or label.
func (s *Server) handleUpdateThreadSource(raw json.RawMessage) (interface{}, error) {
	var args updateThreadSourceArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}
	if args.SourceID <= 0 {
		return nil, errors.New("source_id is required")
	}

	var source models.ThreadSource
	if err := s.db.Where("id = ? AND thread_id = ?", args.SourceID, args.ThreadID).
		First(&source).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("source not found")
		}
		return nil, fmt.Errorf("failed to find source: %w", err)
	}

	updates := map[string]interface{}{}
	if args.URL != nil {
		updates["url"] = *args.URL
	}
	if args.Label != nil {
		updates["label"] = *args.Label
	}

	if len(updates) == 0 {
		return "No changes specified.", nil
	}

	if err := s.db.Model(&source).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update source: %w", err)
	}

	// Reload.
	s.db.First(&source, source.ID)

	return map[string]interface{}{
		"id":       source.ID,
		"url":      source.URL,
		"label":    source.Label,
		"position": source.Position,
	}, nil
}

// getThreadContextArgs holds the arguments for the get_thread_context tool.
type getThreadContextArgs struct {
	ThreadID int64 `json:"thread_id"`
}

// handleGetThreadContext returns a thread's custom context content.
func (s *Server) handleGetThreadContext(raw json.RawMessage) (interface{}, error) {
	var args getThreadContextArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}

	var thread models.Thread
	if err := s.db.First(&thread, args.ThreadID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("thread not found")
		}
		return nil, fmt.Errorf("failed to find thread: %w", err)
	}

	return map[string]interface{}{
		"thread_id":      thread.ID,
		"custom_context": thread.CustomContext,
		"length":         len(thread.CustomContext),
	}, nil
}

// setThreadContextArgs holds the arguments for the set_thread_context tool.
type setThreadContextArgs struct {
	ThreadID int64  `json:"thread_id"`
	Content  string `json:"content"`
}

// handleSetThreadContext updates a thread's custom context content.
func (s *Server) handleSetThreadContext(raw json.RawMessage) (interface{}, error) {
	var args setThreadContextArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return nil, fmt.Errorf("invalid arguments: %w", err)
	}
	if args.ThreadID <= 0 {
		return nil, errors.New("thread_id is required")
	}

	var thread models.Thread
	if err := s.db.First(&thread, args.ThreadID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("thread not found")
		}
		return nil, fmt.Errorf("failed to find thread: %w", err)
	}

	if err := s.db.Model(&thread).Update("custom_context", args.Content).Error; err != nil {
		return nil, fmt.Errorf("failed to update custom context: %w", err)
	}

	return map[string]interface{}{
		"thread_id": thread.ID,
		"length":    len(args.Content),
		"status":    "updated",
	}, nil
}
