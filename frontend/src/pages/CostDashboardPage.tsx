import { useEffect, useState, useCallback } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import { Loader2, MessageSquare, FolderGit2, Cpu, Zap } from 'lucide-react'

import { useDocumentTitle } from '../hooks/useDocumentTitle'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { fetchCostAnalytics } from '../api/client'
import type { CostAnalytics, CostByDate, CostByModel } from '../types'

const PERIOD_OPTIONS = [
  { label: '7d', days: 7 },
  { label: '30d', days: 30 },
  { label: '90d', days: 90 },
] as const

const MODEL_COLORS: Record<string, { bg: string; bar: string; text: string }> = {
  opus: {
    bg: 'bg-violet-500 dark:bg-violet-600',
    bar: 'bg-violet-500/20 dark:bg-violet-500/30',
    text: 'text-violet-600 dark:text-violet-400',
  },
  sonnet: {
    bg: 'bg-blue-500 dark:bg-blue-600',
    bar: 'bg-blue-500/20 dark:bg-blue-500/30',
    text: 'text-blue-600 dark:text-blue-400',
  },
  haiku: {
    bg: 'bg-emerald-500 dark:bg-emerald-600',
    bar: 'bg-emerald-500/20 dark:bg-emerald-500/30',
    text: 'text-emerald-600 dark:text-emerald-400',
  },
  unknown: {
    bg: 'bg-zinc-400 dark:bg-zinc-500',
    bar: 'bg-zinc-400/20 dark:bg-zinc-500/30',
    text: 'text-zinc-500 dark:text-zinc-400',
  },
}

const DEFAULT_MODEL_COLOR = MODEL_COLORS['unknown']!

function getModelColor(model: string) {
  return MODEL_COLORS[model] ?? DEFAULT_MODEL_COLOR
}

function formatTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`
  if (n >= 1_000) return `${(n / 1_000).toFixed(n >= 10_000 ? 0 : 1)}K`
  return String(n)
}

function formatCost(usd: number): string {
  return `$${usd.toFixed(2)}`
}

function TokenChart({ data, models }: { data: CostByDate[]; models: string[] }) {
  // Find max total tokens per day for scaling
  const maxTokens = Math.max(
    ...data.map((d) => {
      let total = 0
      for (const m of models) {
        const mt = d.by_model[m]
        if (mt) total += mt.input + mt.output
      }
      return total
    }),
    1,
  )

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
      <div className="mb-4 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          Daily Token Usage
        </h2>
        <div className="flex items-center gap-3">
          {models.map((m) => (
            <div key={m} className="flex items-center gap-1.5">
              <div className={clsx('h-2.5 w-2.5 rounded-sm', getModelColor(m).bg)} />
              <span className="text-xs text-zinc-500 dark:text-zinc-400">{m}</span>
            </div>
          ))}
        </div>
      </div>
      <div className="flex items-end gap-[2px]" style={{ height: 160 }}>
        {data.map((d) => {
          // Calculate stacked segments for this day
          const segments: { model: string; tokens: number }[] = []
          let dayTotal = 0
          for (const m of models) {
            const mt = d.by_model[m]
            if (mt) {
              const t = mt.input + mt.output
              segments.push({ model: m, tokens: t })
              dayTotal += t
            }
          }

          const totalPct = maxTokens > 0 ? (dayTotal / maxTokens) * 100 : 0
          const barHeight = Math.max(totalPct, dayTotal > 0 ? 2 : 0)

          return (
            <div
              key={d.date}
              className="group relative flex-1"
              style={{ height: '100%' }}
            >
              <div
                className="absolute bottom-0 left-0 right-0 flex flex-col justify-end"
                style={{ height: `${barHeight}%`, minWidth: 2 }}
              >
                {segments.map((seg) => {
                  const segPct = dayTotal > 0 ? (seg.tokens / dayTotal) * 100 : 0
                  return (
                    <div
                      key={seg.model}
                      className={clsx(
                        'w-full transition-opacity group-hover:opacity-80',
                        getModelColor(seg.model).bg,
                        segments[0]?.model === seg.model && 'rounded-t',
                      )}
                      style={{ height: `${segPct}%`, minHeight: seg.tokens > 0 ? 1 : 0 }}
                    />
                  )
                })}
              </div>
              <div className="pointer-events-none absolute -top-2 left-1/2 z-10 hidden -translate-x-1/2 -translate-y-full whitespace-nowrap rounded bg-zinc-800 px-2.5 py-1.5 text-xs text-white shadow-lg group-hover:block dark:bg-zinc-200 dark:text-zinc-900">
                <div className="mb-1 font-medium">{d.date.slice(5)}</div>
                {segments.map((seg) => (
                  <div key={seg.model} className="flex items-center gap-1.5">
                    <div className={clsx('h-1.5 w-1.5 rounded-sm', getModelColor(seg.model).bg)} />
                    <span>
                      {seg.model}: {formatTokens(d.by_model[seg.model]?.input ?? 0)} in, {formatTokens(d.by_model[seg.model]?.output ?? 0)} out
                    </span>
                  </div>
                ))}
                {dayTotal === 0 && <div className="text-zinc-400">No tokens</div>}
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

function ModelBreakdown({ models }: { models: CostByModel[] }) {
  const totalTokens = models.reduce((s, m) => s + m.input_tokens + m.output_tokens, 0)

  return (
    <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
      <div className="mb-4 flex items-center gap-2">
        <Cpu className="h-4 w-4 text-zinc-400" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
          By Model
        </h2>
      </div>
      {models.length === 0 ? (
        <p className="text-sm text-zinc-400 dark:text-zinc-500">No data</p>
      ) : (
        <div className="space-y-4">
          {models.map((m) => {
            const modelTotal = m.input_tokens + m.output_tokens
            const pct = totalTokens > 0 ? (modelTotal / totalTokens) * 100 : 0
            const color = getModelColor(m.model)
            return (
              <div key={m.model}>
                <div className="mb-1.5 flex items-baseline justify-between">
                  <div className="flex items-center gap-2">
                    <div className={clsx('h-2.5 w-2.5 rounded-sm', color.bg)} />
                    <span className="text-sm font-semibold text-zinc-700 dark:text-zinc-300">
                      {m.model}
                    </span>
                    <span className="text-xs text-zinc-400 dark:text-zinc-500">
                      {m.message_count} msg{m.message_count !== 1 ? 's' : ''}
                    </span>
                  </div>
                  <span className="text-sm font-medium tabular-nums text-zinc-600 dark:text-zinc-400">
                    {pct.toFixed(1)}%
                  </span>
                </div>
                <div className="mb-1.5 h-2 w-full overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-700">
                  <div
                    className={clsx('h-full rounded-full', color.bg)}
                    style={{ width: `${pct}%` }}
                  />
                </div>
                <div className="flex items-center gap-3 text-xs text-zinc-500 dark:text-zinc-400">
                  <span title={`${m.input_tokens.toLocaleString()} input tokens`}>
                    {formatTokens(m.input_tokens)} in
                  </span>
                  <span title={`${m.output_tokens.toLocaleString()} output tokens`}>
                    {formatTokens(m.output_tokens)} out
                  </span>
                  <span className="text-zinc-400 dark:text-zinc-500">
                    {formatCost(m.cost_usd)}
                  </span>
                </div>
              </div>
            )
          })}
        </div>
      )}
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
  items: { label: string; tokens: number; cost: number; link?: string }[]
}) {
  const maxTokens = Math.max(...items.map((i) => i.tokens), 1)

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
            const pct = (item.tokens / maxTokens) * 100
            const content = (
              <div key={i}>
                <div className="mb-1 flex items-baseline justify-between">
                  <span className="truncate text-sm font-medium text-zinc-700 dark:text-zinc-300">
                    {item.label}
                  </span>
                  <span className="ml-2 shrink-0 text-sm tabular-nums text-zinc-900 dark:text-zinc-100">
                    {formatTokens(item.tokens)}
                    <span className="ml-1.5 text-xs text-zinc-400 dark:text-zinc-500">
                      {formatCost(item.cost)}
                    </span>
                  </span>
                </div>
                <div className="h-1.5 w-full overflow-hidden rounded-full bg-zinc-100 dark:bg-zinc-700">
                  <div
                    className="h-full rounded-full bg-blue-500 dark:bg-blue-600"
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
  useDocumentTitle('Usage')
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
      setError(e instanceof Error ? e.message : 'Failed to load usage data')
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

  const totalTokens = data.total_input_tokens + data.total_output_tokens

  // Collect all models that appear in the data (for chart legend and ordering)
  const modelOrder = data.by_model.map((m) => m.model)

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100">Usage</h1>
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
        {/* Total Tokens card */}
        <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Total Tokens</p>
              <p
                className="mt-1 text-3xl font-bold tabular-nums text-blue-600 dark:text-blue-400"
                title={totalTokens.toLocaleString()}
              >
                {formatTokens(totalTokens)}
              </p>
              <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
                {formatTokens(data.total_input_tokens)} in / {formatTokens(data.total_output_tokens)} out
              </p>
            </div>
            <div className="rounded-lg bg-blue-50 p-2.5 dark:bg-blue-950">
              <Zap className="h-5 w-5 text-blue-600 dark:text-blue-400" />
            </div>
          </div>
        </div>

        {/* By Model summary card */}
        <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
          <div className="flex items-start justify-between">
            <div className="min-w-0 flex-1">
              <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">By Model</p>
              <div className="mt-2 space-y-1.5">
                {data.by_model.length === 0 && (
                  <p className="text-sm text-zinc-400">No data</p>
                )}
                {data.by_model.map((m) => (
                  <div key={m.model} className="flex items-center gap-2">
                    <div className={clsx('h-2 w-2 rounded-sm', getModelColor(m.model).bg)} />
                    <span className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                      {m.model}
                    </span>
                    <span className="text-xs tabular-nums text-zinc-500 dark:text-zinc-400">
                      {formatTokens(m.input_tokens)} in / {formatTokens(m.output_tokens)} out
                    </span>
                  </div>
                ))}
              </div>
            </div>
            <div className="rounded-lg bg-zinc-100 p-2.5 dark:bg-zinc-700">
              <Cpu className="h-5 w-5 text-zinc-600 dark:text-zinc-400" />
            </div>
          </div>
        </div>

        {/* Cost card (secondary) */}
        <div className="rounded-xl border border-zinc-200 bg-white p-5 dark:border-zinc-700 dark:bg-zinc-800">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-sm font-medium text-zinc-500 dark:text-zinc-400">Total Cost</p>
              <p className="mt-1 text-3xl font-bold tabular-nums text-zinc-600 dark:text-zinc-400">
                {formatCost(data.total_cost_usd)}
              </p>
              <p className="mt-1 text-xs text-zinc-400 dark:text-zinc-500">
                Last {days} days
              </p>
            </div>
            <div className="rounded-lg bg-zinc-100 p-2.5 dark:bg-zinc-700">
              <span className="text-sm font-medium text-zinc-500 dark:text-zinc-400">$</span>
            </div>
          </div>
        </div>
      </div>

      {/* Daily token chart */}
      <TokenChart data={data.by_date} models={modelOrder} />

      {/* Model breakdown + Top threads side by side */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-2">
        <ModelBreakdown models={data.by_model} />
        <TopList
          title="Top Threads"
          icon={<MessageSquare className="h-4 w-4 text-zinc-400" />}
          items={data.by_thread.map((t) => ({
            label: t.title,
            tokens: t.input_tokens + t.output_tokens,
            cost: t.cost_usd,
            link: `/chat/${t.thread_id}`,
          }))}
        />
      </div>

      {/* Top projects */}
      <TopList
        title="Top Projects"
        icon={<FolderGit2 className="h-4 w-4 text-zinc-400" />}
        items={data.by_project.map((p) => ({
          label: p.project_name,
          tokens: p.input_tokens + p.output_tokens,
          cost: p.cost_usd,
        }))}
      />
    </div>
  )
}
