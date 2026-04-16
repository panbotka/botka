package models

import (
	"testing"

	"github.com/google/uuid"
)

func TestCronJob_Defaults(t *testing.T) {
	t.Parallel()

	job := CronJob{}
	if job.Enabled {
		t.Error("expected Enabled to default to zero value (false) in Go; DB default handles true")
	}
	if job.TimeoutMinutes != 0 {
		t.Errorf("expected TimeoutMinutes zero value, got %d", job.TimeoutMinutes)
	}
}

func TestCronJob_Fields(t *testing.T) {
	t.Parallel()

	projectID := uuid.New()
	model := "opus"
	job := CronJob{
		ID:             1,
		Name:           "Weekly scan",
		Schedule:       "0 9 * * 1",
		Prompt:         "Check for issues",
		ProjectID:      projectID,
		Enabled:        true,
		TimeoutMinutes: 60,
		Model:          &model,
	}

	if job.Name != "Weekly scan" {
		t.Errorf("expected Name = %q, got %q", "Weekly scan", job.Name)
	}
	if job.Schedule != "0 9 * * 1" {
		t.Errorf("expected Schedule = %q, got %q", "0 9 * * 1", job.Schedule)
	}
	if job.ProjectID != projectID {
		t.Errorf("expected ProjectID = %s, got %s", projectID, job.ProjectID)
	}
	if *job.Model != "opus" {
		t.Errorf("expected Model = %q, got %q", "opus", *job.Model)
	}
}

func TestCronExecution_Defaults(t *testing.T) {
	t.Parallel()

	exec := CronExecution{}
	if exec.Status != "" {
		t.Errorf("expected empty Status in Go zero value, got %q", exec.Status)
	}
	if exec.CostUSD != 0 {
		t.Errorf("expected CostUSD = 0, got %f", exec.CostUSD)
	}
}
