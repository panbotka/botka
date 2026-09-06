package boxoff

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"botka/internal/claude"
	"botka/internal/runner"
)

// ActivitySource reports the work Botka itself has in flight. Both methods
// return human-readable labels because they end up in the UI as the reason a
// shutdown did not happen.
type ActivitySource interface {
	// RunningTasks lists the tasks the runner currently executes.
	RunningTasks() []string
	// BoxChatThreads lists running chat sessions whose project lives on Box.
	BoxChatThreads(ctx context.Context) ([]string, error)
}

// TaskStatusSource is the slice of *runner.Runner this package needs.
type TaskStatusSource interface {
	GetStatus() runner.Status
}

// ChatRegistry is the slice of claude.ProcessRegistry this package needs.
type ChatRegistry interface {
	List() []claude.ProcessInfo
}

// AppActivity answers the activity questions from the live runner, the chat
// process registry and the database.
type AppActivity struct {
	tasks TaskStatusSource
	chats ChatRegistry
	db    *gorm.DB
}

// NewAppActivity wires an ActivitySource over the running application.
func NewAppActivity(tasks TaskStatusSource, chats ChatRegistry, db *gorm.DB) *AppActivity {
	return &AppActivity{tasks: tasks, chats: chats, db: db}
}

// RunningTasks lists every task the runner has in flight — including tasks on
// projects unrelated to Box, because a shutdown mid-run costs the user a task
// either way.
func (a *AppActivity) RunningTasks() []string {
	if a.tasks == nil {
		return nil
	}
	status := a.tasks.GetStatus()
	labels := make([]string, 0, len(status.ActiveTasks))
	for _, t := range status.ActiveTasks {
		labels = append(labels, fmt.Sprintf("%s (%s)", t.TaskTitle, t.ProjectName))
	}
	return labels
}

// BoxChatThreads lists running chat sessions that spawned Claude Code on Box.
// A chat is on Box when its thread's project path carries the "box:" prefix
// (see internal/claude/remote.go).
//
// With no chat process running at all the database is never touched. With one
// running and no database to resolve it, this is an error rather than an empty
// list: an unresolvable chat is unknown, and unknown must block.
func (a *AppActivity) BoxChatThreads(ctx context.Context) ([]string, error) {
	if a.chats == nil {
		return nil, nil
	}
	procs := a.chats.List()
	if len(procs) == 0 {
		return nil, nil
	}
	if a.db == nil {
		return nil, fmt.Errorf("no database to resolve %d chat threads", len(procs))
	}

	ids := make([]int64, 0, len(procs))
	titles := make(map[int64]string, len(procs))
	for _, p := range procs {
		ids = append(ids, p.ThreadID)
		titles[p.ThreadID] = p.ThreadTitle
	}

	var boxThreadIDs []int64
	err := a.db.WithContext(ctx).
		Table("threads").
		Joins("JOIN projects ON projects.id = threads.project_id").
		Where("threads.id IN ?", ids).
		Where("projects.path LIKE ?", claude.RemotePrefix+"%").
		Pluck("threads.id", &boxThreadIDs).Error
	if err != nil {
		return nil, fmt.Errorf("resolve box chat threads: %w", err)
	}

	labels := make([]string, 0, len(boxThreadIDs))
	for _, id := range boxThreadIDs {
		labels = append(labels, titles[id])
	}
	return labels, nil
}
