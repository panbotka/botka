import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  Clock,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Loader2,
  RefreshCw,
} from 'lucide-react'

import { RunnerStatus } from '../components/RunnerStatus'
import {
  fetchRunnerStatus,
  fetchTasks,
  startRunner,
  pauseRunner,
  stopRunner,
  refreshUsage,
} from '../api/client'
import type {
  RunnerStatus as RunnerStatusType,
  Task,
  UsageInfo,
  TaskStatus,
} from '../types'

const REFRESH_INTERVAL = 10_000

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  return `${hours}h ${remainMinutes}m`
}

function formatTimeUntil(isoDate: string): string {
  const diff = new Date(isoDate).getTime() - Date.now()
  if (diff <= 0) return 'now'
  const minutes = Math.floor(diff / 60_000)
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  if (hours === 0) return `${remainMinutes}m`
  return `${hours}h ${remainMinutes}m`
}

function formatElapsed(startedAt: string): string {
  const diff = Date.now() - new Date(startedAt).getTime()
  return formatDuration(Math.max(0, diff))
}

function usageColor(pct: number): string {
  if (pct > 0.8) return 'bg-red-500'
  if (pct > 0.5) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function usageTrackColor(pct: number): string {
  if (pct > 0.8) return 'bg-red-100'
  if (pct > 0.5) return 'bg-amber-100'
  return 'bg-emerald-100'
}

const statusConfig: Record<TaskStatus, { icon: typeof CheckCircle2; color: string; label: string }> = {
  done: { icon: CheckCircle2, color: 'text-emerald-600', label: 'Done' },
  failed: { icon: XCircle, color: 'text-red-600', label: 'Failed' },
  needs_review: { icon: AlertTriangle, color: 'text-amber-600', label: 'Review' },
  running: { icon: Loader2, color: 'text-blue-600', label: 'Running' },
  queued: { icon: Clock, color: 'text-zinc-500', label: 'Queued' },
  pending: { icon: Clock, color: 'text-zinc-400', label: 'Pending' },
  cancelled: { icon: XCircle, color: 'text-zinc-400', label: 'Cancelled' },
}

function StatusBadge({ status }: { status: TaskStatus }) {
  const cfg = statusConfig[status]
  const Icon = cfg.icon
  return (
    <span className={clsx('inline-flex items-center gap-1 text-xs font-medium', cfg.color)}>
      <Icon className={clsx('h-3.5 w-3.5', status === 'running' && 'animate-spin')} />
      {cfg.label}
    </span>
  )
}

function UsageMeters({ usage, onRefresh }: { usage: UsageInfo | null; onRefresh: () => Promise<void> }) {
  const [refreshing, setRefreshing] = useState(false)

  async function handleRefresh() {
    setRefreshing(true)
    try {
      await onRefresh()
    } finally {
      setRefreshing(false)
    }
  }

  if (!usage) {
    return (
      <div className="rounded-lg border border-zinc-200 bg-white p-5">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
            API Usage
          </h2>
          <button onClick={handleRefresh} disabled={refreshing} className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 disabled:opacity-50">
            <RefreshCw className={clsx('h-4 w-4', refreshing && 'animate-spin')} />
          </button>
        </div>
        <p className="text-sm text-zinc-400">Usage data unavailable</p>
      </div>
    )
  }

  const meters = [
    { label: '5h Window', pct: usage.five_hour_pct, resetLabel: `Resets in ${formatTimeUntil(usage.resets_at)}` },
    { label: '7-day Window', pct: usage.seven_day_pct, resetLabel: null },
  ]

  return (
    <div className="rounded-lg border border-zinc-200 bg-white p-5">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          API Usage
        </h2>
        <button onClick={handleRefresh} disabled={refreshing} className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 disabled:opacity-50">
          <RefreshCw className={clsx('h-4 w-4', refreshing && 'animate-spin')} />
        </button>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {meters.map((m) => (
          <div key={m.label}>
            <div className="mb-1.5 flex items-baseline justify-between">
              <span className="text-sm font-medium text-zinc-700">{m.label}</span>
              <span className="text-sm tabular-nums text-zinc-900">
                {Math.round(m.pct * 100)}%
              </span>
            </div>
            <div className={clsx('h-2.5 w-full overflow-hidden rounded-full', usageTrackColor(m.pct))}>
              <div
                className={clsx('h-full rounded-full transition-all duration-500', usageColor(m.pct))}
                style={{ width: `${Math.min(100, Math.round(m.pct * 100))}%` }}
              />
            </div>
            {m.resetLabel && (
              <p className="mt-1 text-xs text-zinc-400">{m.resetLabel}</p>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

function ActiveTasks({ tasks }: { tasks: RunnerStatusType['active_tasks'] }) {
  return (
    <div className="rounded-lg border border-zinc-200 bg-white p-5">
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Active Tasks
      </h2>
      {tasks.length === 0 ? (
        <p className="text-sm text-zinc-400">No tasks running</p>
      ) : (
        <ul className="divide-y divide-zinc-100">
          {tasks.map((t) => (
            <li key={t.task_id} className="py-2.5 first:pt-0 last:pb-0">
              <Link
                to={`/tasks/${t.task_id}`}
                className="group block"
              >
                <div className="flex items-center justify-between gap-2">
                  <div className="flex items-center gap-2 overflow-hidden">
                    <StatusBadge status="running" />
                    <span className="truncate text-sm font-medium text-zinc-900 group-hover:text-blue-600">
                      {t.task_title}
                    </span>
                  </div>
                  <span className="shrink-0 text-xs tabular-nums text-zinc-400">
                    {formatElapsed(t.started_at)}
                  </span>
                </div>
                <span className="text-xs text-zinc-500">{t.project_name}</span>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function RecentTasks({ tasks }: { tasks: Task[] }) {
  return (
    <div className="rounded-lg border border-zinc-200 bg-white p-5">
      <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
        Recent Results
      </h2>
      {tasks.length === 0 ? (
        <p className="text-sm text-zinc-400">No completed tasks yet</p>
      ) : (
        <ul className="divide-y divide-zinc-100">
          {tasks.map((t) => {
            const lastExec = t.executions?.[0]
            return (
              <li key={t.id} className="py-2.5 first:pt-0 last:pb-0">
                <Link
                  to={`/tasks/${t.id}`}
                  className="group block"
                >
                  <div className="flex items-center justify-between gap-2">
                    <div className="flex items-center gap-2 overflow-hidden">
                      <StatusBadge status={t.status} />
                      <span className="truncate text-sm font-medium text-zinc-900 group-hover:text-blue-600">
                        {t.title}
                      </span>
                    </div>
                    <div className="flex shrink-0 items-center gap-3 text-xs tabular-nums text-zinc-400">
                      {lastExec?.cost_usd != null && (
                        <span>${lastExec.cost_usd.toFixed(2)}</span>
                      )}
                      {lastExec?.duration_ms != null && (
                        <span>{formatDuration(lastExec.duration_ms)}</span>
                      )}
                    </div>
                  </div>
                  <span className="text-xs text-zinc-500">
                    {t.project?.name ?? 'Unknown project'}
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      )}
    </div>
  )
}

export default function DashboardPage() {
  const [runnerStatus, setRunnerStatus] = useState<RunnerStatusType | null>(null)
  const [recentTasks, setRecentTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [status, recent] = await Promise.all([
        fetchRunnerStatus(),
        fetchTasks({ status: 'done,failed,needs_review', limit: 10 }),
      ])
      setRunnerStatus(status)
      setRecentTasks(recent.data)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Could not connect to the server')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refresh()
    const id = setInterval(refresh, REFRESH_INTERVAL)
    return () => clearInterval(id)
  }, [refresh])

  async function handleStart(count?: number) {
    setToggling(true)
    try {
      await startRunner(count)
      await refresh()
    } finally {
      setToggling(false)
    }
  }

  async function handlePause() {
    setToggling(true)
    try {
      await pauseRunner()
      await refresh()
    } finally {
      setToggling(false)
    }
  }

  async function handleStop() {
    setToggling(true)
    try {
      await stopRunner()
      await refresh()
    } finally {
      setToggling(false)
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  if (!runnerStatus) {
    return (
      <div className="flex h-64 items-center justify-center text-center">
        <div>
          <p className="text-sm font-medium text-zinc-500">
            {error ? 'Failed to connect to the scheduler' : 'Waiting for runner status\u2026'}
          </p>
          {error && (
            <p className="mt-1 text-xs text-red-500">{error}</p>
          )}
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-4xl space-y-5">
      <h1 className="text-2xl font-bold text-zinc-900">Dashboard</h1>

      <RunnerStatus
        status={runnerStatus}
        onStart={handleStart}
        onPause={handlePause}
        onStop={handleStop}
        toggling={toggling}
      />

      <UsageMeters usage={runnerStatus.usage} onRefresh={async () => {
        const updated = await refreshUsage()
        setRunnerStatus((prev) => prev ? { ...prev, usage: updated } : prev)
      }} />

      <div className="grid gap-5 lg:grid-cols-2">
        <ActiveTasks tasks={runnerStatus.active_tasks} />
        <RecentTasks tasks={recentTasks} />
      </div>
    </div>
  )
}
