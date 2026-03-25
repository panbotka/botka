import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Loader2,
  Clock,
  ListTodo,
  TrendingUp,
  Timer,
  DollarSign,
  FolderOpen,
  RefreshCw,
  Play,
  Pause,
  Square,
} from 'lucide-react'

import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import {
  fetchTaskStats,
  fetchRunnerStatus,
  refreshUsage,
  startRunner,
  pauseRunner,
  stopRunner,
} from '../api/client'
import type {
  TaskStats,
  RunnerStatus as RunnerStatusType,
  UsageInfo,
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

interface StatCardProps {
  label: string
  value: string | number
  icon: React.ReactNode
  color: string
  bgColor: string
  subtitle?: string
  link?: string
}

function StatCard({ label, value, icon, color, bgColor, subtitle, link }: StatCardProps) {
  const content = (
    <div className={clsx(
      'rounded-xl border p-5 transition-shadow',
      'border-zinc-200 bg-white dark:border-zinc-700 dark:bg-zinc-800',
      link && 'hover:shadow-md cursor-pointer',
    )}>
      <div className="flex items-start justify-between">
        <div>
          <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">{label}</p>
          <p className={clsx('mt-1 text-3xl font-bold tabular-nums', color)}>{value}</p>
          {subtitle && (
            <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">{subtitle}</p>
          )}
        </div>
        <div className={clsx('rounded-lg p-2.5', bgColor)}>
          {icon}
        </div>
      </div>
    </div>
  )

  if (link) {
    return <Link to={link}>{content}</Link>
  }
  return content
}

function usageColor(pct: number): string {
  if (pct > 0.8) return 'bg-red-500'
  if (pct > 0.5) return 'bg-amber-500'
  return 'bg-emerald-500'
}

function usageTrackColor(pct: number): string {
  if (pct > 0.8) return 'bg-red-100 dark:bg-red-950'
  if (pct > 0.5) return 'bg-amber-100 dark:bg-amber-950'
  return 'bg-emerald-100 dark:bg-emerald-950'
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
      <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
        <div className="mb-3 flex items-center justify-between">
          <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
            API Usage
          </h2>
          <button onClick={handleRefresh} disabled={refreshing} className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 disabled:opacity-50 dark:hover:bg-zinc-700 dark:hover:text-zinc-300">
            <RefreshCw className={clsx('h-4 w-4', refreshing && 'animate-spin')} />
          </button>
        </div>
        <p className="text-sm text-zinc-400 dark:text-zinc-500">Usage data unavailable</p>
      </div>
    )
  }

  const meters = [
    { label: '5h Window', pct: usage.five_hour_pct, resetLabel: `Resets in ${formatTimeUntil(usage.resets_at)}` },
    { label: '7-day Window', pct: usage.seven_day_pct, resetLabel: null },
  ]

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          API Usage
        </h2>
        <button onClick={handleRefresh} disabled={refreshing} className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 disabled:opacity-50 dark:hover:bg-zinc-700 dark:hover:text-zinc-300">
          <RefreshCw className={clsx('h-4 w-4', refreshing && 'animate-spin')} />
        </button>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        {meters.map((m) => (
          <div key={m.label}>
            <div className="mb-1.5 flex items-baseline justify-between">
              <span className="text-sm font-medium text-zinc-700 dark:text-zinc-300">{m.label}</span>
              <span className="text-sm tabular-nums text-zinc-900 dark:text-zinc-100">
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
              <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">{m.resetLabel}</p>
            )}
          </div>
        ))}
      </div>
    </div>
  )
}

const runnerStateConfig = {
  running: {
    dot: 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.5)]',
    label: 'Running',
  },
  paused: {
    dot: 'bg-amber-400 shadow-[0_0_6px_rgba(251,191,36,0.5)]',
    label: 'Paused',
  },
  stopped: {
    dot: 'bg-red-500',
    label: 'Stopped',
  },
} as const

