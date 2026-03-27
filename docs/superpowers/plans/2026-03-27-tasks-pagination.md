# Tasks List Pagination Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add server-side pagination to the tasks list page with 100 tasks per page, replacing the current client-side filtering approach.

**Architecture:** The backend already supports `limit`, `offset`, `status`, and `project_id` query params. The frontend `useTasks` hook will be updated to pass pagination params. A new `useTaskCounts` hook fetches global per-status counts from `/tasks/stats`. A `Pagination` component renders page numbers with prev/next buttons. URL search params persist the current page.

**Tech Stack:** React 19, TypeScript, Tailwind CSS 4, existing API client

---

## File Structure

| File | Action | Responsibility |
|------|--------|---------------|
| `frontend/src/hooks/useTasks.ts` | Modify | Add `limit`/`offset` params to hook |
| `frontend/src/hooks/useTaskCounts.ts` | Create | Fetch per-status task counts from `/tasks/stats` |
| `frontend/src/components/Pagination.tsx` | Create | Page navigation UI component |
| `frontend/src/pages/TasksPage.tsx` | Modify | Server-side filtering, pagination state, URL params |

---

### Task 1: Update `useTasks` hook to accept `limit` and `offset`

**Files:**
- Modify: `frontend/src/hooks/useTasks.ts`

- [ ] **Step 1: Update the hook interface and implementation**

Open `frontend/src/hooks/useTasks.ts`. Add `limit` and `offset` to `UseTasksFilters` and pass them through to `fetchTasks`. Remove the hardcoded `limit: 500`.

```typescript
import { useEffect, useState, useCallback } from 'react'
import { fetchTasks } from '../api/client'
import type { Task } from '../types'

interface UseTasksFilters {
  status?: string
  project_id?: string
  limit?: number
  offset?: number
}

interface UseTasksResult {
  tasks: Task[]
  total: number
  loading: boolean
  error: string | null
  refetch: () => Promise<void>
}

export function useTasks(filters: UseTasksFilters = {}): UseTasksResult {
  const [tasks, setTasks] = useState<Task[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const refetch = useCallback(async () => {
    try {
      setError(null)
      const params: { status?: string; project_id?: string; limit?: number; offset?: number } = {}
      if (filters.status) params.status = filters.status
      if (filters.project_id) params.project_id = filters.project_id
      if (filters.limit != null) params.limit = filters.limit
      if (filters.offset != null) params.offset = filters.offset
      const result = await fetchTasks(params)
      setTasks(result.data)
      setTotal(result.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tasks')
    } finally {
      setLoading(false)
    }
  }, [filters.status, filters.project_id, filters.limit, filters.offset])

  useEffect(() => {
    setLoading(true)
    refetch()
  }, [refetch])

  return { tasks, total, loading, error, refetch }
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors (existing callers still compile because `limit`/`offset` are optional)

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useTasks.ts
git commit -m "feat: add limit/offset params to useTasks hook"
```

---

### Task 2: Create `useTaskCounts` hook

**Files:**
- Create: `frontend/src/hooks/useTaskCounts.ts`

This hook fetches global per-status task counts from the existing `/tasks/stats` endpoint, plus a deleted count from `fetchTasks({status: 'deleted', limit: 1})`.

- [ ] **Step 1: Create the hook**

```typescript
import { useEffect, useState, useCallback } from 'react'
import { fetchTaskStats, fetchTasks } from '../api/client'

interface TaskCounts {
  all: number
  pending: number
  queued: number
  running: number
  done: number
  failed: number
  needs_review: number
  deleted: number
}

const zeroCounts: TaskCounts = {
  all: 0,
  pending: 0,
  queued: 0,
  running: 0,
  done: 0,
  failed: 0,
  needs_review: 0,
  deleted: 0,
}

export function useTaskCounts(): {
  counts: TaskCounts
  loading: boolean
  refetch: () => Promise<void>
} {
  const [counts, setCounts] = useState<TaskCounts>(zeroCounts)
  const [loading, setLoading] = useState(true)

  const refetch = useCallback(async () => {
    try {
      const [stats, deletedResult] = await Promise.all([
        fetchTaskStats(),
        fetchTasks({ status: 'deleted', limit: 1 }),
      ])
      const s = stats.by_status
      setCounts({
        all: stats.total,
        pending: s.pending ?? 0,
        queued: s.queued ?? 0,
        running: s.running ?? 0,
        done: s.done ?? 0,
        failed: s.failed ?? 0,
        needs_review: s.needs_review ?? 0,
        deleted: deletedResult.total,
      })
    } catch {
      // Keep previous counts on error
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refetch()
  }, [refetch])

  return { counts, loading, refetch }
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/hooks/useTaskCounts.ts
git commit -m "feat: add useTaskCounts hook for status tab counts"
```

