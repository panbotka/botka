import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import { DollarSign, Loader2, MessageSquare, FolderGit2 } from 'lucide-react'

import { useDocumentTitle } from '../hooks/useDocumentTitle'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { fetchCostAnalytics } from '../api/client'
import type { CostAnalytics, CostByDate } from '../types'

const PERIOD_OPTIONS = [
  { label: '7d', days: 7 },
  { label: '30d', days: 30 },
  { label: '90d', days: 90 },
] as const

function formatCost(usd: number): string {
  return `$${usd.toFixed(2)}`
}

function CostChart({ data }: { data: CostByDate[] }) {
  const maxCost = Math.max(...data.map((d) => d.cost_usd), 0.01)

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
      <h2 className="mb-4 text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
        Daily Spend
      </h2>
      <div className="flex items-end gap-[2px]" style={{ height: 160 }}>
        {data.map((d) => {
          const pct = maxCost > 0 ? (d.cost_usd / maxCost) * 100 : 0
          const barHeight = Math.max(pct, d.cost_usd > 0 ? 2 : 0)
          return (
            <div
              key={d.date}
              className="group relative flex-1"
              style={{ height: '100%' }}
            >
              <div className="absolute bottom-0 left-0 right-0 flex justify-center">
                <div
                  className="w-full rounded-t bg-emerald-500 transition-colors group-hover:bg-emerald-400 dark:bg-emerald-600 dark:group-hover:bg-emerald-500"
                  style={{ height: `${barHeight}%`, minWidth: 2 }}
                />
              </div>
              <div className="pointer-events-none absolute -top-10 left-1/2 z-10 hidden -translate-x-1/2 whitespace-nowrap rounded bg-zinc-800 px-2 py-1 text-xs text-white shadow group-hover:block dark:bg-zinc-200 dark:text-zinc-900">
                {d.date.slice(5)}: {formatCost(d.cost_usd)}
              </div>
            </div>
          )
        })}
      </div>
      <div className="mt-2 flex justify-between text-[10px] text-zinc-400 dark:text-zinc-500">
        <span>{data[0]?.date.slice(5)}</span>
        <span>{data[data.length - 1]?.date.slice(5)}</span>
      </div>
    </div>
  )
}

function TopList({
  title,
  icon,
  items,
}: {
  title: string
  icon: React.ReactNode
  items: { label: string; cost: number; link?: string }[]
}) {
  const maxCost = Math.max(...items.map((i) => i.cost), 0.01)

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
      <div className="mb-4 flex items-center gap-2">
        {icon}
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          {title}
        </h2>
      </div>
      {items.length === 0 ? (
        <p className="text-sm text-zinc-400 dark:text-zinc-500">No data</p>
      ) : (
        <div className="space-y-3">
          {items.map((item, i) => {
            const pct = (item.cost / maxCost) * 100
            const content = (
              <div key={i}>
                <div className="mb-1 flex items-baseline justify-between">
                  <span className="truncate text-sm font-medium text-zinc-700 dark:text-zinc-300">
                    {item.label}
                  </span>
                  <span className="ml-2 shrink-0 text-sm tabular-nums text-zinc-900 dark:text-zinc-100">
                    {formatCost(item.cost)}
                  </span>
                </div>
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-700">
                  <div
                    className="h-full rounded-full bg-emerald-500 dark:bg-emerald-600"
                    style={{ width: `${pct}%` }}
                  />
                </div>
              </div>
            )

            if (item.link) {
              return (
                <Link key={i} to={item.link} className="block rounded-md px-1 -mx-1 hover:bg-zinc-50 dark:hover:bg-zinc-700/50">
                  {content}
                </Link>
              )
            }
            return <div key={i}>{content}</div>
          })}
        </div>
      )}
    </div>
  )
}

export default function CostDashboardPage() {
  useDocumentTitle('Cost')
  const [data, setData] = useState<CostAnalytics | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [days, setDays] = useState(30)

  const load = useCallback(async () => {
    try {
      const result = await fetchCostAnalytics(days)
      setData(result)
      setError(null)
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to load cost data')
    } finally {
      setLoading(false)
    }
  }, [days])

  useEffect(() => {
    setLoading(true)
    load()
  }, [load])

  useRefreshOnFocus(load)

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  if (error || !data) {
    return (
      <div className="flex h-64 items-center justify-center text-center">
        <div>
          <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">
            {error || 'Failed to load'}
          </p>
        </div>
      </div>
    )
  }

  // Compute period averages
  const daysWithCost = data.by_date.filter((d) => d.cost_usd > 0).length
  const avgDaily = daysWithCost > 0 ? data.total_cost_usd / daysWithCost : 0

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100">Cost</h1>
        <div className="flex rounded-lg border border-zinc-200 bg-white dark:border-zinc-700 dark:bg-zinc-800">
          {PERIOD_OPTIONS.map((opt) => (
            <button
              key={opt.days}
              onClick={() => setDays(opt.days)}
              className={clsx(
                'px-3 py-1.5 text-sm font-medium transition-colors',
                days === opt.days
                  ? 'bg-zinc-900 text-white dark:bg-zinc-100 dark:text-zinc-900'
                  : 'text-zinc-600 hover:bg-zinc-50 dark:text-zinc-400 dark:hover:bg-zinc-700',
                opt.days === 7 && 'rounded-l-lg',
                opt.days === 90 && 'rounded-r-lg',
              )}
            >
              {opt.label}
            </button>
          ))}
        </div>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Total Spend</p>
              <p className="mt-1 text-3xl font-bold tabular-nums text-emerald-600 dark:text-emerald-400">
                {formatCost(data.total_cost_usd)}
              </p>
              <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
                Last {days} days
              </p>
            </div>
            <div className="rounded-lg bg-emerald-50 p-2.5 dark:bg-emerald-950">
              <DollarSign className="h-5 w-5 text-emerald-600 dark:text-emerald-400" />
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Daily Average</p>
              <p className="mt-1 text-3xl font-bold tabular-nums text-zinc-900 dark:text-zinc-100">
                {formatCost(avgDaily)}
              </p>
              <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
                {daysWithCost} active day{daysWithCost !== 1 ? 's' : ''}
              </p>
            </div>
            <div className="rounded-lg bg-zinc-100 p-2.5 dark:bg-zinc-700">
              <DollarSign className="h-5 w-5 text-zinc-600 dark:text-zinc-400" />
            </div>
          </div>
        </div>
        <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Today</p>
              <p className="mt-1 text-3xl font-bold tabular-nums text-zinc-900 dark:text-zinc-100">
                {formatCost(data.by_date[data.by_date.length - 1]?.cost_usd ?? 0)}
              </p>
            </div>
            <div className="rounded-lg bg-zinc-100 p-2.5 dark:bg-zinc-700">
              <DollarSign className="h-5 w-5 text-zinc-600 dark:text-zinc-400" />
            </div>
          </div>
        </div>
      </div>

      {/* Daily spend chart */}
      <CostChart data={data.by_date} />

      {/* Top threads and projects side by side */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <TopList
          title="Top Threads"
          icon={<MessageSquare className="h-4 w-4 text-zinc-400" />}
          items={data.by_thread.map((t) => ({
            label: t.title,
            cost: t.cost_usd,
            link: `/chat/${t.thread_id}`,
          }))}
        />
        <TopList
          title="Top Projects"
          icon={<FolderGit2 className="h-4 w-4 text-zinc-400" />}
          items={data.by_project.map((p) => ({
            label: p.project_name,
            cost: p.cost_usd,
          }))}
        />
      </div>
    </div>
  )
}
