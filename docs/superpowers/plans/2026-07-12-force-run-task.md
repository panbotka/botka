# Force-Run a Queued Task Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a per-task "Spustit teď" escape hatch that force-launches a single queued task immediately, bypassing both rate-limit gates, while honoring MAX_WORKERS and the one-running-per-project invariant.

**Architecture:** A new `Runner.ForceRunTask` claims a queued task by ID (`pickTaskByID`, a gate-free sibling of `pickNextTask`) and launches it through the existing `launchTask`/`executeTask` machinery — never consulting `UsageMonitor.IsRateLimited()` or the `RateLimitGate`. A REST endpoint `POST /api/v1/tasks/:id/force-run` and a frontend button drive it. `launchTask` gains a `bool` return so the API reports success only when the task actually started; a `launchFn` test seam (mirroring the existing `pingFn`/`activityFn`/`resetsAtFn` seams) makes the launch deterministically testable without a real Claude subprocess.

**Tech Stack:** Go 1.25+, Gin, GORM, PostgreSQL 17; React 19 + TypeScript + Vitest.

## Global Constraints

- `make check` (fmt + vet + lint + test + frontend type-check) MUST pass before any commit is considered done.
- DB-backed Go tests need the `botka_test` database (`make test-db` once). They auto-skip when it is unavailable, so `make test` still passes without it — but locally run `make test-db` first so the new tests actually execute.
- Follow existing patterns: sentinel errors like the runner package's existing ones; handler shape mirrors `KillTask`; frontend mirrors `handleRetry` + `retryTask`.
- Force operates on `queued` tasks only. It bypasses both rate-limit gates but refuses when the runner is `Stopped`, when all workers are busy, or when the task's project already has a running task.
- Every git commit message ends with the trailer:
  `Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>`
- Work happens on branch `feat/force-run-task` (already created). Stage only the files each task touches — the working tree has unrelated pre-existing modifications that must NOT be committed.

## File Map

- `internal/runner/runner.go` — `launchFn` field, `doLaunch`, `SetLaunchHookForTest`, `launchTask` returns `bool`, sentinel errors, `pickTaskByID`, `ForceRunTask`.
- `internal/runner/runner_test.go` — launchTask bool assertions + `ForceRunTask` tests + `newForceRunner` helper.
- `internal/handlers/runner.go` — `ForceRun` handler + route registration + `errors`/`gorm` imports.
- `internal/handlers/runner_test.go` — handler tests (400/404/409/200).
- `frontend/src/api/client.ts` — `forceRunTask`.
- `frontend/src/pages/TaskDetailPage.tsx` — `handleForceRun` + queued "Spustit teď" button + imports.
- `frontend/src/pages/TaskDetailPage.forcerun.test.tsx` — button render + click test (new file).

---

### Task 1: `launchTask` returns bool + launch test seam

**Files:**
- Modify: `internal/runner/runner.go` (struct ~line 87-108; `tick` line 531; `launchTask` lines 678-714)
- Test: `internal/runner/runner_test.go` (lines 226, 279 — existing refusal tests)

**Interfaces:**
- Produces: `launchTask(task *models.Task, execution *models.TaskExecution) bool` (true = goroutine started, false = requeued); `doLaunch(task, execution) bool`; `SetLaunchHookForTest(fn func(*models.Task, *models.TaskExecution) bool)`; unexported field `launchFn func(*models.Task, *models.TaskExecution) bool`.

- [ ] **Step 1: Update the two existing launchTask refusal tests to assert `false`**

In `internal/runner/runner_test.go`, in `TestLaunchTask_RefusesDuplicateProject` change the call at line 226 from:

```go
	// Try to launch second task for the same project.
	r.launchTask(&task2, &models.TaskExecution{TaskID: task2.ID})
```

to:

```go
	// Try to launch second task for the same project.
	if r.launchTask(&task2, &models.TaskExecution{TaskID: task2.ID}) {
		t.Error("expected launchTask to return false when project is busy")
	}
```

And in `TestLaunchTask_RefusesWhenMaxWorkersReached` change the call at line 279 from:

```go
	// Try to launch a third task on a different project.
	r.launchTask(&taskC, &models.TaskExecution{TaskID: taskC.ID})
```

to:

```go
	// Try to launch a third task on a different project.
	if r.launchTask(&taskC, &models.TaskExecution{TaskID: taskC.ID}) {
		t.Error("expected launchTask to return false when workers are full")
	}
```