function RunnerControls({ status, onStart, onPause, onStop, toggling }: {
  status: RunnerStatusType
  onStart: (count?: number) => void
  onPause: () => void
  onStop: () => void
  toggling: boolean
}) {
  const [taskCount, setTaskCount] = useState('')
  const activeCount = status.active_tasks.length
  const cfg = runnerStateConfig[status.state]
  const hasLimit = status.task_limit > 0

  function handleStart() {
    const n = parseInt(taskCount, 10)
    onStart(n > 0 ? n : undefined)
    setTaskCount('')
  }

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <span className={clsx('inline-block h-3 w-3 rounded-full', cfg.dot)} />
          <span className="text-lg font-semibold text-zinc-900 dark:text-zinc-100">{cfg.label}</span>
          {hasLimit && status.state === 'running' && (
            <span className="rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-700 dark:bg-blue-900 dark:text-blue-300">
              {status.completed_count}/{status.task_limit} tasks
            </span>
          )}
          <span className="text-sm text-zinc-500 dark:text-zinc-400">
            {activeCount}/{status.max_workers} active
          </span>
        </div>
        <div className="flex items-center gap-2">
          {status.state !== 'running' && (
            <>
              <input
                type="number"
                min="1"
                placeholder="N"
                value={taskCount}
                onChange={(e) => setTaskCount(e.target.value)}
                onKeyDown={(e) => { if (e.key === 'Enter') handleStart() }}
                className="w-16 rounded-md border border-zinc-300 px-2 py-1.5 text-sm tabular-nums text-zinc-900 placeholder:text-zinc-400 focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100 dark:placeholder:text-zinc-500"
              />
              <button
                type="button"
                disabled={toggling}
                onClick={handleStart}
                className="inline-flex items-center gap-1.5 rounded-md bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white transition-colors hover:bg-emerald-700 disabled:opacity-50"
              >
                <Play className="h-3.5 w-3.5" />
                {taskCount && parseInt(taskCount, 10) > 0 ? `Start ${taskCount}` : 'Start'}
              </button>
            </>
          )}
          {status.state === 'running' && (
            <button
              type="button"
              disabled={toggling}
              onClick={onPause}
              className="inline-flex items-center gap-1.5 rounded-md bg-zinc-100 px-3 py-1.5 text-sm font-medium text-zinc-700 transition-colors hover:bg-zinc-200 disabled:opacity-50 dark:bg-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-600"
            >
              <Pause className="h-3.5 w-3.5" />
              Pause
            </button>
          )}
          {status.state !== 'stopped' && (
            <button
              type="button"
              disabled={toggling}
              onClick={onStop}
              className="inline-flex items-center gap-1.5 rounded-md bg-red-50 px-3 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-100 disabled:opacity-50 dark:bg-red-950 dark:text-red-400 dark:hover:bg-red-900"
            >
              <Square className="h-3.5 w-3.5" />
              Stop
            </button>
          )}
        </div>
      </div>
      {status.active_tasks.length > 0 && (
        <div className="mt-3 space-y-1.5">
          {status.active_tasks.map((t) => (
            <Link
              key={t.task_id}
              to={`/tasks/${t.task_id}`}
              className="flex items-center gap-2 rounded-md px-2 py-1 text-sm hover:bg-zinc-50 dark:hover:bg-zinc-700/50"
            >
              <Loader2 className="h-3.5 w-3.5 animate-spin text-blue-500" />
              <span className="truncate text-zinc-700 dark:text-zinc-300">{t.task_title}</span>
              <span className="ml-auto shrink-0 text-xs text-zinc-400 dark:text-zinc-500">{t.project_name}</span>
            </Link>
          ))}
        </div>
      )}
    </div>
  )
}

