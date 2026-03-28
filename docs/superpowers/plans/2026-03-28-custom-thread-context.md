# Custom Context Per Thread Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a free-form text field to each chat thread that gets injected into the context assembly, with a UI editor and MCP tools for programmatic access.

**Architecture:** New `custom_context` TEXT column on `threads` table, injected as a context layer between thread system prompt and URL sources. New `PUT /threads/:id/custom-context` endpoint following the per-field endpoint pattern. New frontend component opened from the thread sidebar context menu. Two new MCP tools (`get_thread_context`, `set_thread_context`).

**Tech Stack:** Go/Gin/GORM backend, PostgreSQL migration, React/TypeScript frontend, MCP JSON-RPC tools.

---

## File Structure

| Action | File | Responsibility |
|--------|------|---------------|
| Create | `migrations/015_thread_custom_context.up.sql` | Add `custom_context` column |
| Create | `migrations/015_thread_custom_context.down.sql` | Remove `custom_context` column |
| Modify | `internal/models/thread.go:12-28` | Add `CustomContext` field to Thread struct |
| Modify | `internal/claude/context.go:70-80` | Add custom context layer between system prompt and sources |
| Modify | `internal/handlers/thread.go:34-50` | Register new `PUT /threads/:id/custom-context` route |
| Modify | `internal/handlers/thread.go` (append) | Add `UpdateCustomContext` handler |
| Modify | `internal/mcp/server.go:213-232` | Register `get_thread_context` and `set_thread_context` handlers |
| Modify | `internal/mcp/server.go:425-471` | Add tool definitions for both new tools |
| Modify | `internal/mcp/tools_thread.go` (append) | Add handler implementations for both tools |
| Modify | `frontend/src/types/index.ts:147-165` | Add `custom_context` to Thread interface |
| Modify | `frontend/src/api/client.ts` (append) | Add `updateCustomContext` API function |
| Create | `frontend/src/components/CustomContextEditor.tsx` | Modal editor component |
| Modify | `frontend/src/components/ThreadSidebar.tsx` | Add "Custom Context" menu item |
| Modify | `internal/handlers/thread_test.go` | Add tests for custom context endpoint |
| Modify | `internal/claude/context_test.go` | Add test for custom context layer |
| Modify | `internal/mcp/server_test.go:107-124` | Add new tools to expected tools map |

---

### Task 1: Database Migration

**Files:**
- Create: `migrations/015_thread_custom_context.up.sql`
- Create: `migrations/015_thread_custom_context.down.sql`

- [ ] **Step 1: Create up migration**

```sql
ALTER TABLE threads ADD COLUMN custom_context TEXT NOT NULL DEFAULT '';
```

- [ ] **Step 2: Create down migration**

```sql
ALTER TABLE threads DROP COLUMN custom_context;
```

- [ ] **Step 3: Verify migration compiles**

Run: `make build`
Expected: Build succeeds (migrations are embedded via golang-migrate, auto-applied on startup)

- [ ] **Step 4: Commit**

```bash
git add migrations/015_thread_custom_context.up.sql migrations/015_thread_custom_context.down.sql
git commit -m "feat: add custom_context column to threads table"
```

---

### Task 2: Thread Model Update

**Files:**
- Modify: `internal/models/thread.go:12-28`

- [ ] **Step 1: Add CustomContext field to Thread struct**

Add after `SystemPrompt` (line 16):

```go
CustomContext   string     `gorm:"type:text;not null;default:''" json:"custom_context"`
```

The struct should now have `SystemPrompt` on line 16 followed by `CustomContext` on line 17.

- [ ] **Step 2: Verify build**

