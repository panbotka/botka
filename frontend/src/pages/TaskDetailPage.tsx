import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
import { formatDateTime } from '../utils/dateFormat'
import ReactMarkdown from 'react-markdown'
import { clsx } from 'clsx'
import {
  ArrowLeft,
  Pencil,
  RotateCcw,
  Trash2,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Clock,
  Loader2,
  Undo2,
  StopCircle,
  ChevronDown,
  ChevronRight,
  Play,
  Ban,
  RefreshCw,
  Sparkles,
} from 'lucide-react'
import { TaskForm } from '../components/TaskForm'
import { LiveOutputInline } from '../components/LiveOutput'
import TaskOutputView from '../components/TaskOutputView'
import TaskChangesSection from '../components/TaskChangesSection'
import { TaskTagPicker } from '../components/TaskTagPicker'
import { fetchTask, retryTask, deleteTask, updateTask, killTask, fetchTaskRawOutput, regenerateTaskFailureSummary } from '../api/client'
import { parseNDJSON, type TaskOutputEvent } from '../utils/parseNDJSON'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useTaskEvents } from '../hooks/useTaskEvents'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import type { Task, TaskStatus, TaskTag, TaskExecution } from '../types'

const statusConfig: Record<
  TaskStatus,
  { icon: typeof CheckCircle2; color: string; bgColor: string; label: string }
> = {
  done: { icon: CheckCircle2, color: 'text-emerald-700', bgColor: 'bg-emerald-50', label: 'Done' },
  failed: { icon: XCircle, color: 'text-red-700', bgColor: 'bg-red-50', label: 'Failed' },
  needs_review: {
    icon: AlertTriangle,
    color: 'text-amber-700',
    bgColor: 'bg-amber-50',
    label: 'Needs Review',
  },
  running: { icon: Loader2, color: 'text-blue-700', bgColor: 'bg-blue-50', label: 'Running' },
  queued: { icon: Clock, color: 'text-zinc-700', bgColor: 'bg-zinc-100', label: 'Queued' },
  pending: { icon: Clock, color: 'text-zinc-500', bgColor: 'bg-zinc-50', label: 'Pending' },
  cancelled: { icon: XCircle, color: 'text-zinc-500', bgColor: 'bg-zinc-50', label: 'Cancelled' },
  deleted: { icon: Trash2, color: 'text-zinc-500', bgColor: 'bg-zinc-50', label: 'Deleted' },
}

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  return `${hours}h ${remainMinutes}m`
}

function formatDate(iso: string): string {
  return formatDateTime(iso)
}

function formatTokens(n: number | null | undefined): string {
  if (n == null) return '—'
  return n.toLocaleString('en-US')
}

function formatCost(usd: number | null | undefined): string {
  if (usd == null) return '—'
  return `$${usd.toFixed(4)}`
}

export default function TaskDetailPage() {
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  useDocumentTitle(id === 'new' ? 'New Task' : '')

  if (id === 'new') {
    return (
      <div className="mx-auto max-w-3xl">
        <Link
          to="/tasks"
          className="mb-4 inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-700"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to tasks
        </Link>
        <h1 className="mb-6 text-2xl font-bold text-zinc-900">New Task</h1>
        <TaskForm onSave={() => navigate('/tasks')} onCancel={() => navigate('/tasks')} />
      </div>
    )
  }

  return <TaskDetail taskId={id!} />
}

