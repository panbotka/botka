import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate, Link } from 'react-router-dom'
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
} from 'lucide-react'
import { TaskForm } from '../components/TaskForm'
import { LiveOutputInline } from '../components/LiveOutput'
import { fetchTask, retryTask, deleteTask, updateTask } from '../api/client'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import type { Task, TaskStatus, TaskExecution } from '../types'

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
  return new Date(iso).toLocaleString()
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
  useDocumentTitle(task?.title || 'Task')

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

      {/* Failure reason */}
      {task.failure_reason && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">
          <span className="font-medium">Failure:</span> {task.failure_reason}
        </div>
      )}

      {/* Spec */}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">Spec</h2>
        {task.spec ? (
          <div className="prose prose-sm prose-zinc max-w-none">
            <ReactMarkdown>{task.spec}</ReactMarkdown>
          </div>
        ) : (
          <p className="text-sm text-zinc-400">No spec provided</p>
        )}
      </div>

      {/* Live Output */}
      {task.status === 'running' && (
        <LiveOutputInline taskId={taskId} taskTitle={task.title} />
      )}

      {/* Actions */}
      {task.status !== 'running' && (
        <div className="flex gap-3">
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