Run: `make build`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/models/thread.go
git commit -m "feat: add CustomContext field to Thread model"
```

---

### Task 3: Context Assembly — Custom Context Layer

**Files:**
- Modify: `internal/claude/context.go:70-80`
- Modify: `internal/claude/context_test.go`

- [ ] **Step 1: Write the failing test**

Add to `internal/claude/context_test.go`:

```go
func TestAssembleContext_CustomContext(t *testing.T) {
	workspace := t.TempDir()
	contextDir := t.TempDir()

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	customCtx := "API endpoint: POST /users\nSchema: id, name, email"

	path, err := AssembleContext(context.Background(), cfg, 50, nil, "Be helpful", customCtx, "project CLAUDE.md content", "myproject", "/home/pi/projects/myproject", nil, nil)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	assembled := string(content)
	if !strings.Contains(assembled, "# Reference Context") {
		t.Error("expected Reference Context section header")
	}
	if !strings.Contains(assembled, "API endpoint: POST /users") {
		t.Error("expected custom context content")
	}

	// Verify ordering: Thread Instructions before Reference Context before Reference Sources
	instrIdx := strings.Index(assembled, "# Thread Instructions")
	refCtxIdx := strings.Index(assembled, "# Reference Context")
	projCtxIdx := strings.Index(assembled, "# Project Context")
	if instrIdx >= refCtxIdx {
		t.Error("expected Thread Instructions before Reference Context")
	}
	if refCtxIdx >= projCtxIdx {
		t.Error("expected Reference Context before Project Context")
	}
}