function TaskDetail({ taskId }: { taskId: string }) {
  const [task, setTask] = useState<Task | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [editing, setEditing] = useState(false)
  const [acting, setActing] = useState(false)

  const load = useCallback(async () => {
    try {
      const t = await fetchTask(taskId)
      setTask(t)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load task')
    } finally {
      setLoading(false)
    }
  }, [taskId])

  useEffect(() => {
    load()
  }, [load])

  useRefreshOnFocus(load)
  useTaskEvents(load)
  useDocumentTitle(task?.title || 'Task')

  // Polling fallback: refresh every 5s while task is running
  useEffect(() => {
    if (task?.status !== 'running') return
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [task?.status, load])

  async function handleRetry() {
    setActing(true)
    try {
      await retryTask(taskId)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Retry failed')
    } finally {
      setActing(false)
    }
  }

  async function handleDelete() {
    setActing(true)
    try {
      await deleteTask(taskId)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Delete failed')
    } finally {
      setActing(false)
    }
  }

  async function handleRestore() {
    setActing(true)
    try {
      await updateTask(taskId, { status: 'pending' as TaskStatus })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Restore failed')
    } finally {
      setActing(false)
    }
  }

  async function handleMarkDone() {
    setActing(true)
    try {
      await updateTask(taskId, { status: 'done' as TaskStatus })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Update failed')
    } finally {
      setActing(false)
    }
  }

  async function handleKill() {
    if (!confirm('This will terminate the running task and revert all changes. Continue?')) return
    setActing(true)
    try {
      await killTask(taskId)
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Kill failed')
    } finally {
      setActing(false)
    }
  }

  async function handleQueue() {
    setActing(true)
    try {
      await updateTask(taskId, { status: 'queued' as TaskStatus })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Queue failed')
    } finally {
      setActing(false)
    }
  }

  async function handleCancel() {
    setActing(true)
    try {
      await updateTask(taskId, { status: 'cancelled' as TaskStatus })
      await load()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Cancel failed')
    } finally {
      setActing(false)
    }
  }

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  if (error && !task) {
    return (
      <div className="mx-auto max-w-3xl">
        <Link
          to="/tasks"
          className="mb-4 inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-700"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to tasks
        </Link>
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      </div>
    )
  }

  if (!task) return null

  if (editing) {
    return (
      <div className="mx-auto max-w-3xl">
        <Link
          to="/tasks"
          className="mb-4 inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-700"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to tasks
        </Link>
        <h1 className="mb-6 text-2xl font-bold text-zinc-900">Edit Task</h1>
        <TaskForm
          taskId={taskId}
          onSave={() => {
            setEditing(false)
            load()
          }}
          onCancel={() => setEditing(false)}
        />
      </div>
    )
  }

  const cfg = statusConfig[task.status]
  const StatusIcon = cfg.icon
  const hasCompletedOutput = task.status === 'done' || task.status === 'failed' || task.status === 'needs_review'

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      <Link
        to="/tasks"
        className="inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-700"
      >
        <ArrowLeft className="h-4 w-4" />
        Back to tasks
      </Link>

      {error && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Header */}
      <div className="flex items-start justify-between gap-4">
        <div>
          <div className="flex items-center gap-3">
            <h1 className="text-2xl font-bold text-zinc-900">{task.title}</h1>
            <span
              className={clsx(
                'inline-flex items-center gap-1 rounded-full px-2.5 py-0.5 text-xs font-medium',
                cfg.bgColor,
                cfg.color,
              )}
            >
              <StatusIcon
                className={clsx('h-3.5 w-3.5', task.status === 'running' && 'animate-spin')}
              />
              {cfg.label}
            </span>
          </div>
          <div className="mt-2 flex flex-wrap gap-x-4 gap-y-1 text-sm text-zinc-500">
            <span>Project: {task.project?.name ?? 'Unknown'}</span>
            <span>Priority: {task.priority}</span>
            <span>Created: {formatDate(task.created_at)}</span>
            <span>Updated: {formatDate(task.updated_at)}</span>
            {task.schedule_id && (
              <span>
                Created by schedule:{' '}
                <Link
                  to="/schedules"
                  className="text-zinc-700 underline hover:text-zinc-900"
                >
                  {task.schedule?.title ?? `#${task.schedule_id}`}
                </Link>
              </span>
            )}
          </div>
        </div>
        <button
          onClick={() => setEditing(true)}
          className="inline-flex items-center gap-1.5 rounded-md border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
        >
          <Pencil className="h-3.5 w-3.5" />
          Edit
        </button>
      </div>

      {/* Tags */}
      <div className="flex items-center gap-2">
        <span className="text-xs uppercase tracking-wide text-zinc-400">Tags</span>
        <TaskTagPicker
          taskId={taskId}
          selected={task.tags ?? []}
          onChange={(tags: TaskTag[]) =>
            setTask((prev) => (prev ? { ...prev, tags } : prev))
          }
        />
      </div>

      {/* Failure summary */}
      {task.status === 'failed' && (
        <FailureSummaryBox
          taskId={taskId}
          summary={task.failure_summary ?? null}
          onSummary={(s) => setTask((prev) => (prev ? { ...prev, failure_summary: s } : prev))}
        />
      )}

      {/* Failure reason */}
      {task.failure_reason && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">
          <span className="font-medium">Failure:</span> {task.failure_reason}
        </div>
      )}

      {/* Spec */}
      <div className="overflow-hidden rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">Spec</h2>
        {task.spec ? (
          <div className="prose prose-sm prose-zinc max-w-none min-w-0 break-words overflow-x-auto">
            <ReactMarkdown>{task.spec}</ReactMarkdown>
          </div>
        ) : (
          <p className="text-sm text-zinc-400">No spec provided</p>
        )}
      </div>

      {/* Changes (git diff) — visible for completed runs that recorded a commit range */}
      {hasCompletedOutput && task.base_commit_sha && task.head_commit_sha && (
        <TaskChangesSection taskId={taskId} />
      )}

      {/* Live Output */}
      {task.status === 'running' && (
        <LiveOutputInline taskId={taskId} taskTitle={task.title} />
      )}

      {/* Historical Output */}
      {hasCompletedOutput && task.status !== 'running' && (
        <HistoricalOutput taskId={taskId} />
      )}

      {/* Kill button for running tasks */}
      {task.status === 'running' && (
        <div className="flex gap-3">
          <button
            onClick={handleKill}
            disabled={acting}
            className="inline-flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
          >
            <StopCircle className="h-3.5 w-3.5" />
            Kill Task
          </button>
        </div>
      )}

      {/* Actions */}
      {task.status !== 'running' && (
        <div className="flex gap-3">
          {task.status === 'pending' && (
            <button
              onClick={handleQueue}
              disabled={acting}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              <Play className="h-3.5 w-3.5" />
              Queue
            </button>
          )}
          {task.status === 'pending' && (
            <button
              onClick={handleCancel}
              disabled={acting}
              className="inline-flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
            >
              <Ban className="h-3.5 w-3.5" />
              Cancel
            </button>
          )}
          {(task.status === 'failed' || task.status === 'needs_review') && (
            <button
              onClick={handleRetry}
              disabled={acting}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              <RotateCcw className="h-3.5 w-3.5" />
              Retry
            </button>
          )}
          {task.status === 'needs_review' && (
            <button
              onClick={handleMarkDone}
              disabled={acting}
              className="inline-flex items-center gap-1.5 rounded-md bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-700 disabled:opacity-50"
            >
              <CheckCircle2 className="h-3.5 w-3.5" />
              Mark Done
            </button>
          )}
          {task.status === 'deleted' && (
            <button
              onClick={handleRestore}
              disabled={acting}
              className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
            >
              <Undo2 className="h-3.5 w-3.5" />
              Restore
            </button>
          )}
          {task.status !== 'deleted' && (
            <button
              onClick={handleDelete}
              disabled={acting}
              className="inline-flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 disabled:opacity-50"
            >
              <Trash2 className="h-3.5 w-3.5" />
              Delete
            </button>
          )}
        </div>
      )}

      {/* Token Usage */}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Token Usage
        </h2>
        <TokenUsage task={task} />
      </div>

      {/* Execution History */}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Execution History
        </h2>
        {!task.executions || task.executions.length === 0 ? (
          <p className="text-sm text-zinc-400">No executions yet</p>
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-zinc-200 text-left text-xs font-medium uppercase tracking-wide text-zinc-500">
                  <th className="pb-2 pr-4">#</th>
                  <th className="pb-2 pr-4">Started</th>
                  <th className="pb-2 pr-4">Finished</th>
                  <th className="pb-2 pr-4">Exit</th>
                  <th className="pb-2 pr-4">Cost</th>
                  <th className="pb-2 pr-4">Duration</th>
                  <th className="pb-2">Result</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-100">
                {task.executions.map((exec) => (
                  <ExecutionRow key={exec.id} exec={exec} />
                ))}
              </tbody>
            </table>
          </div>
        )}
      </div>
    </div>
  )
}

