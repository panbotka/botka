import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import { Loader2 } from 'lucide-react'

import { formatStopwatch } from './TaskStatsCards'
import { runPhaseLabel } from '../utils/runPhase'
import type { RunnerStatus, Task } from '../types'

// How many queued tasks the "Následuje" section lists before "+N dalších".
export const UP_NEXT_LIMIT = 5

interface RunQueuePanelProps {
  running: Task[]
  queued: Task[]
  // Total number of queued tasks on the server, which may exceed queued.length.
  queuedTotal: number
  runner: RunnerStatus | null
  onShowQueued: () => void
}

// orderQueue sorts queued tasks the way the runner picks them:
// priority DESC, created_at ASC (buildPickQuery in internal/runner/runner.go).
export function orderQueue(tasks: Task[]): Task[] {
  return [...tasks].sort(
    (a, b) =>
      b.priority - a.priority ||
      new Date(a.created_at).getTime() - new Date(b.created_at).getTime(),
  )
}

// busyProjectIds returns the projects that already have a running task — the
// runner skips their queued tasks until that task finishes.
export function busyProjectIds(running: Task[]): Set<string> {
  return new Set(running.map((t) => t.project_id))
}

const runnerStateConfig = {
  running: { dot: 'bg-emerald-500', label: 'běží' },
  paused: { dot: 'bg-amber-400', label: 'pozastaven' },
  stopped: { dot: 'bg-red-500', label: 'zastaven' },
} as const

function projectName(task: Task): string {
  return task.project_name || task.project?.name || '—'
}

function formatTime(iso: string): string {
  return new Date(iso).toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' })
}

function RunnerHeader({ runner }: { runner: RunnerStatus | null }) {
  if (!runner) return null
  const cfg = runnerStateConfig[runner.state]
  const active = runner.active_tasks.filter((t) => !t.orphaned).length
  const pausedUntil =
    runner.paused_until && new Date(runner.paused_until).getTime() > Date.now()
      ? runner.paused_until
      : null
  return (
    <div className="flex min-w-0 items-center gap-2 text-xs text-zinc-500" data-testid="runner-state">
      <span className={clsx('inline-block h-2 w-2 flex-shrink-0 rounded-full', cfg.dot)} />
      <span className="font-medium text-zinc-700">Runner {cfg.label}</span>
      <span className="tabular-nums">
        {active}/{runner.max_workers} workerů
      </span>
      {pausedUntil && (
        <span className="truncate text-amber-700" title={runner.pause_reason ?? undefined}>
          pauza do {formatTime(pausedUntil)}
        </span>
      )}
    </div>
  )
}

const rowClass =
  'flex min-h-[44px] items-center gap-3 px-3 text-sm transition-colors hover:bg-zinc-50'

/**
 * Compact "Běží teď / Následuje" panel shown above the task status tabs: what
 * the runner is doing right now and which queued tasks it will pick next.
 */
export default function RunQueuePanel({
  running,
  queued,
  queuedTotal,
  runner,
  onShowQueued,
}: RunQueuePanelProps) {
  const hasRunning = running.length > 0
  const [now, setNow] = useState(() => Date.now())

  useEffect(() => {
    if (!hasRunning) return
    setNow(Date.now())
    const id = setInterval(() => setNow(Date.now()), 1000)
    return () => clearInterval(id)
  }, [hasRunning])

  if (!hasRunning && queuedTotal === 0 && queued.length === 0) {
    return (
      <div
        className="flex min-h-[44px] flex-wrap items-center justify-between gap-2 rounded-lg border border-zinc-200 bg-white px-3 py-2"
        data-testid="run-queue-panel"
      >
        <p className="text-sm text-zinc-400">Nic neběží, fronta je prázdná</p>
        <RunnerHeader runner={runner} />
      </div>
    )
  }

  const busy = busyProjectIds(running)
  const upNext = orderQueue(queued).slice(0, UP_NEXT_LIMIT)
  const more = Math.max(0, queuedTotal - upNext.length)

  return (
    <div
      className="overflow-hidden rounded-lg border border-zinc-200 bg-white"
      data-testid="run-queue-panel"
    >
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-zinc-100 px-3 py-2">
        <h2 className="text-xs font-semibold uppercase tracking-wide text-zinc-500">Běží teď</h2>
        <RunnerHeader runner={runner} />
      </div>
      {hasRunning ? (
        <ul className="divide-y divide-zinc-100" aria-label="Běží teď">
          {running.map((task) => {
            const phase = runPhaseLabel(task.status, task.run_phase)
            const started = task.started_at ? new Date(task.started_at).getTime() : null
            return (
              <li key={task.id}>
                <Link to={`/tasks/${task.id}`} className={rowClass}>
                  <Loader2 className="h-4 w-4 flex-shrink-0 animate-spin text-amber-600" />
                  <span className="min-w-0 flex-1 truncate font-medium text-zinc-900">{task.title}</span>
                  <span className="max-w-[6rem] flex-shrink-0 truncate text-xs text-zinc-500 sm:max-w-[10rem]">
                    {projectName(task)}
                  </span>
                  {phase && (
                    <span className="flex-shrink-0 rounded-full bg-amber-50 px-2 py-0.5 text-xs text-amber-700">
                      {phase}
                    </span>
                  )}
                  <span className="w-14 flex-shrink-0 text-right text-xs tabular-nums text-zinc-500">
                    {started !== null ? formatStopwatch(now - started) : '—'}
                  </span>
                </Link>
              </li>
            )
          })}
        </ul>
      ) : (
        <p className="flex min-h-[44px] items-center px-3 text-sm text-zinc-400">Nic neběží</p>
      )}
      {upNext.length > 0 && (
        <>
          <div className="border-y border-zinc-100 px-3 py-2">
            <h2 className="text-xs font-semibold uppercase tracking-wide text-zinc-500">Následuje</h2>
          </div>
          <ol className="divide-y divide-zinc-100" aria-label="Následuje">
            {upNext.map((task, i) => (
              <li key={task.id} data-testid="up-next-row">
                <Link to={`/tasks/${task.id}`} className={rowClass}>
                  <span className="w-5 flex-shrink-0 text-right text-xs tabular-nums text-zinc-400">{i + 1}.</span>
                  <span className="min-w-0 flex-1 truncate text-zinc-900">{task.title}</span>
                  {busy.has(task.project_id) && (
                    <span
                      className="flex-shrink-0 rounded-full bg-zinc-100 px-2 py-0.5 text-xs text-zinc-500"
                      title="Na tomto projektu už běží jiný task — runner ho přeskočí, dokud neskončí."
                    >
                      čeká na projekt
                    </span>
                  )}
                  <span className="max-w-[6rem] flex-shrink-0 truncate text-xs text-zinc-500 sm:max-w-[10rem]">
                    {projectName(task)}
                  </span>
                  <span
                    className="w-8 flex-shrink-0 text-right text-xs tabular-nums text-zinc-500"
                    title="Priorita"
                  >
                    P{task.priority}
                  </span>
                </Link>
              </li>
            ))}
          </ol>
          {more > 0 && (
            <button
              type="button"
              onClick={onShowQueued}
              className="flex min-h-[44px] w-full items-center border-t border-zinc-100 px-3 text-sm font-medium text-blue-600 hover:bg-zinc-50"
            >
              +{more} dalších
            </button>
          )}
        </>
      )}
    </div>
  )
}