- [ ] **Step 2: Run the tests to verify they fail to compile**

Run: `go test ./internal/runner/ -run 'TestLaunchTask_Refuses' -v`
Expected: FAIL — compile error `r.launchTask(...) (no value) used as value` (launchTask currently returns nothing).

- [ ] **Step 3: Add the `launchFn` field to the Runner struct**

In `internal/runner/runner.go`, inside `type Runner struct` (after `rateLimitGate *RateLimitGate` at line 107), add:

```go
	// launchFn, when non-nil, replaces launchTask. Test-only seam (like pingFn /
	// activityFn / resetsAtFn) so force-run tests can assert launch behavior
	// without spawning a real Claude subprocess.
	launchFn func(task *models.Task, execution *models.TaskExecution) bool
```

- [ ] **Step 4: Make `launchTask` return bool**

In `internal/runner/runner.go`, change the signature and the three exit points of `launchTask` (lines 678-714):

- Signature: `func (r *Runner) launchTask(task *models.Task, execution *models.TaskExecution) bool {`
- After `r.unclaimTask(task)` in the duplicate-project branch (line 686-687), add `return false` before the closing brace of that `if`.
- After `r.unclaimTask(task)` in the worker-limit branch (line 694-695), add `return false`.
- At the very end of the function (after `go r.executeTask(ctx, task, execution, buf)`, line 713), add `return true`.

The tick call site at line 531 (`r.launchTask(task, execution)`) stays as-is — Go allows ignoring the return value.

- [ ] **Step 5: Add `doLaunch` and `SetLaunchHookForTest`**

In `internal/runner/runner.go`, immediately after `launchTask`, add:

```go
// doLaunch launches a task, honoring a test override if one is installed. Used
// by ForceRunTask so tests can stub the launch; the scheduler's tick calls
// launchTask directly.
func (r *Runner) doLaunch(task *models.Task, execution *models.TaskExecution) bool {
	if r.launchFn != nil {
		return r.launchFn(task, execution)
	}
	return r.launchTask(task, execution)
}

// SetLaunchHookForTest installs a launch override. Intended for tests; production
// code leaves launchFn nil so doLaunch calls launchTask.
func (r *Runner) SetLaunchHookForTest(fn func(task *models.Task, execution *models.TaskExecution) bool) {
	r.launchFn = fn
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/runner/ -run 'TestLaunchTask_Refuses' -v`
Expected: PASS (both tests).

- [ ] **Step 7: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go
git commit -m "$(cat <<'EOF'
feat(runner): launchTask returns bool + add launch test seam

launchTask now reports whether the goroutine actually started (false when
it requeues on a busy project or full workers), so the upcoming
ForceRunTask can return success only on a real launch. Adds a launchFn
override seam mirroring the existing pingFn/activityFn/resetsAtFn seams.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: `ForceRunTask` + `pickTaskByID` + sentinel errors

**Files:**
- Modify: `internal/runner/runner.go` (add near `pickNextTask`/`launchTask`)
- Test: `internal/runner/runner_test.go`

**Interfaces:**
- Consumes: `doLaunch` (Task 1); `claimTask`, `isUniqueViolation`, `buildPickQuery` conventions (existing).
- Produces: `ForceRunTask(taskID uuid.UUID) (*models.Task, error)`; `pickTaskByID(taskID uuid.UUID) (*models.Task, *models.TaskExecution, error)`; exported sentinels `ErrRunnerStopped`, `ErrTaskNotQueued`, `ErrWorkersBusy`, `ErrProjectBusy`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/runner/runner_test.go`:

```go
// newForceRunner builds a StateRunning runner whose gates report "ready" so
// ForceRunTask tests exercise only the force logic, not the gate bypass path.
func newForceRunner(db *gorm.DB) *Runner {
	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = true
	return &Runner{
		db:             db,
		state:          models.StateRunning,
		maxWorkers:     2,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		usageMon:       usage,
		rateLimitGate:  NewRateLimitGate(nil),
		TaskEvents:     NewTaskEventHub(),
	}
}