function HistoricalOutput({ taskId }: { taskId: string }) {
  const [events, setEvents] = useState<TaskOutputEvent[] | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [expanded, setExpanded] = useState(true)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)

    fetchTaskRawOutput(taskId)
      .then(data => {
        if (cancelled) return
        const parsed = parseNDJSON(data.raw_output)
        setEvents(parsed)
      })
      .catch(err => {
        if (cancelled) return
        if (err instanceof Error && err.message.includes('no output available')) {
          setEvents([])
        } else {
          setError(err instanceof Error ? err.message : 'Failed to load output')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })

    return () => { cancelled = true }
  }, [taskId])

  if (loading) {
    return (
      <div className="overflow-hidden rounded-lg border border-zinc-200">
        <div className="flex items-center gap-2 border-b border-zinc-200 bg-zinc-50 px-4 py-2">
          <Loader2 className="h-4 w-4 animate-spin text-zinc-400" />
          <span className="text-sm text-zinc-500">Loading output...</span>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">
        Failed to load output: {error}
      </div>
    )
  }

  if (!events || events.length === 0) return null

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center justify-between border-b border-zinc-200 bg-zinc-50 px-4 py-2 text-left hover:bg-zinc-100 cursor-pointer"
      >
        <span className="text-sm font-medium text-zinc-700">
          Execution Output
        </span>
        <span className="flex items-center gap-2 text-xs text-zinc-500">
          {events.filter(e => e.type === 'tool_use').length} tool calls
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </span>
      </button>
      {expanded && <TaskOutputView events={events} />}
    </div>
  )
}

