import { useEffect, useState } from 'react'
import { clsx } from 'clsx'
import { Activity, CalendarClock, Flag, Timer, RotateCcw, DollarSign } from 'lucide-react'
import { formatDateTime } from '../utils/dateFormat'
import { runPhaseLabel } from '../utils/runPhase'
import type { Task, TaskStatus } from '../types'

const statusTone: Record<TaskStatus, { label: string; className: string }> = {
  done: { label: 'Done', className: 'text-emerald-700' },
  failed: { label: 'Failed', className: 'text-red-700' },
  needs_review: { label: 'Needs Review', className: 'text-amber-700' },
  running: { label: 'Running', className: 'text-blue-700' },
  queued: { label: 'Queued', className: 'text-zinc-700' },
  pending: { label: 'Pending', className: 'text-zinc-500' },
  cancelled: { label: 'Cancelled', className: 'text-zinc-500' },
  deleted: { label: 'Deleted', className: 'text-zinc-500' },
}

// formatStopwatch renders an elapsed time as H:MM:SS (or M:SS under an hour),
// the stopwatch-style format that reads well while it ticks.
export function formatStopwatch(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  const ss = s.toString().padStart(2, '0')
  if (h > 0) return `${h}:${m.toString().padStart(2, '0')}:${ss}`
  return `${m}:${ss}`
}

// taskTimes picks the start/finish timestamps that belong to the current run.
// completed_at is not cleared when a task is requeued, so while running (or when
// it predates started_at) it belongs to a superseded run and is dropped. A
// queued/pending task has not started this run at all, so both are hidden.
export function taskTimes(task: Task): { started: Date | null; finished: Date | null } {
  if (task.status === 'queued' || task.status === 'pending') return { started: null, finished: null }
  const started = task.started_at ? new Date(task.started_at) : null
  let finished = task.completed_at ? new Date(task.completed_at) : null
  if (task.status === 'running' || (started && finished && finished < started)) finished = null
  return { started, finished }
}

function useNow(active: boolean): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    if (!active) return
    setNow(Date.now())
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [active])
  return now
}

export default function TaskStatsCards({ task }: { task: Task }) {
  const running = task.status === 'running'
  const now = useNow(running)
  const { started, finished } = taskTimes(task)
  const phase = runPhaseLabel(task.status, task.run_phase)
  const tone = statusTone[task.status]

  let duration = '—'
  let durationHint: string | undefined
  if (started && running) {
    duration = formatStopwatch(now - started.getTime())
    durationHint = 'běží'
  } else if (started && finished) {
    duration = formatStopwatch(finished.getTime() - started.getTime())
  }

  const attempts = task.executions?.length ?? 0

  const cards: { icon: typeof Activity; label: string; value: string; hint?: string; valueClass?: string; live?: boolean }[] = [
    { icon: Activity, label: 'Status', value: tone.label, hint: phase ?? undefined, valueClass: tone.className },
    { icon: CalendarClock, label: 'Started', value: started ? formatDateTime(started) : '—' },
    {
      icon: Flag,
      label: 'Finished',
      value: finished ? formatDateTime(finished) : '—',
      hint: running ? 'ještě běží' : undefined,
    },
    { icon: Timer, label: 'Duration', value: duration, hint: durationHint, live: running && !!started },
    {
      icon: RotateCcw,
      label: 'Attempts',
      value: attempts > 0 ? String(attempts) : '—',
      hint: task.retry_count > 0 ? `retry ${task.retry_count}` : undefined,
    },
    { icon: DollarSign, label: 'Cost', value: task.cost_usd != null ? `$${task.cost_usd.toFixed(2)}` : '—' },
  ]

  return (
    <dl className="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-6" data-testid="task-stats">
      {cards.map(({ icon: Icon, label, value, hint, valueClass, live }) => (
        <div
          key={label}
          className={clsx(
            'min-w-0 rounded-lg border bg-white px-3 py-2.5',
            live ? 'border-blue-200 bg-blue-50/50' : 'border-zinc-200',
          )}
        >
          <dt className="flex items-center gap-1.5 text-xs uppercase tracking-wide text-zinc-500">
            <Icon className={clsx('h-3.5 w-3.5', live && 'animate-pulse text-blue-600')} />
            {label}
          </dt>
          <dd
            className={clsx('mt-1 truncate text-base font-semibold tabular-nums', valueClass ?? 'text-zinc-900')}
            aria-label={label}
          >
            {value}
          </dd>
          {hint && <dd className="truncate text-xs text-zinc-500">{hint}</dd>}
        </div>
      ))}
    </dl>
  )
}
