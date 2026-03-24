import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import { Plus, Loader2 } from 'lucide-react'

import { TaskList } from '../components/TaskList'
import { useTasks } from '../hooks/useTasks'
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

export default function TasksPage() {
  const [activeFilter, setActiveFilter] = useState<Filter>('all')
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())

  const initialFilterSet = useRef(false)

  const { tasks, loading, error, refetch } = useTasks()

  // Set initial tab to the most actionable status
  useEffect(() => {
    if (initialFilterSet.current || tasks.length === 0) return
    initialFilterSet.current = true
    const hasStatus = (s: TaskStatus) => tasks.some((t) => t.status === s)
    if (hasStatus('running')) setActiveFilter('running')
    else if (hasStatus('pending')) setActiveFilter('pending')
    else if (hasStatus('queued')) setActiveFilter('queued')
  }, [tasks])

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

  const handleFilterChange = useCallback((value: Filter) => {
    setActiveFilter(value)
    setSelectedIds(new Set())
  }, [])

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
