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
]

const validFilters = new Set<string>(filters.map((f) => f.value))

export default function TasksPage() {
  useDocumentTitle('Tasks')
  const [searchParams, setSearchParams] = useSearchParams()
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const urlStatus = searchParams.get('status')
  const hasValidUrlParam = urlStatus !== null && validFilters.has(urlStatus)

  const autoSelectedFilter = useRef<Filter | null>(null)

  const { tasks, loading, error, refetch } = useTasks()
  useRefreshOnFocus(refetch)

  // Auto-select most actionable status when no valid URL param is present
  useEffect(() => {
    if (hasValidUrlParam || autoSelectedFilter.current !== null || tasks.length === 0) return
    const hasStatus = (s: TaskStatus) => tasks.some((t) => t.status === s)
    if (hasStatus('running')) autoSelectedFilter.current = 'running'
    else if (hasStatus('pending')) autoSelectedFilter.current = 'pending'
    else if (hasStatus('queued')) autoSelectedFilter.current = 'queued'
    else autoSelectedFilter.current = 'all'
  }, [tasks, hasValidUrlParam])

  const activeFilter: Filter = hasValidUrlParam
    ? (urlStatus as Filter)
    : (autoSelectedFilter.current ?? 'all')

  const counts = useMemo(() => {
    const c: Record<string, number> = { all: tasks.length }
    for (const t of tasks) {
      c[t.status] = (c[t.status] ?? 0) + 1
    }
    return c
  }, [tasks])

  const filtered = useMemo(
    () => (activeFilter === 'all' ? tasks : tasks.filter((t) => t.status === activeFilter)),
    [tasks, activeFilter],
  )

  const handleFilterChange = useCallback(
    (value: Filter) => {
      if (value === 'all') {
        setSearchParams({}, { replace: true })
      } else {
        setSearchParams({ status: value }, { replace: true })
      }
      setSelectedIds(new Set())
    },
    [setSearchParams],
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

      {/* Filter tabs */}
      <div className="flex flex-wrap gap-1 border-b border-zinc-200 pb-px">
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