---

### Task 3: Create `Pagination` component

**Files:**
- Create: `frontend/src/components/Pagination.tsx`

A reusable pagination component showing page numbers with previous/next buttons.

- [ ] **Step 1: Create the component**

```tsx
import { useCallback } from 'react'
import { clsx } from 'clsx'
import { ChevronLeft, ChevronRight } from 'lucide-react'

interface PaginationProps {
  currentPage: number
  totalPages: number
  onPageChange: (page: number) => void
}

export function Pagination({ currentPage, totalPages, onPageChange }: PaginationProps) {
  if (totalPages <= 1) return null

  const getPageNumbers = useCallback(() => {
    const pages: (number | 'ellipsis')[] = []
    const maxVisible = 7

    if (totalPages <= maxVisible) {
      for (let i = 1; i <= totalPages; i++) pages.push(i)
    } else {
      // Always show first page
      pages.push(1)

      if (currentPage > 3) {
        pages.push('ellipsis')
      }

      // Pages around current
      const start = Math.max(2, currentPage - 1)
      const end = Math.min(totalPages - 1, currentPage + 1)
      for (let i = start; i <= end; i++) {
        pages.push(i)
      }

      if (currentPage < totalPages - 2) {
        pages.push('ellipsis')
      }

      // Always show last page
      pages.push(totalPages)
    }

    return pages
  }, [currentPage, totalPages])

  return (
    <nav className="flex items-center justify-center gap-1 pt-4" aria-label="Pagination">
      <button
        onClick={() => onPageChange(currentPage - 1)}
        disabled={currentPage === 1}
        className={clsx(
          'inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-sm font-medium',
          currentPage === 1
            ? 'cursor-not-allowed text-zinc-300'
            : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900',
        )}
        aria-label="Previous page"
      >
        <ChevronLeft className="h-4 w-4" />
        Prev
      </button>

      {getPageNumbers().map((page, i) =>
        page === 'ellipsis' ? (
          <span key={`ellipsis-${i}`} className="px-2 text-sm text-zinc-400">
            ...
          </span>
        ) : (
          <button
            key={page}
            onClick={() => onPageChange(page)}
            className={clsx(
              'min-w-[2rem] rounded-md px-2 py-1.5 text-sm font-medium tabular-nums',
              page === currentPage
                ? 'bg-zinc-900 text-white'
                : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900',
            )}
            aria-label={`Page ${page}`}
            aria-current={page === currentPage ? 'page' : undefined}
          >
            {page}
          </button>
        ),
      )}

      <button
        onClick={() => onPageChange(currentPage + 1)}
        disabled={currentPage === totalPages}
        className={clsx(
          'inline-flex items-center gap-1 rounded-md px-2 py-1.5 text-sm font-medium',
          currentPage === totalPages
            ? 'cursor-not-allowed text-zinc-300'
            : 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900',
        )}
        aria-label="Next page"
      >
        Next
        <ChevronRight className="h-4 w-4" />
      </button>
    </nav>
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/Pagination.tsx
git commit -m "feat: add Pagination component"
```

---

### Task 4: Rewrite `TasksPage` with server-side pagination

**Files:**
- Modify: `frontend/src/pages/TasksPage.tsx`

This is the largest change. The page switches from loading all tasks client-side to using server-side filtering and pagination.