func TestForceRunTask_LaunchesDespiteActiveGates(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	proj := createProject(t, db, "project-force")
	task := createTask(t, db, proj.ID, "stuck-task", models.TaskStatusQueued)

	// Both gates would block a normal tick: usage never polled OK (rate limited),
	// and the rate-limit gate is paused.
	usage := NewUsageMonitor("", 0.99, 0.99)
	usage.lastPollOK = false // IsRateLimited() == true
	gate := NewRateLimitGate(nil)
	gate.PauseUntil(time.Now().Add(2*time.Hour), "test pause", uuid.New())

	var launched *models.Task
	r := &Runner{
		db:             db,
		state:          models.StateRunning,
		maxWorkers:     2,
		executors:      make(map[uuid.UUID]*activeTask),
		buffers:        make(map[uuid.UUID]*Buffer),
		retryNotBefore: make(map[uuid.UUID]time.Time),
		usageMon:       usage,
		rateLimitGate:  gate,
		TaskEvents:     NewTaskEventHub(),
		launchFn: func(tk *models.Task, _ *models.TaskExecution) bool {
			launched = tk
			return true
		},
	}

	got, err := r.ForceRunTask(task.ID)
	if err != nil {
		t.Fatalf("ForceRunTask: %v", err)
	}
	if got == nil || got.ID != task.ID {
		t.Fatalf("expected returned task %v, got %v", task.ID, got)
	}
	if launched == nil || launched.ID != task.ID {
		t.Fatalf("expected launch hook called with task %v, got %v", task.ID, launched)
	}
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusRunning {
		t.Errorf("expected task running, got %s", reloaded.Status)
	}
}

func TestForceRunTask_RefusesNonQueued(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-nq")
	task := createTask(t, db, proj.ID, "done-task", models.TaskStatusDone)

	called := false
	r := newForceRunner(db)
	r.launchFn = func(*models.Task, *models.TaskExecution) bool { called = true; return true }

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrTaskNotQueued) {
		t.Fatalf("want ErrTaskNotQueued, got %v", err)
	}
	if called {
		t.Error("launch hook must not be called for a non-queued task")
	}
}

func TestForceRunTask_RefusesWhenStopped(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-stopped")
	task := createTask(t, db, proj.ID, "q", models.TaskStatusQueued)

	r := newForceRunner(db)
	r.state = models.StateStopped

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrRunnerStopped) {
		t.Fatalf("want ErrRunnerStopped, got %v", err)
	}
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusQueued {
		t.Errorf("task must stay queued, got %s", reloaded.Status)
	}
}

func TestForceRunTask_RefusesWhenWorkersBusy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	projA := createProject(t, db, "force-a")
	projB := createProject(t, db, "force-b")
	taskA := createTask(t, db, projA.ID, "a", models.TaskStatusRunning)
	task := createTask(t, db, projB.ID, "b", models.TaskStatusQueued)

	r := newForceRunner(db)
	r.maxWorkers = 1
	r.executors[projA.ID] = &activeTask{task: &taskA, execution: &models.TaskExecution{TaskID: taskA.ID}}

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrWorkersBusy) {
		t.Fatalf("want ErrWorkersBusy, got %v", err)
	}
}

