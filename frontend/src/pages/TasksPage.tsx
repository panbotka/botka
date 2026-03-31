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

  // Poll every 10s while the page is visible
  useEffect(() => {
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') {
        refetchAll()
      }
    }, 10_000)
    return () => clearInterval(id)
  }, [refetchAll])

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