func TestAssembleContext_EmptyCustomContext(t *testing.T) {
	workspace := t.TempDir()
	contextDir := t.TempDir()

	cfg := ContextConfig{
		OpenClawWorkspace: workspace,
		ContextDir:        contextDir,
	}

	path, err := AssembleContext(context.Background(), cfg, 51, nil, "", "", "", "", "", nil, nil)
	if err != nil {
		t.Fatalf("AssembleContext error: %v", err)
	}

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	if strings.Contains(string(content), "# Reference Context") {
		t.Error("empty custom context should not produce a Reference Context section")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/claude/ -run TestAssembleContext_CustomContext -v`
Expected: FAIL — `AssembleContext` doesn't accept `customContext` parameter yet

- [ ] **Step 3: Update AssembleContext signature and implementation**

In `internal/claude/context.go`, change the function signature on line 40 to add `customContext` parameter after `systemPrompt`:

Old signature:
```go
func AssembleContext(ctx context.Context, cfg ContextConfig, threadID int64, getMemories MemoryFunc, systemPrompt, folderClaudeMD, projectName, projectPath string, sources []SourceInput, messages []models.Message) (string, error) {
```

New signature:
```go
func AssembleContext(ctx context.Context, cfg ContextConfig, threadID int64, getMemories MemoryFunc, systemPrompt, customContext, folderClaudeMD, projectName, projectPath string, sources []SourceInput, messages []models.Message) (string, error) {
```

Then insert a new layer between layer 6 (system prompt) and layer 6b (URL sources). After line 73 (`}`), add:

```go
	// Layer 6a: Custom reference context
	if customContext != "" {
		parts = append(parts, "# Reference Context\n\n"+customContext)
	}
```

- [ ] **Step 4: Update all call sites**

In `internal/handlers/chat.go` line 462, add `thread.CustomContext` after `thread.SystemPrompt`:

Old:
```go
contextFile, err = claude.AssembleContext(c.Request.Context(), h.contextCfg, threadID, getMemories, thread.SystemPrompt, projectClaudeMD, projectName, projectPath, sourceInputs, existingMessages)
```

New:
```go
contextFile, err = claude.AssembleContext(c.Request.Context(), h.contextCfg, threadID, getMemories, thread.SystemPrompt, thread.CustomContext, projectClaudeMD, projectName, projectPath, sourceInputs, existingMessages)
```

- [ ] **Step 5: Update existing tests to match new signature**

Every existing call to `AssembleContext` in `context_test.go` needs an empty string `""` inserted after the `systemPrompt` argument for the new `customContext` parameter. There are 6 calls in the existing tests:

1. `TestAssembleContext_AllLayers` (line 57): add `""` after `systemPrompt`
2. `TestAssembleContext_EmptyWorkspace` (line 111): add `""` after `""`(systemPrompt)
3. `TestAssembleContext_MessageTruncation` (line 141): add `""` after `""`
4. `TestAssembleContext_MessageLimit` (line 178): add `""` after `""`
5. `TestAssembleContext_MemoryFuncError` (line 208): add `""` after `""`
6. `TestAssembleContext_WithSources` (line 232): add `""` after `"Be helpful"`

- [ ] **Step 6: Run all context tests**

Run: `go test ./internal/claude/ -v`
Expected: ALL PASS

- [ ] **Step 7: Commit**

```bash
git add internal/claude/context.go internal/claude/context_test.go internal/handlers/chat.go
git commit -m "feat: add custom context layer to context assembly"
```

---

### Task 4: Backend API — Update Custom Context Endpoint

**Files:**
- Modify: `internal/handlers/thread.go:34-50` (route registration)
- Modify: `internal/handlers/thread.go` (append handler)
- Modify: `internal/handlers/thread_test.go`

- [ ] **Step 1: Write the failing tests**

Add to `internal/handlers/thread_test.go`:

```go
func TestThread_UpdateCustomContext(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := models.Thread{Title: "Test"}
	db.Create(&thread)

	r := threadRouter(db)
	body := `{"custom_context":"API docs: POST /users creates a user"}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/custom-context", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify it was saved
	var updated models.Thread
	db.First(&updated, thread.ID)
	if updated.CustomContext != "API docs: POST /users creates a user" {
		t.Errorf("expected custom context to be saved, got %q", updated.CustomContext)
	}
}

func TestThread_UpdateCustomContextEmpty(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	thread := models.Thread{Title: "Test", CustomContext: "old content"}
	db.Create(&thread)

	r := threadRouter(db)
	body := `{"custom_context":""}`
	w := doRequest(r, http.MethodPut, fmt.Sprintf("/api/v1/threads/%d/custom-context", thread.ID), body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var updated models.Thread
	db.First(&updated, thread.ID)
	if updated.CustomContext != "" {
		t.Errorf("expected empty custom context, got %q", updated.CustomContext)
	}
}

func TestThread_UpdateCustomContextNotFound(t *testing.T) {
	db := setupTestDB(t)
	cleanTables(t, db)

	r := threadRouter(db)
	body := `{"custom_context":"some content"}`
	w := doRequest(r, http.MethodPut, "/api/v1/threads/99999/custom-context", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/handlers/ -run TestThread_UpdateCustomContext -v`
Expected: FAIL — route not registered, 404

- [ ] **Step 3: Add route registration**

In `internal/handlers/thread.go`, add to `RegisterThreadRoutes` (after the tags route on line 49):

```go
	rg.PUT("/threads/:id/custom-context", h.UpdateCustomContext)
```

- [ ] **Step 4: Add handler implementation**

Append to `internal/handlers/thread.go`:

```go
type updateCustomContextRequest struct {
	CustomContext string `json:"custom_context"`
}

// UpdateCustomContext updates a thread's custom reference context.
func (h *ThreadHandler) UpdateCustomContext(c *gin.Context) {
	id, err := paramInt64(c, "id")
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid thread id")
		return
	}

	var thread models.Thread
	if err := h.db.First(&thread, id).Error; err != nil {
		respondError(c, http.StatusNotFound, "thread not found")
		return
	}

	var req updateCustomContextRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.db.Model(&thread).Updates(map[string]interface{}{
		"custom_context": req.CustomContext,
		"updated_at":     time.Now(),
	}).Error; err != nil {
		respondError(c, http.StatusInternalServerError, "failed to update custom context")
		return
	}

	respondOK(c, gin.H{"status": "ok"})
}
```

- [ ] **Step 5: Run tests**

Run: `go test ./internal/handlers/ -run TestThread_UpdateCustomContext -v`
Expected: ALL PASS

- [ ] **Step 6: Commit**

```bash
git add internal/handlers/thread.go internal/handlers/thread_test.go
git commit -m "feat: add PUT /threads/:id/custom-context endpoint"
```

---

### Task 5: MCP Tools — get_thread_context and set_thread_context

**Files:**
- Modify: `internal/mcp/server.go:213-232` (handler registration)
- Modify: `internal/mcp/server.go:425-471` (tool definitions)
- Modify: `internal/mcp/tools_thread.go` (append handlers)
- Modify: `internal/mcp/server_test.go:107-124` (expected tools)

- [ ] **Step 1: Update expected tools in test**

In `internal/mcp/server_test.go`, add two new entries to the `expectedTools` map (after `"update_thread_source": false,` on line 123):

```go
		"get_thread_context":   false,
		"set_thread_context":   false,
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/mcp/ -run TestDispatch_toolsList -v`
Expected: FAIL — missing tool definitions

- [ ] **Step 3: Add tool definitions**

In `internal/mcp/server.go`, append to the `threadToolDefinitions()` return slice (after the `update_thread_source` definition ending around line 470):

```go
		{
			Name:        "get_thread_context",
			Description: "Get a thread's custom reference context",
			InputSchema: schema(map[string]interface{}{
				"thread_id": prop("integer", "Thread ID"),
			}, "thread_id"),
		},
		{
			Name:        "set_thread_context",
			Description: "Set a thread's custom reference context (replaces existing)",
			InputSchema: schema(map[string]interface{}{
				"thread_id": prop("integer", "Thread ID"),
				"content":   prop("string", "Custom context content (replaces existing)"),
			}, "thread_id", "content"),
		},
```

- [ ] **Step 4: Add handler registration**

In `internal/mcp/server.go`, add to the `toolHandlers()` map (after `"update_thread_source": s.handleUpdateThreadSource,` on line 231):

```go
		"get_thread_context":   s.handleGetThreadContext,
		"set_thread_context":   s.handleSetThreadContext,
```

- [ ] **Step 5: Add handler implementations**

Append to `internal/mcp/tools_thread.go`:

```go
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
```

- [ ] **Step 6: Run MCP tests**

Run: `go test ./internal/mcp/ -v`
Expected: ALL PASS

- [ ] **Step 7: Run full test suite**

Run: `make test`
Expected: ALL PASS

- [ ] **Step 8: Commit**

```bash
git add internal/mcp/server.go internal/mcp/tools_thread.go internal/mcp/server_test.go
git commit -m "feat: add get_thread_context and set_thread_context MCP tools"
```

---

### Task 6: Frontend — Type, API Client, and Editor Component

**Files:**
- Modify: `frontend/src/types/index.ts:147-165`
- Modify: `frontend/src/api/client.ts`
- Create: `frontend/src/components/CustomContextEditor.tsx`
- Modify: `frontend/src/components/ThreadSidebar.tsx`

- [ ] **Step 1: Add `custom_context` to Thread type**

In `frontend/src/types/index.ts`, add after `system_prompt` (line 151):

```typescript
  custom_context: string
```

- [ ] **Step 2: Add API function**

Append to `frontend/src/api/client.ts` (near the other thread functions):

```typescript
export function updateCustomContext(threadId: number, customContext: string): Promise<void> {
  return request<void>(`/threads/${threadId}/custom-context`, {
    method: 'PUT',
    body: JSON.stringify({ custom_context: customContext }),
  })
}
```

- [ ] **Step 3: Create CustomContextEditor component**

Create `frontend/src/components/CustomContextEditor.tsx`:

```tsx
import { useState, useEffect, useRef } from 'react'
import { FileText } from 'lucide-react'
import { updateCustomContext } from '../api/client'

interface Props {
  threadId: number
  initialContent: string
  onClose: () => void
  onSaved: () => void
}

export default function CustomContextEditor({ threadId, initialContent, onClose, onSaved }: Props) {
  const [content, setContent] = useState(initialContent)
  const [saving, setSaving] = useState(false)
  const [dirty, setDirty] = useState(false)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    textareaRef.current?.focus()
  }, [])

  const handleSave = async () => {
    if (!dirty) return
    setSaving(true)
    try {
      await updateCustomContext(threadId, content)
      setDirty(false)
      onSaved()
    } finally {
      setSaving(false)
    }
  }

  const handleClose = async () => {
    if (dirty) await handleSave()
    onClose()
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
         onClick={handleClose}>
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col"
           onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
          <div className="flex items-center gap-2">
            <FileText className="w-4 h-4 text-zinc-500" />
            <h2 className="text-sm font-semibold text-zinc-800">Custom Context</h2>
          </div>
          <button onClick={handleClose}
                  className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer">
            Done
          </button>
        </div>

        <div className="px-5 py-3 flex-1 flex flex-col min-h-0">
          <p className="text-xs text-zinc-400 mb-2">
            Reference material included in every message. Paste API docs, schemas, notes, etc.
          </p>
          <textarea
            ref={textareaRef}
            value={content}
            onChange={e => { setContent(e.target.value); setDirty(true) }}
            onBlur={handleSave}
            rows={12}
            placeholder="Paste reference material here..."
            className="flex-1 min-h-[200px] w-full text-sm bg-zinc-50 border border-zinc-200
                       rounded-lg px-3 py-2 text-zinc-800 placeholder-zinc-300
                       focus:border-zinc-400 focus:outline-none resize-y
                       font-mono leading-relaxed"
          />
        </div>

        <div className="px-5 py-3 border-t border-zinc-100 flex items-center justify-between">
          <span className="text-[11px] text-zinc-400">
            {content.length.toLocaleString()} chars
          </span>
          {saving && <span className="text-[11px] text-zinc-400">Saving...</span>}
        </div>
      </div>
    </div>
  )
}
```

- [ ] **Step 4: Add Custom Context menu item to ThreadSidebar**

In `frontend/src/components/ThreadSidebar.tsx`:

4a. Add import at top (line 5, add `FileText` to the lucide import):

```typescript
import {
  Plus, Search, Pin, Archive, MoreVertical, Pencil,
  Trash2, Download, Cpu, Tag as TagIcon, ChevronRight,
  X, ChevronDown, FolderGit2, Globe, FileText,
} from 'lucide-react'
```

4b. Add import for the new component (after line 6):

```typescript
import CustomContextEditor from './CustomContextEditor'
```

4c. Add state for the custom context editor (after `sourcesThreadId` state on line 61):

```typescript
const [customContextThread, setCustomContextThread] = useState<Thread | null>(null)
```

4d. Add a "Custom Context" button in the context menu, right after the "Sources" button (after line 402). Insert before the tags section:

```tsx
                  <button
                    onClick={() => { setCustomContextThread(thread); setMenuOpenId(null) }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <FileText className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    <div className="flex-1 min-w-0 text-left">
                      <div>Custom Context</div>
                      {thread.custom_context && (
                        <div className="text-[11px] text-zinc-400 truncate">
                          {thread.custom_context.length.toLocaleString()} chars
                        </div>
                      )}
                    </div>
                  </button>
```

4e. Add the CustomContextEditor modal render at the end of the component, after the ThreadSourcesEditor render (after line 750, before the closing `</>`):

```tsx
      {customContextThread && (
        <CustomContextEditor
          threadId={customContextThread.id}
          initialContent={customContextThread.custom_context || ''}
          onClose={() => setCustomContextThread(null)}
          onSaved={onThreadsChange}
        />
      )}
```

- [ ] **Step 5: Verify frontend types**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit`
Expected: PASS (no type errors)

- [ ] **Step 6: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/client.ts frontend/src/components/CustomContextEditor.tsx frontend/src/components/ThreadSidebar.tsx
git commit -m "feat: add custom context editor UI for threads"
```

---

### Task 7: Final Verification

- [ ] **Step 1: Run full check**

Run: `make check`
Expected: ALL PASS (fmt + vet + lint + test + frontend type-check)

- [ ] **Step 2: Fix any issues found**

If `make check` reports issues, fix them and re-run.

- [ ] **Step 3: Final commit if needed**

If fixes were needed, commit them separately.
