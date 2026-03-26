# Max Workers Settings Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Allow users to change the maximum number of concurrent task workers from the Settings page, persisted in a DB-backed key-value table.

**Architecture:** New `app_settings` key-value table stores server-side settings. A Settings handler serves GET/PUT endpoints. The Runner gains a `maxWorkers` field (separate from config) initialized from DB on startup and updated via `SetMaxWorkers()`. The frontend Settings page gets a "Task Runner" section, and the RunnerStatus component shows a draining indicator.

**Tech Stack:** Go/Gin/GORM backend, PostgreSQL migration, React/TypeScript frontend

---

### Task 1: Database Migration for app_settings Table

**Files:**
- Create: `migrations/006_app_settings.up.sql`
- Create: `migrations/006_app_settings.down.sql`

- [ ] **Step 1: Create the up migration**

```sql
-- migrations/006_app_settings.up.sql
CREATE TABLE app_settings (
    key VARCHAR(100) PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO app_settings (key, value) VALUES ('max_workers', '2');
```

- [ ] **Step 2: Create the down migration**

```sql
-- migrations/006_app_settings.down.sql
DROP TABLE IF EXISTS app_settings;
```

- [ ] **Step 3: Commit**

```bash
git add migrations/006_app_settings.up.sql migrations/006_app_settings.down.sql
git commit -m "feat: add app_settings migration for server-side settings"
```

---

### Task 2: Setting Model and Helpers

**Files:**
- Create: `internal/models/setting.go`
- Create: `internal/models/setting_test.go`

- [ ] **Step 1: Write tests for the Setting model**

Create `internal/models/setting_test.go`:

```go
package models

import "testing"

func TestSetting_TableName(t *testing.T) {
	s := Setting{}
	if s.TableName() != "app_settings" {
		t.Errorf("expected table name 'app_settings', got %q", s.TableName())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /home/pi/projects/botka && go test ./internal/models/ -run TestSetting_TableName -v`
Expected: FAIL — `Setting` type not defined.

- [ ] **Step 3: Write the Setting model**

Create `internal/models/setting.go`:

```go
package models

import "time"

// Setting stores a server-side configuration key-value pair.
type Setting struct {
	Key       string    `gorm:"column:key;primaryKey;size:100" json:"key"`
	Value     string    `gorm:"column:value;not null" json:"value"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the database table name for the Setting model.
func (Setting) TableName() string {
	return "app_settings"
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd /home/pi/projects/botka && go test ./internal/models/ -run TestSetting_TableName -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/models/setting.go internal/models/setting_test.go
git commit -m "feat: add Setting model for app_settings table"
```

---

### Task 3: Settings Handler (GET + PUT endpoints)

**Files:**
- Create: `internal/handlers/settings.go`
- Create: `internal/handlers/settings_test.go`
- Modify: `internal/handlers/testhelpers_test.go` (add app_settings table setup)
- Modify: `cmd/server/main.go:142-194` (register settings routes)

- [ ] **Step 1: Write handler tests**

Create `internal/handlers/settings_test.go`:

```go
package handlers

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func settingsRouter(db *gorm.DB) *gin.Engine {
	r := gin.New()
	h := NewSettingsHandler(db)
	v1 := r.Group("/api/v1")
	RegisterSettingsRoutes(v1, h)
	return r
}

func seedSettings(t *testing.T, db *gorm.DB) {
	t.Helper()
	db.Exec("DELETE FROM app_settings")
	db.Exec("INSERT INTO app_settings (key, value, updated_at) VALUES ('max_workers', '2', NOW())")
}

func TestSettings_Get(t *testing.T) {
	db := setupTestDB(t)
	seedSettings(t, db)

	r := settingsRouter(db)
	w := doRequest(r, http.MethodGet, "/api/v1/settings", "")

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)

	mw, ok := resp.Data["max_workers"]
	if !ok {
		t.Fatal("expected max_workers in response")
	}
	// JSON numbers decode as float64
	if mw.(float64) != 2 {
		t.Errorf("expected max_workers=2, got %v", mw)
	}
}