func TestForceRunTask_RefusesWhenProjectBusy(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createProject(t, db, "force-proj-busy")
	running := createTask(t, db, proj.ID, "running", models.TaskStatusRunning)
	task := createTask(t, db, proj.ID, "queued", models.TaskStatusQueued)

	r := newForceRunner(db)
	r.executors[proj.ID] = &activeTask{task: &running, execution: &models.TaskExecution{TaskID: running.ID}}

	if _, err := r.ForceRunTask(task.ID); !errors.Is(err, ErrProjectBusy) {
		t.Fatalf("want ErrProjectBusy, got %v", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/runner/ -run 'TestForceRunTask' -v`
Expected: FAIL — compile error `r.ForceRunTask undefined` and `undefined: ErrTaskNotQueued` etc.

- [ ] **Step 3: Add the sentinel errors**

In `internal/runner/runner.go`, near the top after the imports (above `type Runner struct`), add:

```go
// Force-run outcome sentinels, mapped to HTTP status codes by the handler.
var (
	// ErrRunnerStopped is returned when force-run is attempted while the runner
	// is hard-stopped.
	ErrRunnerStopped = errors.New("runner is stopped")
	// ErrTaskNotQueued is returned when the target task is not in the queued
	// state.
	ErrTaskNotQueued = errors.New("task is not queued")
	// ErrWorkersBusy is returned when all worker slots are occupied.
	ErrWorkersBusy = errors.New("all workers are busy")
	// ErrProjectBusy is returned when the task's project already has a running
	// task.
	ErrProjectBusy = errors.New("another task on this project is already running")
)
```

(`errors` is already imported in runner.go.)

- [ ] **Step 4: Add `pickTaskByID`**

In `internal/runner/runner.go`, after `pickNextTask` (ends line 606), add:

```go
// pickTaskByID claims one specific queued task by ID, bypassing the scheduler's
// gate checks and priority ordering. It still enforces one-running-per-project
// via the NOT EXISTS subquery and FOR UPDATE SKIP LOCKED, with the DB unique
// index as the final backstop. Returns (nil, nil, nil) when the task is no
// longer eligible (status changed, or the project already has a running task).
func (r *Runner) pickTaskByID(
	taskID uuid.UUID,
) (*models.Task, *models.TaskExecution, error) {
	tx := r.db.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("begin transaction: %w", tx.Error)
	}
	defer tx.Rollback() //nolint:errcheck // safe no-op after commit

	var task models.Task
	err := tx.
		Where("id = ? AND status = ?", taskID, models.TaskStatusQueued).
		Where("NOT EXISTS (SELECT 1 FROM tasks t2 WHERE t2.project_id = tasks.project_id AND t2.status = ?)", models.TaskStatusRunning).
		Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
		First(&task).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("query task: %w", err)
	}

	t, exec, err := r.claimTask(tx, &task)
	if err != nil {
		if isUniqueViolation(err) {
			return nil, nil, nil
		}
		return nil, nil, err
	}
	return t, exec, nil
}
```

(`clause`, `gorm`, `fmt`, `errors`, `uuid`, `models` are already imported.)

- [ ] **Step 5: Add `ForceRunTask`**

In `internal/runner/runner.go`, after `pickTaskByID`, add:

```go
// ForceRunTask launches a specific queued task immediately, bypassing both
// rate-limit gates (the UsageMonitor 5h/7d threshold and the RateLimitGate
// cooldown). It backs the "Spustit teď" escape hatch for tasks stuck behind an
// exhausted limit. It still honors the hard invariants: the runner must not be
// hard-stopped, a worker slot must be free, and the task's project must have no
// other running task. A forced task runs through the normal executeTask path, so
// if it fails on a genuine rate-limit error the gate re-arms itself.
func (r *Runner) ForceRunTask(taskID uuid.UUID) (*models.Task, error) {
	var task models.Task
	if err := r.db.First(&task, "id = ?", taskID).Error; err != nil {
		return nil, err // gorm.ErrRecordNotFound maps to 404 in the handler
	}
	if task.Status != models.TaskStatusQueued {
		return nil, ErrTaskNotQueued
	}

	// Pre-flight capacity check under the lock (mirrors collectTickState +
	// launchTask). The lock is released before the claim transaction; the DB
	// unique index and launchTask's re-check are the final authority, exactly as
	// in the normal tick path.
	r.mu.Lock()
	if r.state == models.StateStopped {
		r.mu.Unlock()
		return nil, ErrRunnerStopped
	}
	if len(r.executors) >= r.maxWorkers {
		r.mu.Unlock()
		return nil, ErrWorkersBusy
	}
	if _, busy := r.executors[task.ProjectID]; busy {
		r.mu.Unlock()
		return nil, ErrProjectBusy
	}
	r.mu.Unlock()

	claimed, execution, err := r.pickTaskByID(taskID)
	if err != nil {
		return nil, fmt.Errorf("claim task: %w", err)
	}
	if claimed == nil {
		// Lost the race: status changed or the project became busy between the
		// pre-flight check and the claim.
		return nil, ErrProjectBusy
	}
	if !r.doLaunch(claimed, execution) {
		// launchTask requeued it (a worker slot or the project was taken in the
		// gap between the pre-flight check and the launch).
		return nil, ErrProjectBusy
	}
	return claimed, nil
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/runner/ -run 'TestForceRunTask' -v`
Expected: PASS (all five tests).

- [ ] **Step 7: Run the full runner package tests**

Run: `go test ./internal/runner/ -race`
Expected: PASS (`ok  botka/internal/runner`).

- [ ] **Step 8: Commit**

```bash
git add internal/runner/runner.go internal/runner/runner_test.go
git commit -m "$(cat <<'EOF'
feat(runner): ForceRunTask to launch a queued task past rate-limit gates

Adds ForceRunTask + pickTaskByID (a gate-free, by-ID sibling of
pickNextTask) so a single queued task can be launched immediately even
when the UsageMonitor or RateLimitGate would otherwise block the
scheduler. Still refuses when the runner is stopped, workers are full, or
the project already has a running task. Typed sentinels map cleanly to
HTTP status codes.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 3: REST endpoint `POST /api/v1/tasks/:id/force-run`

**Files:**
- Modify: `internal/handlers/runner.go` (imports; `RegisterRunnerRoutes` line 25-33; add handler after `KillTask` line 98)
- Test: `internal/handlers/runner_test.go`

**Interfaces:**
- Consumes: `Runner.ForceRunTask` + sentinels (Task 2); `respondOK`, `respondError`, `NewRunnerForTest`, `SetLaunchHookForTest`.
- Produces: `RunnerHandler.ForceRun(c *gin.Context)`; route `POST /tasks/:id/force-run`.

- [ ] **Step 1: Write the failing tests**

Add to `internal/handlers/runner_test.go` (add `"botka/internal/models"` to the import block):

```go
func TestForceRun_InvalidID(t *testing.T) {
	router := gin.New()
	h := &RunnerHandler{} // nil runner — only parameter parsing is exercised
	router.POST("/api/v1/tasks/:id/force-run", h.ForceRun)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/not-a-uuid/force-run", "")
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForceRun_NotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+uuid.New().String()+"/force-run", "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForceRun_NotQueued(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusDone)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+task.ID.String()+"/force-run", "")
	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestForceRun_Success(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)
	proj := createTestProject(t, db)
	task := createTestTask(t, db, proj.ID, models.TaskStatusQueued)
	usage := runner.NewUsageMonitor("", 0.99, 0.99)
	r := runner.NewRunnerForTest(db, usage, runner.NewRateLimitGate(nil))
	r.SetLaunchHookForTest(func(*models.Task, *models.TaskExecution) bool { return true })
	router := newRunnerTestRouter(r)

	w := doRequest(router, http.MethodPost, "/api/v1/tasks/"+task.ID.String()+"/force-run", "")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var reloaded models.Task
	if err := db.First(&reloaded, "id = ?", task.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Status != models.TaskStatusRunning {
		t.Errorf("expected running, got %s", reloaded.Status)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/handlers/ -run 'TestForceRun' -v`
Expected: FAIL — compile error `h.ForceRun undefined`.

- [ ] **Step 3: Add imports to `internal/handlers/runner.go`**

Change the import block (lines 3-11) to add `errors` and `gorm.io/gorm`:

```go
import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"botka/internal/runner"
)
```

- [ ] **Step 4: Register the route**

In `RegisterRunnerRoutes` (`internal/handlers/runner.go` line 25-33), after `rg.POST("/tasks/:id/kill", h.KillTask)` (line 32), add:

```go
	rg.POST("/tasks/:id/force-run", h.ForceRun)
```

- [ ] **Step 5: Add the `ForceRun` handler**

In `internal/handlers/runner.go`, after `KillTask` (line 98), add:

```go
// ForceRun launches a specific queued task immediately, bypassing the rate-limit
// gates. Backs the "Spustit teď" button for tasks stuck behind an exhausted 5h
// limit.
func (h *RunnerHandler) ForceRun(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid task id")
		return
	}
	task, err := h.runner.ForceRunTask(id)
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			respondError(c, http.StatusNotFound, "task not found")
		case errors.Is(err, runner.ErrTaskNotQueued):
			respondError(c, http.StatusConflict, "task is not queued")
		case errors.Is(err, runner.ErrRunnerStopped):
			respondError(c, http.StatusConflict, "runner is stopped; start it first")
		case errors.Is(err, runner.ErrWorkersBusy):
			respondError(c, http.StatusConflict, "all workers are busy")
		case errors.Is(err, runner.ErrProjectBusy):
			respondError(c, http.StatusConflict, "another task on this project is already running")
		default:
			respondError(c, http.StatusInternalServerError, err.Error())
		}
		return
	}
	respondOK(c, task)
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `go test ./internal/handlers/ -run 'TestForceRun' -v`
Expected: PASS (all four tests; DB-backed ones require `botka_test`).

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/runner.go internal/handlers/runner_test.go
git commit -m "$(cat <<'EOF'
feat(handlers): POST /tasks/:id/force-run endpoint

Maps ForceRunTask outcomes to HTTP: 200 with the now-running task, 404
for a missing task, 409 for not-queued / stopped runner / workers busy /
project busy, 400 for a bad id.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 4: Frontend "Spustit teď" button

**Files:**
- Modify: `frontend/src/api/client.ts` (after `killTask`, line 173-175)
- Modify: `frontend/src/pages/TaskDetailPage.tsx` (imports line 20 + line 31; `handleForceRun` after `handleRetry` line 156; button in the Actions block line 418)
- Test: `frontend/src/pages/TaskDetailPage.forcerun.test.tsx` (new)

**Interfaces:**
- Consumes: `POST /tasks/:id/force-run` (Task 3).
- Produces: `forceRunTask(id: string): Promise<Task>`; a "Spustit teď" button visible only for `queued` tasks.

- [ ] **Step 1: Write the failing frontend test**

Create `frontend/src/pages/TaskDetailPage.forcerun.test.tsx`:

```tsx
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { Task } from '../types'

const { fetchTask, fetchTaskNotes, fetchTaskTags, forceRunTask } = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskNotes: vi.fn(),
  fetchTaskTags: vi.fn(),
  forceRunTask: vi.fn(),
}))