function TokenUsage({ task }: { task: Task }) {
  const cells: { label: string; value: string }[] = [
    { label: 'Input', value: formatTokens(task.input_tokens) },
    { label: 'Output', value: formatTokens(task.output_tokens) },
    { label: 'Cache read', value: formatTokens(task.cache_read_tokens) },
    { label: 'Cache creation', value: formatTokens(task.cache_creation_tokens) },
    { label: 'Cost', value: formatCost(task.cost_usd) },
  ]
  return (
    <dl className="grid grid-cols-2 gap-4 sm:grid-cols-5">
      {cells.map((c) => (
        <div key={c.label}>
          <dt className="text-xs uppercase tracking-wide text-zinc-500">{c.label}</dt>
          <dd className="mt-1 text-base font-medium tabular-nums text-zinc-900">{c.value}</dd>
        </div>
      ))}
    </dl>
  )
}

function FailureSummaryBox({
  taskId,
  summary,
  onSummary,
}: {
  taskId: string
  summary: string | null
  onSummary: (s: string) => void
}) {
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState<string | null>(null)

  async function handleRegenerate() {
    setBusy(true)
    setError(null)
    try {
      const result = await regenerateTaskFailureSummary(taskId)
      onSummary(result.failure_summary)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to regenerate summary')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="rounded-lg border border-amber-200 bg-amber-50 p-4">
      <div className="flex items-start gap-3">
        <Sparkles className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-600" />
        <div className="min-w-0 flex-1">
          <div className="mb-1 flex items-center justify-between gap-2">
            <span className="text-xs font-semibold uppercase tracking-wide text-amber-700">
              Shrnutí selhání
            </span>
            <button
              onClick={handleRegenerate}
              disabled={busy}
              className="inline-flex items-center gap-1 rounded-md border border-amber-300 bg-white px-2 py-0.5 text-xs font-medium text-amber-800 hover:bg-amber-100 disabled:opacity-50"
            >
              <RefreshCw className={clsx('h-3 w-3', busy && 'animate-spin')} />
              {busy ? 'Generating…' : 'Regenerate'}
            </button>
          </div>
          {summary ? (
            <p className="text-sm leading-relaxed text-amber-900 whitespace-pre-wrap">{summary}</p>
          ) : (
            <p className="text-sm italic text-amber-700">
              {busy ? 'Generuji shrnutí…' : 'Shrnutí zatím není k dispozici.'}
            </p>
          )}
          {error && <p className="mt-2 text-xs text-red-700">{error}</p>}
        </div>
      </div>
    </div>
  )
}

function ExecutionRow({ exec }: { exec: TaskExecution }) {
  return (
    <tr className="text-zinc-700">
      <td className="py-2 pr-4 tabular-nums">{exec.attempt}</td>
      <td className="whitespace-nowrap py-2 pr-4">{formatDate(exec.started_at)}</td>
      <td className="whitespace-nowrap py-2 pr-4">
        {exec.finished_at ? formatDate(exec.finished_at) : <span className="text-zinc-400">&mdash;</span>}
      </td>
      <td className="py-2 pr-4 tabular-nums">
        {exec.exit_code != null ? (
          <span className={exec.exit_code === 0 ? 'text-emerald-600' : 'text-red-600'}>
            {exec.exit_code}
          </span>
        ) : (
          <span className="text-zinc-400">&mdash;</span>
        )}
      </td>
      <td className="py-2 pr-4 tabular-nums">
        {exec.cost_usd != null ? (
          `$${exec.cost_usd.toFixed(2)}`
        ) : (
          <span className="text-zinc-400">&mdash;</span>
        )}
      </td>
      <td className="py-2 pr-4 tabular-nums">
        {exec.duration_ms != null ? (
          formatDuration(exec.duration_ms)
        ) : (
          <span className="text-zinc-400">&mdash;</span>
        )}
      </td>
      <td className="max-w-xs py-2">
        {exec.summary && <p className="truncate text-zinc-700">{exec.summary}</p>}
        {exec.error_message && <p className="truncate text-red-600">{exec.error_message}</p>}
        {!exec.summary && !exec.error_message && <span className="text-zinc-400">&mdash;</span>}
      </td>
    </tr>
  )
}