func TestSettings_UpdateMaxWorkers(t *testing.T) {
	db := setupTestDB(t)
	seedSettings(t, db)

	r := settingsRouter(db)
	w := doRequest(r, http.MethodPut, "/api/v1/settings", `{"max_workers": 5}`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data map[string]interface{} `json:"data"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Data["max_workers"].(float64) != 5 {
		t.Errorf("expected max_workers=5, got %v", resp.Data["max_workers"])
	}
}

func TestSettings_UpdateMaxWorkers_InvalidLow(t *testing.T) {
	db := setupTestDB(t)
	seedSettings(t, db)

	r := settingsRouter(db)
	w := doRequest(r, http.MethodPut, "/api/v1/settings", `{"max_workers": 0}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSettings_UpdateMaxWorkers_InvalidHigh(t *testing.T) {
	db := setupTestDB(t)
	seedSettings(t, db)

	r := settingsRouter(db)
	w := doRequest(r, http.MethodPut, "/api/v1/settings", `{"max_workers": 11}`)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Add app_settings table to test helpers**

In `internal/handlers/testhelpers_test.go`, add the app_settings table creation to `setupTestDB` (after the runner_state table creation, around line 73), and add it to the `cleanTables` TRUNCATE list.

Add after the `runner_state` table creation block:

```go
				sharedDB.Exec(`CREATE TABLE IF NOT EXISTS app_settings (
					key VARCHAR(100) PRIMARY KEY,
					value TEXT NOT NULL,
					updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
				)`)
```

Add `app_settings` to the TRUNCATE statement in `cleanTables`:

```go
db.Exec("TRUNCATE TABLE thread_tags, branch_selections, attachments, messages, task_executions, tasks, threads, projects, personas, tags, memories, runner_state, app_settings CASCADE")
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd /home/pi/projects/botka && go test ./internal/handlers/ -run TestSettings -v`
Expected: FAIL — `NewSettingsHandler` not defined.

- [ ] **Step 4: Write the settings handler**

Create `internal/handlers/settings.go`:

```go
package handlers

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"botka/internal/models"
)

// SettingsHandler handles HTTP requests for server-side settings.
type SettingsHandler struct {
	db       *gorm.DB
	onChange func(key, value string) // optional callback when a setting changes
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(db *gorm.DB) *SettingsHandler {
	return &SettingsHandler{db: db}
}

// SetOnChange sets a callback invoked when a setting is updated.
func (h *SettingsHandler) SetOnChange(fn func(key, value string)) {
	h.onChange = fn
}

// RegisterSettingsRoutes registers settings routes on the given router group.
func RegisterSettingsRoutes(rg *gin.RouterGroup, h *SettingsHandler) {
	rg.GET("/settings", h.Get)
	rg.PUT("/settings", h.Update)
}

// Get returns all server-side settings as a typed JSON object.
func (h *SettingsHandler) Get(c *gin.Context) {
	var settings []models.Setting
	if err := h.db.Find(&settings).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to load settings")
		return
	}

	result := make(map[string]interface{})
	for _, s := range settings {
		switch s.Key {
		case "max_workers":
			n, _ := strconv.Atoi(s.Value)
			result[s.Key] = n
		default:
			result[s.Key] = s.Value
		}
	}
	respondOK(c, result)
}

// Update accepts a partial settings update.
func (h *SettingsHandler) Update(c *gin.Context) {
	var body struct {
		MaxWorkers *int `json:"max_workers"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if body.MaxWorkers != nil {
		n := *body.MaxWorkers
		if n < 1 || n > 10 {
			respondError(c, http.StatusBadRequest, "max_workers must be between 1 and 10")
			return
		}
		if err := h.db.Where("key = ?", "max_workers").Assign(models.Setting{
			Value: strconv.Itoa(n),
		}).FirstOrCreate(&models.Setting{Key: "max_workers"}).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to save setting")
			return
		}
		if h.onChange != nil {
			h.onChange("max_workers", strconv.Itoa(n))
		}
	}

	// Return current state
	h.Get(c)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd /home/pi/projects/botka && go test ./internal/handlers/ -run TestSettings -v`
Expected: PASS

- [ ] **Step 6: Register settings routes in main.go**

In `cmd/server/main.go`, after the status handler registration (around line 193), add:

```go
	settingsHandler := handlers.NewSettingsHandler(db)
	handlers.RegisterSettingsRoutes(v1, settingsHandler)
```

- [ ] **Step 7: Commit**

```bash
git add internal/handlers/settings.go internal/handlers/settings_test.go internal/handlers/testhelpers_test.go cmd/server/main.go
git commit -m "feat: add server-side settings API endpoints (GET/PUT /api/v1/settings)"
```

---

### Task 4: Runner maxWorkers Field with DB Init and SetMaxWorkers

**Files:**
- Modify: `internal/runner/runner.go:62-96` (Runner struct + NewRunner)
- Modify: `internal/runner/runner.go:206-229` (GetStatus)
- Modify: `internal/runner/runner.go:287-293` (collectTickState)

- [ ] **Step 1: Add maxWorkers field to Runner struct**

In `internal/runner/runner.go`, add a `maxWorkers` field to the Runner struct (after `completedCount int`):

```go
	maxWorkers     int
```

- [ ] **Step 2: Initialize maxWorkers from DB in NewRunner**

In `NewRunner`, after `r.state = r.loadState()` (line 94), add:

```go
	r.maxWorkers = cfg.MaxWorkers // default from env
	r.loadMaxWorkersFromDB()
```

Add the `loadMaxWorkersFromDB` method:

```go
// loadMaxWorkersFromDB reads the max_workers setting from the app_settings table.
func (r *Runner) loadMaxWorkersFromDB() {
	if r.db == nil {
		return
	}
	var value string
	err := r.db.Table("app_settings").Where("key = ?", "max_workers").Pluck("value", &value).Error
	if err != nil || value == "" {
		return
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 1 {
		return
	}
	r.maxWorkers = n
}
```

Add `"strconv"` to the import block.

- [ ] **Step 3: Add SetMaxWorkers method**

```go
// SetMaxWorkers updates the maximum number of concurrent task workers.
// Thread-safe: acquires the mutex before updating.
func (r *Runner) SetMaxWorkers(n int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.maxWorkers = n
	slog.Info("max workers updated", "max_workers", n)
}
```

- [ ] **Step 4: Update GetStatus to use r.maxWorkers and add Draining**

In the `Status` struct, add the `Draining` field:

```go
type Status struct {
	State          models.RunnerStateType `json:"state"`
	ActiveTasks    []ActiveTaskInfo       `json:"active_tasks"`
	MaxWorkers     int                    `json:"max_workers"`
	Draining       bool                   `json:"draining"`
	Usage          *UsageInfo             `json:"usage,omitempty"`
	TaskLimit      int                    `json:"task_limit"`
	CompletedCount int                    `json:"completed_count"`
}
```

Update `GetStatus` to use `r.maxWorkers` instead of `r.config.MaxWorkers`:

```go
	return Status{
		State:          r.state,
		ActiveTasks:    tasks,
		MaxWorkers:     r.maxWorkers,
		Draining:       len(r.executors) > r.maxWorkers,
		Usage:          &usage,
		TaskLimit:      r.taskLimit,
		CompletedCount: r.completedCount,
	}
```

- [ ] **Step 5: Update collectTickState to use r.maxWorkers**

Change line 291 from:

```go
	if r.state != models.StateRunning || len(r.executors) >= r.config.MaxWorkers {
```

to:

```go
	if r.state != models.StateRunning || len(r.executors) >= r.maxWorkers {
```

- [ ] **Step 6: Run all tests**

Run: `cd /home/pi/projects/botka && make test`
Expected: All tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/runner/runner.go
git commit -m "feat: runner reads max_workers from DB with SetMaxWorkers support"
```

---

### Task 5: Wire Settings onChange to Runner.SetMaxWorkers

**Files:**
- Modify: `cmd/server/main.go` (wire onChange callback)

- [ ] **Step 1: Wire the onChange callback in setupRouter**

In `cmd/server/main.go`, update the settings handler registration to wire the callback. Replace the two lines added in Task 3 Step 6 with:

```go
	settingsHandler := handlers.NewSettingsHandler(db)
	settingsHandler.SetOnChange(func(key, value string) {
		if key == "max_workers" {
			n, err := strconv.Atoi(value)
			if err == nil {
				taskRunner.SetMaxWorkers(n)
			}
		}
	})
	handlers.RegisterSettingsRoutes(v1, settingsHandler)
```

Add `"strconv"` to the imports in `cmd/server/main.go`.

- [ ] **Step 2: Run full check**

Run: `cd /home/pi/projects/botka && make check`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat: wire settings onChange to Runner.SetMaxWorkers for live updates"
```

---

### Task 6: Frontend API Client + Types

**Files:**
- Modify: `frontend/src/api/client.ts` (add settings API functions)
- Modify: `frontend/src/types/index.ts` (add ServerSettings type, draining field)

- [ ] **Step 1: Add ServerSettings type**

In `frontend/src/types/index.ts`, add after the `RunnerStatus` interface:

```typescript
export interface ServerSettings {
  max_workers: number
}
```

Add `draining` field to the `RunnerStatus` interface:

```typescript
export interface RunnerStatus {
  state: RunnerStateValue
  active_tasks: ActiveTaskInfo[]
  max_workers: number
  draining: boolean
  usage: UsageInfo | null
  task_limit: number
  completed_count: number
}
```

- [ ] **Step 2: Add API functions**

In `frontend/src/api/client.ts`, add at the end of the file (before the last closing block or at the bottom), with the appropriate import for `ServerSettings`:

```typescript
// Server settings

export function fetchServerSettings(): Promise<ServerSettings> {
  return requestData<ServerSettings>('/settings')
}

export function updateServerSettings(settings: Partial<ServerSettings>): Promise<ServerSettings> {
  return requestData<ServerSettings>('/settings', {
    method: 'PUT',
    body: JSON.stringify(settings),
  })
}
```

Add `ServerSettings` to the type import at the top of `client.ts`.

- [ ] **Step 3: Run type check**

Run: `cd /home/pi/projects/botka && cd frontend && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/client.ts
git commit -m "feat: add server settings API client and types"
```

---

### Task 7: Settings Page — Task Runner Section

**Files:**
- Modify: `frontend/src/pages/SettingsPage.tsx` (add Task Runner tab)

- [ ] **Step 1: Add 'runner' tab to TABS and TabId**

Update the `TabId` type to include `'runner'`:

```typescript
type TabId = 'general' | 'runner' | 'personas' | 'tags' | 'memories' | 'voice'
```

Add a tab entry to `TABS` array after the 'general' entry. Import `Cpu` from lucide-react:

```typescript
  { id: 'runner', label: 'Task Runner', icon: Cpu },
```

- [ ] **Step 2: Add RunnerTab component**

Add the `RunnerTab` component before the `// ── Main Settings Page ──` comment. Import `fetchServerSettings` and `updateServerSettings` from the API client:

```typescript
// ── Runner Tab ──

function RunnerTab() {
  const [maxWorkers, setMaxWorkers] = useState<number | null>(null)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState('')

  useEffect(() => {
    fetchServerSettings()
      .then((s) => setMaxWorkers(s.max_workers))
      .catch(() => setError('Failed to load settings'))
  }, [])

  async function handleChange(value: number) {
    if (value < 1 || value > 10) return
    setMaxWorkers(value)
    setSaving(true)
    setError('')
    try {
      const updated = await updateServerSettings({ max_workers: value })
      setMaxWorkers(updated.max_workers)
    } catch {
      setError('Failed to save setting')
    } finally {
      setSaving(false)
    }
  }

  if (maxWorkers === null && !error) {
    return (
      <div className="flex items-center gap-2 text-sm text-zinc-500">
        <Loader2 className="h-4 w-4 animate-spin" />
        Loading...
      </div>
    )
  }

  return (
    <div className="space-y-6">
      <div>
        <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">
          Max Workers
        </label>
        <p className="mt-0.5 text-xs text-zinc-500 dark:text-zinc-400">
          Maximum concurrent task execution slots (1–10)
        </p>
        <div className="mt-2 flex items-center gap-3">
          <input
            type="number"
            min={1}
            max={10}
            value={maxWorkers ?? 2}
            onChange={(e) => {
              const n = parseInt(e.target.value, 10)
              if (!isNaN(n)) handleChange(n)
            }}
            className="w-20 rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm tabular-nums text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
          />
          {saving && <Loader2 className="h-4 w-4 animate-spin text-zinc-400" />}
        </div>
        {error && <p className="mt-1 text-xs text-red-500">{error}</p>}
      </div>
    </div>
  )
}
```

- [ ] **Step 3: Add RunnerTab rendering in the tab content section**

In the tab content section (around line 1072), add:

```typescript
        {activeTab === 'runner' && <RunnerTab />}
```

- [ ] **Step 4: Add imports**

Add `fetchServerSettings` and `updateServerSettings` to the imports from `'../api/client'`. Add `Cpu` to the lucide-react imports.

- [ ] **Step 5: Run type check**

Run: `cd /home/pi/projects/botka && cd frontend && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add frontend/src/pages/SettingsPage.tsx
git commit -m "feat: add Task Runner tab to Settings page with max workers control"
```

---

### Task 8: Dashboard Draining Indicator

**Files:**
- Modify: `frontend/src/components/RunnerStatus.tsx` (add draining badge)

- [ ] **Step 1: Add draining indicator to RunnerStatus component**

In `frontend/src/components/RunnerStatus.tsx`, find the active count display (line 52-54):

```tsx
          <span className="text-sm text-zinc-500">
            {activeCount}/{status.max_workers} active
          </span>
```

Replace with:

```tsx
          <span className="text-sm text-zinc-500">
            {activeCount}/{status.max_workers} active
          </span>
          {status.draining && (
            <span className="rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-medium text-amber-700">
              draining
            </span>
          )}
```

- [ ] **Step 2: Run type check**

Run: `cd /home/pi/projects/botka && cd frontend && npx tsc --noEmit`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/RunnerStatus.tsx
git commit -m "feat: add draining badge to runner status when active > max workers"
```

---

### Task 9: Full Validation

- [ ] **Step 1: Run make check**

Run: `cd /home/pi/projects/botka && make check`
Expected: All linting, formatting, vetting, tests, and frontend type checks pass.

- [ ] **Step 2: Fix any issues found**

If any failures, fix them and re-run.

- [ ] **Step 3: Final commit (if any fixes needed)**

Commit any remaining fixes.
