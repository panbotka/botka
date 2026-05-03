import { useState } from 'react'
import { Play, Pause, Square, AlertTriangle, X } from 'lucide-react'
import { clsx } from 'clsx'
import type { RunnerStatus as RunnerStatusType } from '../types'

interface RunnerStatusProps {
  status: RunnerStatusType
  onStart: (count?: number) => void
  onPause: () => void
  onStop: () => void
  onClearRateLimit?: () => void
  toggling: boolean
}

const stateConfig = {
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

function formatLocal(iso: string): string {
  try {
    return new Date(iso).toLocaleString(undefined, {
      hour: '2-digit',
      minute: '2-digit',
      day: '2-digit',
      month: 'short',
    })
  } catch {
    return iso
  }
}

function describeSource(source: string | null): string {
  switch (source) {
    case 'rate_limit':
      return 'Claude rate limit'
    default:
      return source ?? 'paused'
  }
}

export function RunnerStatus({ status, onStart, onPause, onStop, onClearRateLimit, toggling }: RunnerStatusProps) {
  const [taskCount, setTaskCount] = useState('')
  const trackedCount = status.active_tasks.filter((t) => !t.orphaned).length
  const orphanedCount = status.active_tasks.length - trackedCount
  const cfg = stateConfig[status.state]
  const hasLimit = status.task_limit > 0
  const pausedActive = status.paused_until !== null
    && new Date(status.paused_until).getTime() > Date.now()

  function handleStart() {
    const n = parseInt(taskCount, 10)
    onStart(n > 0 ? n : undefined)
    setTaskCount('')
  }

  return (
    <div className="space-y-3">
      {pausedActive && status.paused_until && (
        <div className="flex items-start justify-between gap-3 rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 text-amber-900">
          <div className="flex items-start gap-2">
            <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0" />
            <div className="text-sm">
              <p className="font-medium">
                Runner paused until {formatLocal(status.paused_until)} — {describeSource(status.pause_source)}
              </p>
              {status.pause_reason && (
                <p className="mt-0.5 text-xs text-amber-800/80">{status.pause_reason}</p>
              )}
            </div>
          </div>
          {onClearRateLimit && (
            <button
              type="button"
              onClick={onClearRateLimit}
              disabled={toggling}
              className="inline-flex items-center gap-1 rounded-md border border-amber-300 bg-white px-2.5 py-1 text-xs font-medium text-amber-900 transition-colors hover:bg-amber-100 disabled:opacity-50"
            >
              <X className="h-3 w-3" />
              Clear pause
            </button>
          )}
        </div>
      )}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <span className={clsx('inline-block h-3 w-3 rounded-full', cfg.dot)} />
            <span className="text-lg font-semibold text-zinc-900">{cfg.label}</span>
            {hasLimit && status.state === 'running' && (
              <span className="rounded-full bg-blue-100 px-2.5 py-0.5 text-xs font-medium text-blue-700">
                {status.completed_count}/{status.task_limit} tasks
              </span>
            )}
            <span className="text-sm text-zinc-500">
              {trackedCount}/{status.max_workers} active
            </span>
            {orphanedCount > 0 && (
              <span
                className="inline-flex items-center gap-1 rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-medium text-amber-700"
                title="Tasks marked running in the database that are not tracked by the current runner process."
              >
                <AlertTriangle className="h-3 w-3" />
                {orphanedCount} orphaned
              </span>
            )}
            {status.draining && (
              <span className="rounded-full bg-amber-100 px-2.5 py-0.5 text-xs font-medium text-amber-700">
                draining
              </span>
            )}
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
                  className="w-16 rounded-md border border-zinc-300 px-2 py-1.5 text-sm tabular-nums text-zinc-900 placeholder:text-zinc-400 focus:border-emerald-500 focus:outline-none focus:ring-1 focus:ring-emerald-500"
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
                className="inline-flex items-center gap-1.5 rounded-md bg-zinc-100 px-3 py-1.5 text-sm font-medium text-zinc-700 transition-colors hover:bg-zinc-200 disabled:opacity-50"
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
                className="inline-flex items-center gap-1.5 rounded-md bg-red-50 px-3 py-1.5 text-sm font-medium text-red-600 transition-colors hover:bg-red-100 disabled:opacity-50"
              >
                <Square className="h-3.5 w-3.5" />
                Stop
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
