import { useEffect, useMemo, useState } from 'react'
import { Loader2, BarChart3, FolderGit2 } from 'lucide-react'
import { clsx } from 'clsx'

import { useDocumentTitle } from '../hooks/useDocumentTitle'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { fetchTaskStatsAggregated } from '../api/client'
import type { TaskStatsAggregated } from '../types'

// Stable hue ramp so the same project always paints the same color across
// re-renders. Tailwind doesn't support purely dynamic class names, so each
// entry must be a literal class string.
const STACK_COLORS = [
  'bg-blue-500',
  'bg-emerald-500',
  'bg-amber-500',
  'bg-rose-500',
  'bg-violet-500',
  'bg-cyan-500',
  'bg-orange-500',
  'bg-lime-500',
  'bg-fuchsia-500',
  'bg-teal-500',
] as const

const FALLBACK_COLOR = 'bg-zinc-400'

function colorForKey(key: string, allKeys: string[]): string {
  const idx = allKeys.indexOf(key)
  if (idx === -1) return FALLBACK_COLOR
  return STACK_COLORS[idx % STACK_COLORS.length] ?? FALLBACK_COLOR
}

function formatCost(usd: number): string {
  if (usd === 0) return '$0.00'
  if (usd < 0.01) return '<$0.01'
  if (usd < 10) return `$${usd.toFixed(2)}`
  return `$${usd.toFixed(0)}`
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(0)}K`
  return String(n)
}

// formatDayLabel turns a YYYY-MM-DD into a compact axis label like "5/10".
function formatDayLabel(day: string): string {
  const parts = day.split('-')
  if (parts.length !== 3) return day
  const month = parseInt(parts[1] ?? '', 10)
  const date = parseInt(parts[2] ?? '', 10)
  if (Number.isNaN(month) || Number.isNaN(date)) return day
  return `${month}/${date}`
}

interface DayStack {
  day: string
  total: number
  segments: { project: string; cost: number }[]
}

// buildDayStacks reshapes the (day, project) buckets into one entry per day
// with the per-project segments needed to render a stacked bar. Days within
// the requested window with zero spend become empty stacks so the x-axis
// doesn't compress visually around outlier days.
function buildDayStacks(data: TaskStatsAggregated): DayStack[] {
  const fromDate = new Date(`${data.from}T00:00:00Z`)
  const toDate = new Date(`${data.to}T00:00:00Z`)
  if (Number.isNaN(fromDate.getTime()) || Number.isNaN(toDate.getTime())) {
    return []
  }

  const byDay = new Map<string, DayStack>()
  for (const b of data.buckets) {
    if (!b.day) continue
    const key = b.day
    let entry = byDay.get(key)
    if (!entry) {
      entry = { day: key, total: 0, segments: [] }
      byDay.set(key, entry)
    }
    const project = b.project ?? 'Unknown'
    entry.segments.push({ project, cost: b.cost_usd })
    entry.total += b.cost_usd
  }

  const out: DayStack[] = []
  for (
    let cursor = new Date(fromDate.getTime());
    cursor.getTime() <= toDate.getTime();
    cursor.setUTCDate(cursor.getUTCDate() + 1)
  ) {
    const key = cursor.toISOString().slice(0, 10)
    out.push(byDay.get(key) ?? { day: key, total: 0, segments: [] })
  }
  return out
}

// projectTotals returns the per-project totals for the legend, sorted by
// descending spend so the top-cost project always appears first.
function projectTotals(data: TaskStatsAggregated): { project: string; cost: number }[] {
  const totals = new Map<string, number>()
  for (const b of data.buckets) {
    const key = b.project ?? 'Unknown'
    totals.set(key, (totals.get(key) ?? 0) + b.cost_usd)
  }
  return Array.from(totals.entries())
    .map(([project, cost]) => ({ project, cost }))
    .sort((a, b) => b.cost - a.cost)
}

function StackedBarChart({
  stacks,
  projects,
}: {
  stacks: DayStack[]
  projects: string[]
}) {
  const maxTotal = Math.max(...stacks.map((s) => s.total), 0.000001)

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5">
      <div className="mb-4 flex items-center gap-2">
        <BarChart3 className="h-4 w-4 text-zinc-400" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Daily Cost (last {stacks.length} days)
        </h2>
      </div>
      {stacks.every((s) => s.total === 0) ? (
        <p className="py-12 text-center text-sm text-zinc-400">No completed tasks in this window.</p>
      ) : (
        <>
          <div className="flex items-end gap-[2px]" style={{ height: 220 }}>
            {stacks.map((stack) => {
              const heightPct = stack.total > 0 ? (stack.total / maxTotal) * 100 : 0
              return (
                <div
                  key={stack.day}
                  className="group relative flex-1"
                  style={{ height: '100%' }}
                  title={`${stack.day} — ${formatCost(stack.total)}`}
                >
                  <div
                    className="absolute bottom-0 left-0 right-0 flex flex-col-reverse justify-end overflow-hidden rounded-t"
                    style={{ height: `${heightPct}%` }}
                  >
                    {stack.segments.map((seg) => {
                      const segPct = stack.total > 0 ? (seg.cost / stack.total) * 100 : 0
                      return (
                        <div
                          key={seg.project}
                          className={clsx(
                            'w-full transition-opacity group-hover:opacity-90',
                            colorForKey(seg.project, projects),
                          )}
                          style={{ height: `${segPct}%`, minHeight: seg.cost > 0 ? 1 : 0 }}
                        />
                      )
                    })}
                  </div>
                  {stack.total > 0 && (
                    <div className="pointer-events-none absolute bottom-full left-1/2 z-10 mb-1 hidden -translate-x-1/2 whitespace-nowrap rounded bg-zinc-900 px-2 py-1 text-[10px] text-white shadow-lg group-hover:block">
                      <div className="font-medium">{stack.day}</div>
                      <div>{formatCost(stack.total)}</div>
                      {stack.segments.slice(0, 5).map((seg) => (
                        <div key={seg.project} className="flex items-center gap-1.5">
                          <div
                            className={clsx(
                              'h-1.5 w-1.5 rounded-sm',
                              colorForKey(seg.project, projects),
                            )}
                          />
                          <span>
                            {seg.project}: {formatCost(seg.cost)}
                          </span>
                        </div>
                      ))}
                    </div>
                  )}
                </div>
              )
            })}
          </div>
          <div className="mt-2 flex justify-between text-[10px] text-zinc-400">
            <span>{stacks[0] ? formatDayLabel(stacks[0].day) : ''}</span>
            <span>{stacks[stacks.length - 1] ? formatDayLabel(stacks[stacks.length - 1]!.day) : ''}</span>
          </div>
        </>
      )}
    </div>
  )
}

function ProjectBreakdown({
  totals,
  projects,
}: {
  totals: { project: string; cost: number }[]
  projects: string[]
}) {
  const grandTotal = totals.reduce((acc, t) => acc + t.cost, 0)

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5">
      <div className="mb-4 flex items-center gap-2">
        <FolderGit2 className="h-4 w-4 text-zinc-400" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          By Project
        </h2>
      </div>
      {totals.length === 0 ? (
        <p className="text-sm text-zinc-400">No data</p>
      ) : (
        <div className="space-y-3">
          {totals.map((t) => {
            const pct = grandTotal > 0 ? (t.cost / grandTotal) * 100 : 0
            return (
              <div key={t.project}>
                <div className="mb-1 flex items-baseline justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <div
                      className={clsx(
                        'h-2.5 w-2.5 flex-shrink-0 rounded-sm',
                        colorForKey(t.project, projects),
                      )}
                    />
                    <span className="truncate text-sm font-medium text-zinc-700">
                      {t.project}
                    </span>
                  </div>
                  <span className="shrink-0 text-sm tabular-nums text-zinc-700">
                    {formatCost(t.cost)}
                    <span className="ml-1.5 text-xs text-zinc-400">
                      {pct.toFixed(0)}%
                    </span>
                  </span>
                </div>
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-100">
                  <div
                    className={clsx('h-full rounded-full', colorForKey(t.project, projects))}
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export default function StatsPage() {
  useDocumentTitle('Stats')
  const [data, setData] = useState<TaskStatsAggregated | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useMemo(
    () => async () => {
      try {
        const result = await fetchTaskStatsAggregated({ group_by: 'day,project' })
        setData(result)
        setError(null)
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Failed to load stats')
      } finally {
        setLoading(false)
      }
    },
    [],
  )

  useEffect(() => {
    load()
  }, [load])

  useRefreshOnFocus(load)

  const stacks = useMemo(() => (data ? buildDayStacks(data) : []), [data])
  const totals = useMemo(() => (data ? projectTotals(data) : []), [data])
  const projectKeys = useMemo(() => totals.map((t) => t.project), [totals])

  const totalTasks = useMemo(
    () => (data ? data.buckets.reduce((acc, b) => acc + b.task_count, 0) : 0),
    [data],
  )
  const totalCost = useMemo(
    () => totals.reduce((acc, t) => acc + t.cost, 0),
    [totals],
  )
  const totalTokens = useMemo(
    () =>
      data
        ? data.buckets.reduce(
            (acc, b) =>
              acc +
              b.input_tokens +
              b.output_tokens +
              b.cache_read_tokens +
              b.cache_creation_tokens,
            0,
          )
        : 0,
    [data],
  )

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  if (error || !data) {
    return (
      <div className="mx-auto max-w-5xl">
        <h1 className="mb-6 text-2xl font-bold text-zinc-900">Stats</h1>
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">
          {error ?? 'Failed to load stats'}
        </div>
      </div>
    )
  }

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-zinc-900">Stats</h1>
        <p className="mt-1 text-sm text-zinc-500">
          Task spending from {data.from} to {data.to}.
        </p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-xl border border-zinc-200 bg-white p-5">
          <p className="text-sm font-medium text-zinc-500">Total Cost</p>
          <p className="mt-1 text-3xl font-bold tabular-nums text-zinc-900">
            {formatCost(totalCost)}
          </p>
        </div>
        <div className="rounded-xl border border-zinc-200 bg-white p-5">
          <p className="text-sm font-medium text-zinc-500">Tasks Completed</p>
          <p className="mt-1 text-3xl font-bold tabular-nums text-zinc-900">
            {totalTasks.toLocaleString('en-US')}
          </p>
        </div>
        <div className="rounded-xl border border-zinc-200 bg-white p-5">
          <p className="text-sm font-medium text-zinc-500">Total Tokens</p>
          <p className="mt-1 text-3xl font-bold tabular-nums text-zinc-900">
            {formatTokens(totalTokens)}
          </p>
        </div>
      </div>

      <StackedBarChart stacks={stacks} projects={projectKeys} />
      <ProjectBreakdown totals={totals} projects={projectKeys} />
    </div>
  )
}