Key changes:
- Use `useTaskCounts` for tab counts instead of computing from loaded tasks
- Use `useProjects` for the project dropdown instead of deriving from loaded tasks
- Add `page` URL param, compute `offset` from it
- Convert project name to `project_id` for server-side filtering
- Pass `status`, `project_id`, `limit`, `offset` to `useTasks`
- Reset to page 1 when filters change
- Render `Pagination` component below the task list

- [ ] **Step 1: Rewrite TasksPage.tsx**

Replace the entire content of `frontend/src/pages/TasksPage.tsx` with:

```tsx
import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { clsx } from 'clsx'
import { Plus, Loader2 } from 'lucide-react'

import { TaskList } from '../components/TaskList'
import { Pagination } from '../components/Pagination'
import { useTasks } from '../hooks/useTasks'
import { useTaskCounts } from '../hooks/useTaskCounts'
import { useProjects } from '../hooks/useProjects'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import type { TaskStatus } from '../types'

type Filter = 'all' | TaskStatus

const PAGE_SIZE = 100

const filters: { value: Filter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'pending', label: 'Pending' },
  { value: 'queued', label: 'Queued' },
  { value: 'running', label: 'Running' },
  { value: 'done', label: 'Done' },
  { value: 'failed', label: 'Failed' },
  { value: 'needs_review', label: 'Needs Review' },
  { value: 'deleted', label: 'Deleted' },
]

const validFilters = new Set<string>(filters.map((f) => f.value))

export default function TasksPage() {
  useDocumentTitle('Tasks')
  const [searchParams, setSearchParams] = useSearchParams()
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const urlStatus = searchParams.get('status')
  const urlProject = searchParams.get('project')
  const urlPage = searchParams.get('page')
  const hasValidUrlParam = urlStatus !== null && validFilters.has(urlStatus)

  const autoSelectedFilter = useRef<Filter | null>(null)

  // Parse current page from URL (1-based)
  const currentPage = Math.max(1, parseInt(urlPage ?? '1', 10) || 1)

  // Fetch projects for dropdown and name→ID mapping
  const { projects } = useProjects()

  const projectNames = useMemo(
    () => projects.map((p) => p.name).sort((a, b) => a.localeCompare(b)),
    [projects],
  )

  const activeProject = urlProject && projectNames.includes(urlProject) ? urlProject : null

  // Map project name to project ID for server-side filtering
  const activeProjectId = useMemo(() => {
    if (!activeProject) return undefined
    const proj = projects.find((p) => p.name === activeProject)
    return proj?.id
  }, [activeProject, projects])

  // Determine the active status filter
  const activeFilter: Filter = hasValidUrlParam
    ? (urlStatus as Filter)
    : (autoSelectedFilter.current ?? 'all')

  // Build server-side filter params
  const apiStatus = activeFilter === 'all' ? undefined : activeFilter
  const offset = (currentPage - 1) * PAGE_SIZE

  // Fetch paginated tasks with server-side filters
  const { tasks, total, loading, error, refetch } = useTasks({
    status: apiStatus,
    project_id: activeProjectId,
    limit: PAGE_SIZE,
    offset,
  })

  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  // Fetch tab counts from stats endpoint
  const { counts, refetch: refetchCounts } = useTaskCounts()

  // Auto-select most actionable status when no valid URL param is present
  useEffect(() => {
    if (hasValidUrlParam || autoSelectedFilter.current !== null) return
    if (counts.running > 0) autoSelectedFilter.current = 'running'
    else if (counts.queued > 0) autoSelectedFilter.current = 'queued'
    else if (counts.pending > 0) autoSelectedFilter.current = 'pending'
    else autoSelectedFilter.current = 'all'
  }, [counts, hasValidUrlParam])

  const refetchAll = useCallback(async () => {
    await Promise.all([refetch(), refetchCounts()])
  }, [refetch, refetchCounts])

  useRefreshOnFocus(refetchAll)

  const updateSearchParams = useCallback(
    (updates: Record<string, string | null>) => {
      const next: Record<string, string> = {}
      const current = Object.fromEntries(searchParams.entries())
      for (const [k, v] of Object.entries({ ...current, ...updates })) {
        if (v !== null && v !== undefined) next[k] = v
      }
      setSearchParams(next, { replace: true })
    },
    [searchParams, setSearchParams],
  )

  const handleFilterChange = useCallback(
    (value: Filter) => {
      updateSearchParams({ status: value === 'all' ? null : value, page: null })
      setSelectedIds(new Set())
    },
    [updateSearchParams],
  )

  const handleProjectChange = useCallback(
    (value: string) => {
      updateSearchParams({ project: value || null, page: null })
      setSelectedIds(new Set())
    },
    [updateSearchParams],
  )

  const handlePageChange = useCallback(
    (page: number) => {
      updateSearchParams({ page: page <= 1 ? null : String(page) })
      setSelectedIds(new Set())
    },
    [updateSearchParams],
  )

  const handleSelectionChange = useCallback((ids: Set<string>) => {
    setSelectedIds(ids)
  }, [])

  const handleStatusChange = useCallback(() => {
    refetchAll()
  }, [refetchAll])

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900">Tasks</h1>
        <Link
          to="/tasks/new"
          className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800"
        >
          <Plus className="h-4 w-4" />
          New Task
        </Link>
      </div>

      {/* Filters */}
      <div className="space-y-3">
        <div className="flex flex-wrap items-center gap-3 border-b border-zinc-200 pb-px">
          <div className="flex flex-wrap gap-1">
            {filters.map(({ value, label }) => {
              const count = counts[value as keyof typeof counts] ?? 0
              return (
                <button
                  key={value}
                  onClick={() => handleFilterChange(value)}
                  className={clsx(
                    'inline-flex items-center gap-1.5 rounded-t-md border-b-2 px-3 py-2 text-sm font-medium transition-colors',
                    activeFilter === value
                      ? 'border-zinc-900 text-zinc-900'
                      : 'border-transparent text-zinc-500 hover:border-zinc-300 hover:text-zinc-700',
                  )}
                >
                  {label}
                  <span
                    className={clsx(
                      'rounded-full px-1.5 py-0.5 text-xs tabular-nums',
                      activeFilter === value
                        ? 'bg-zinc-900 text-white'
                        : 'bg-zinc-100 text-zinc-500',
                    )}
                  >
                    {count}
                  </span>
                </button>
              )
            })}
          </div>
          {projectNames.length > 1 && (
            <select
              value={activeProject ?? ''}
              onChange={(e) => handleProjectChange(e.target.value)}
              className="ml-auto rounded-md border border-zinc-200 bg-white dark:bg-zinc-100 px-2.5 py-1.5 text-sm text-zinc-700 focus:border-zinc-400 focus:outline-none"
            >
              <option value="">All projects</option>
              {projectNames.map((name) => (
                <option key={name} value={name}>
                  {name}
                </option>
              ))}
            </select>
          )}
        </div>
      </div>

      {/* Content */}
      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
        </div>
      ) : error ? (
        <div className="flex h-48 items-center justify-center">
          <p className="text-sm text-red-500">{error}</p>
        </div>
      ) : (
        <>
          <TaskList
            tasks={tasks}
            onReorder={refetchAll}
            selectedIds={selectedIds}
            onSelectionChange={handleSelectionChange}
            onStatusChange={handleStatusChange}
          />
          <Pagination
            currentPage={currentPage}
            totalPages={totalPages}
            onPageChange={handlePageChange}
          />
        </>
      )}
    </div>
  )
}
```

- [ ] **Step 2: Verify TypeScript compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 3: Verify the full check passes**

Run: `cd /home/pi/projects/botka && make check`
Expected: All Go tests pass, frontend type-check passes, lint passes.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/pages/TasksPage.tsx
git commit -m "feat: add server-side pagination to tasks list page

Switch from loading all tasks client-side to server-side filtering
with limit/offset pagination. Status and project filters are sent
as query params. Tab counts come from /tasks/stats endpoint.
Current page persists in URL search params for browser navigation."
```