vi.mock('../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/client')>()),
  fetchTask,
  fetchTaskNotes,
  fetchTaskTags,
  forceRunTask,
}))

// The SSE hook needs an EventSource, which jsdom does not provide.
vi.mock('../hooks/useTaskEvents', () => ({ useTaskEvents: () => {} }))

import TaskDetailPage from './TaskDetailPage'

const baseTask = {
  id: 'ef0f448c-6426-4d34-abb2-93464a66ce81',
  title: 'Stuck task',
  spec: 'do the thing',
  status: 'queued',
  priority: 1,
  project_id: 'p1',
  created_at: '2026-07-09T10:00:00Z',
  updated_at: '2026-07-09T10:00:00Z',
} as unknown as Task

function renderPage() {
  return render(
    <MemoryRouter initialEntries={[`/tasks/${baseTask.id}`]}>
      <Routes>
        <Route path="/tasks/:id" element={<TaskDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('TaskDetailPage force run', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchTaskNotes.mockResolvedValue([])
    fetchTaskTags.mockResolvedValue([])
    forceRunTask.mockResolvedValue({ ...baseTask, status: 'running' } as Task)
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows "Spustit teď" for a queued task and calls forceRunTask on click', async () => {
    fetchTask.mockResolvedValue(baseTask)
    renderPage()
    const btn = await screen.findByText('Spustit teď')
    fireEvent.click(btn)
    await waitFor(() => expect(forceRunTask).toHaveBeenCalledWith(baseTask.id))
  })

  it('does not show "Spustit teď" for a non-queued task', async () => {
    fetchTask.mockResolvedValue({ ...baseTask, status: 'done' } as Task)
    renderPage()
    await screen.findByText('Stuck task')
    expect(screen.queryByText('Spustit teď')).toBeNull()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/pages/TaskDetailPage.forcerun.test.tsx`
Expected: FAIL — `forceRunTask` is not an export of `../api/client` and/or the button text is not found.

- [ ] **Step 3: Add `forceRunTask` to the API client**

In `frontend/src/api/client.ts`, after `killTask` (ends line 175), add:

```ts
export function forceRunTask(id: string): Promise<Task> {
  return requestData<Task>(`/tasks/${id}/force-run`, { method: 'POST' })
}
```

- [ ] **Step 4: Wire imports in TaskDetailPage**

In `frontend/src/pages/TaskDetailPage.tsx`:

- Add `Zap` to the `lucide-react` import block (near `Play` on line 20):

```tsx
  Play,
  Zap,
```

- Add `forceRunTask` to the api-client import on line 31:

```tsx
import { fetchTask, retryTask, deleteTask, updateTask, killTask, forceRunTask, fetchTaskRawOutput, regenerateTaskFailureSummary } from '../api/client'
```

- [ ] **Step 5: Add the `handleForceRun` handler**

In `frontend/src/pages/TaskDetailPage.tsx`, after `handleRetry` (ends line 156), add:

```tsx
  async function handleForceRun() {
    setActing(true)
    try {
      await forceRunTask(taskId)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Force run failed')
    } finally {
      setActing(false)
    }
  }
```

- [ ] **Step 6: Add the button to the Actions block**

In `frontend/src/pages/TaskDetailPage.tsx`, inside the Actions block, immediately after `<div className="flex gap-3">` (line 418) and before the `pending` branch (line 419), add:

```tsx
          {task.status === 'queued' && (
            <button
              onClick={handleForceRun}
              disabled={acting}
              title="Obejde rate-limit bránu a spustí task hned"
              className="inline-flex items-center gap-1.5 rounded-md bg-amber-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-700 disabled:opacity-50"
            >
              <Zap className="h-3.5 w-3.5" />
              Spustit teď
            </button>
          )}
```

- [ ] **Step 7: Run the test to verify it passes**

Run: `cd frontend && npx vitest run src/pages/TaskDetailPage.forcerun.test.tsx`
Expected: PASS (both tests).

- [ ] **Step 8: Commit**

```bash
git add frontend/src/api/client.ts frontend/src/pages/TaskDetailPage.tsx frontend/src/pages/TaskDetailPage.forcerun.test.tsx
git commit -m "$(cat <<'EOF'
feat(frontend): "Spustit teď" button to force-run a queued task

Adds forceRunTask API client + an amber button shown only on queued tasks
in the task detail page, wired like handleRetry. Lets the user manually
launch a task stuck behind an exhausted 5h limit.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 5: Full gate + docs

**Files:**
- Modify: `CLAUDE.md` (Important Patterns — one line documenting the escape hatch), optional.

- [ ] **Step 1: Run the full check gate**

Run: `make check`
Expected: PASS (fmt clean, vet clean, lint clean, all Go tests pass, frontend type-check passes).

- [ ] **Step 2: If `make check` passes, document the escape hatch (optional but recommended)**

In `CLAUDE.md`, under **Important Patterns**, add a bullet:

```markdown
- **Force-run escape hatch:** `POST /api/v1/tasks/:id/force-run` (`Runner.ForceRunTask`) launches one queued task immediately, bypassing both rate-limit gates (UsageMonitor 5h/7d threshold and RateLimitGate cooldown). It still honors MAX_WORKERS and one-running-per-project, and refuses when the runner is hard-stopped. Surfaced as the amber "Spustit teď" button on queued tasks in the task detail page. Per-task only — other queued tasks stay gated; a forced task that fails on a real rate limit re-arms the gate via the normal `executeTask` path.
```

- [ ] **Step 3: Commit the docs (only if changed)**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs: document the force-run escape hatch

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Self-Review

**1. Spec coverage:**
- `Runner.ForceRunTask` bypassing both gates → Task 2 (headline test proves it with both gates active). ✅
- Honors MAX_WORKERS / one-per-project / not-stopped / queued-only → Task 2 (four refusal tests). ✅
- `launchTask` returns bool for honest success → Task 1. ✅
- REST `POST /tasks/:id/force-run` with 200/404/409/400 mapping → Task 3. ✅
- Frontend `forceRunTask` + queued-only button → Task 4. ✅
- Out-of-scope items (no MCP, no pending/failed force, no global clear, no migration) → respected; nothing in the plan adds them. ✅
- Testing gate `make check` → Task 5. ✅

**2. Placeholder scan:** No TBD/TODO/"add error handling"/"similar to". Every code step shows complete code. ✅

**3. Type consistency:**
- `ForceRunTask(taskID uuid.UUID) (*models.Task, error)` — defined Task 2, consumed identically in Task 3 handler (`task, err := h.runner.ForceRunTask(id)`). ✅
- `launchTask(...) bool` — defined Task 1, consumed by `doLaunch` and the two updated refusal tests. ✅
- `SetLaunchHookForTest(func(*models.Task, *models.TaskExecution) bool)` — defined Task 1, used in Task 3 `TestForceRun_Success`. ✅
- Sentinels `ErrRunnerStopped`/`ErrTaskNotQueued`/`ErrWorkersBusy`/`ErrProjectBusy` — defined Task 2, matched in Task 2 tests and Task 3 handler switch. ✅
- Frontend `forceRunTask(id: string): Promise<Task>` — defined Task 4 client, used in `handleForceRun` and the test mock. ✅
