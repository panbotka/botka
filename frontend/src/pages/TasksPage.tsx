import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { clsx } from 'clsx'
import { Plus, Loader2 } from 'lucide-react'

import { TaskList } from '../components/TaskList'
import { useTasks } from '../hooks/useTasks'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import type { TaskStatus } from '../types'

type Filter = 'all' | TaskStatus

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
  const hasValidUrlParam = urlStatus !== null && validFilters.has(urlStatus)

  const autoSelectedFilter = useRef<Filter | null>(null)

  const isDeletedView = urlStatus === 'deleted'

  // Fetch non-deleted tasks (backend excludes deleted by default)
  const { tasks: mainTasks, loading: mainLoading, error: mainError, refetch: mainRefetch } = useTasks()
  // Fetch deleted tasks separately for the Deleted tab
  const { tasks: deletedTasks, loading: deletedLoading, error: deletedError, refetch: deletedRefetch } = useTasks({ status: 'deleted' })

  const refetch = useCallback(async () => {
    await Promise.all([mainRefetch(), deletedRefetch()])
  }, [mainRefetch, deletedRefetch])

  useRefreshOnFocus(refetch)

  const tasks = isDeletedView ? deletedTasks : mainTasks
  const loading = isDeletedView ? deletedLoading : mainLoading
  const error = isDeletedView ? deletedError : mainError

  // Derive unique project names from main tasks (not deleted)
  const projectNames = useMemo(() => {
    const names = new Set<string>()
    for (const t of mainTasks) {
      const name = t.project_name ?? t.project?.name
      if (name) names.add(name)
    }
    return Array.from(names).sort((a, b) => a.localeCompare(b))
  }, [mainTasks])

  const activeProject = urlProject && projectNames.includes(urlProject) ? urlProject : null

  // Auto-select most actionable status when no valid URL param is present
  useEffect(() => {
    if (hasValidUrlParam || autoSelectedFilter.current !== null || mainTasks.length === 0) return
    const projectTasks = activeProject
      ? mainTasks.filter((t) => (t.project_name ?? t.project?.name) === activeProject)
      : mainTasks
    const hasStatus = (s: TaskStatus) => projectTasks.some((t) => t.status === s)
    if (hasStatus('running')) autoSelectedFilter.current = 'running'
    else if (hasStatus('pending')) autoSelectedFilter.current = 'pending'
    else if (hasStatus('queued')) autoSelectedFilter.current = 'queued'
    else autoSelectedFilter.current = 'all'
  }, [mainTasks, hasValidUrlParam, activeProject])

  const activeFilter: Filter = hasValidUrlParam
    ? (urlStatus as Filter)
    : (autoSelectedFilter.current ?? 'all')

  // Filter tasks by project first, then compute status counts
  const projectFiltered = useMemo(
    () =>
      activeProject
        ? tasks.filter((t) => (t.project_name ?? t.project?.name) === activeProject)
        : tasks,
    [tasks, activeProject],
  )

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: projectFiltered.length, deleted: deletedTasks.length }
    for (const t of projectFiltered) {
      c[t.status] = (c[t.status] ?? 0) + 1
    }
    return c
  }, [projectFiltered, deletedTasks.length])

  const filtered = useMemo(
    () =>
      activeFilter === 'all' || activeFilter === 'deleted'
        ? projectFiltered
        : projectFiltered.filter((t) => t.status === activeFilter),
    [projectFiltered, activeFilter],
  )

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
      updateSearchParams({ status: value === 'all' ? null : value })
      setSelectedIds(new Set())
    },
    [updateSearchParams],
  )

  const handleProjectChange = useCallback(
    (value: string) => {
      updateSearchParams({ project: value || null })
      setSelectedIds(new Set())
    },
    [updateSearchParams],
  )

  const handleSelectionChange = useCallback((ids: Set<string>) => {
    setSelectedIds(ids)
  }, [])

  const handleStatusChange = useCallback(() => {
    refetch()
  }, [refetch])

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
              const count = counts[value] ?? 0
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
              className="ml-auto rounded-md border border-zinc-200 bg-white px-2.5 py-1.5 text-sm text-zinc-700 focus:border-zinc-400 focus:outline-none"
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
        <TaskList
          tasks={filtered}
          onReorder={refetch}
          selectedIds={selectedIds}
          onSelectionChange={handleSelectionChange}
          onStatusChange={handleStatusChange}
        />
      )}
    </div>
  )
}