export default function DashboardPage() {
  useDocumentTitle('Dashboard')
  const [stats, setStats] = useState<TaskStats | null>(null)
  const [runnerStatus, setRunnerStatus] = useState<RunnerStatusType | null>(null)
  const [loading, setLoading] = useState(true)
  const [toggling, setToggling] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [taskStats, runner] = await Promise.all([
        fetchTaskStats(),
        fetchRunnerStatus(),
      ])
      setStats(taskStats)
      setRunnerStatus(runner)
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

  useRefreshOnFocus(refresh)

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

  if (!stats || !runnerStatus) {
    return (
      <div className="flex h-64 items-center justify-center text-center">
        <div>
          <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">
            {error ? 'Failed to connect' : 'Loading...'}
          </p>
          {error && (
            <p className="mt-1 text-xs text-red-500">{error}</p>
          )}
        </div>
      </div>
    )
  }

  const byStatus = stats.by_status

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100">Dashboard</h1>

      {/* Runner controls */}
      <RunnerControls
        status={runnerStatus}
        onStart={handleStart}
        onPause={handlePause}
        onStop={handleStop}
        toggling={toggling}
      />

      {/* Status cards grid */}
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Running"
          value={byStatus.running ?? 0}
          icon={<Loader2 className="h-5 w-5 text-blue-600 dark:text-blue-400" />}
          color="text-blue-600 dark:text-blue-400"
          bgColor="bg-blue-50 dark:bg-blue-950"
          link="/tasks?status=running"
        />
        <StatCard
          label="Queued"
          value={byStatus.queued ?? 0}
          icon={<Clock className="h-5 w-5 text-indigo-600 dark:text-indigo-400" />}
          color="text-indigo-600 dark:text-indigo-400"
          bgColor="bg-indigo-50 dark:bg-indigo-950"
          link="/tasks?status=queued"
        />
        <StatCard
          label="Done"
          value={byStatus.done ?? 0}
          icon={<CheckCircle2 className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />}
          color="text-emerald-600 dark:text-emerald-400"
          bgColor="bg-emerald-50 dark:bg-emerald-950"
          subtitle={`${stats.completed_today} today / ${stats.completed_week} this week`}
          link="/tasks?status=done"
        />
        <StatCard
          label="Failed"
          value={byStatus.failed ?? 0}
          icon={<XCircle className="h-5 w-5 text-red-600 dark:text-red-400" />}
          color="text-red-600 dark:text-red-400"
          bgColor="bg-red-50 dark:bg-red-950"
          link="/tasks?status=failed"
        />
      </div>

      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
        <StatCard
          label="Needs Review"
          value={byStatus.needs_review ?? 0}
          icon={<AlertTriangle className="h-5 w-5 text-amber-600 dark:text-amber-400" />}
          color="text-amber-600 dark:text-amber-400"
          bgColor="bg-amber-50 dark:bg-amber-950"
          link="/tasks?status=needs_review"
        />
        <StatCard
          label="Pending"
          value={byStatus.pending ?? 0}
          icon={<Clock className="h-5 w-5 text-zinc-500 dark:text-zinc-400" />}
          color="text-zinc-600 dark:text-zinc-400"
          bgColor="bg-zinc-100 dark:bg-zinc-700"
          link="/tasks?status=pending"
        />
        <StatCard
          label="Total Tasks"
          value={stats.total}
          icon={<ListTodo className="h-5 w-5 text-zinc-600 dark:text-zinc-400" />}
          color="text-zinc-900 dark:text-zinc-100"
          bgColor="bg-zinc-100 dark:bg-zinc-700"
          link="/tasks"
        />
        <StatCard
          label="Success Rate"
          value={stats.success_rate != null ? `${Math.round(stats.success_rate * 100)}%` : '-'}
          icon={<TrendingUp className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />}
          color={stats.success_rate != null && stats.success_rate >= 0.7
            ? 'text-emerald-600 dark:text-emerald-400'
            : 'text-amber-600 dark:text-amber-400'}
          bgColor="bg-emerald-50 dark:bg-emerald-950"
        />
      </div>

      {/* Extra insights row */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <StatCard
          label="Avg Duration"
          value={stats.avg_duration_ms != null ? formatDuration(stats.avg_duration_ms) : '-'}
          icon={<Timer className="h-5 w-5 text-violet-600 dark:text-violet-400" />}
          color="text-violet-600 dark:text-violet-400"
          bgColor="bg-violet-50 dark:bg-violet-950"
        />
        <StatCard
          label="Total Cost"
          value={stats.total_cost_usd != null ? `$${stats.total_cost_usd.toFixed(2)}` : '-'}
          icon={<DollarSign className="h-5 w-5 text-green-600 dark:text-green-400" />}
          color="text-green-600 dark:text-green-400"
          bgColor="bg-green-50 dark:bg-green-950"
        />
        {stats.top_project ? (
          <StatCard
            label="Most Active Project"
            value={stats.top_project.name}
            icon={<FolderOpen className="h-5 w-5 text-sky-600 dark:text-sky-400" />}
            color="text-sky-600 dark:text-sky-400"
            bgColor="bg-sky-50 dark:bg-sky-950"
            subtitle={`${stats.top_project.count} tasks`}
            link={`/projects/${stats.top_project.id}`}
          />
        ) : (
          <StatCard
            label="Most Active Project"
            value="-"
            icon={<FolderOpen className="h-5 w-5 text-sky-600 dark:text-sky-400" />}
            color="text-sky-600 dark:text-sky-400"
            bgColor="bg-sky-50 dark:bg-sky-950"
          />
        )}
      </div>

      {/* API Usage */}
      <UsageMeters usage={runnerStatus.usage} onRefresh={async () => {
        const updated = await refreshUsage()
        setRunnerStatus((prev) => prev ? { ...prev, usage: updated } : prev)
      }} />
    </div>
  )
}
