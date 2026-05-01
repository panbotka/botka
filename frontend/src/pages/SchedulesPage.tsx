import { useCallback, useEffect, useMemo, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  CalendarClock,
  Loader2,
  Pencil,
  Play,
  Plus,
  Trash2,
  XCircle,
} from 'lucide-react'

import {
  createSchedule,
  deleteSchedule,
  listSchedules,
  runScheduleNow,
  updateSchedule,
} from '../api/client'
import { useProjects } from '../hooks/useProjects'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { describeCron, isLikelyValidCron } from '../utils/cronExpression'
import { formatDateTime } from '../utils/dateFormat'
import type { Project, TaskSchedule } from '../types'

function formatRelativeFuture(iso: string | null | undefined): string {
  if (!iso) return '—'
  const d = new Date(iso)
  const now = new Date()
  const diffMs = d.getTime() - now.getTime()
  if (diffMs <= 0) return 'overdue'
  const diffMin = Math.round(diffMs / 60000)
  if (diffMin < 60) return `in ${diffMin}m`
  const diffH = Math.floor(diffMin / 60)
  if (diffH < 24) return `in ${diffH}h ${diffMin % 60}m`
  const diffD = Math.floor(diffH / 24)
  return `in ${diffD}d ${diffH % 24}h`
}

export default function SchedulesPage() {
  useDocumentTitle('Schedules')

  const [schedules, setSchedules] = useState<TaskSchedule[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [creating, setCreating] = useState(false)
  const [editing, setEditing] = useState<TaskSchedule | null>(null)

  const refetch = useCallback(async () => {
    try {
      setError(null)
      const data = await listSchedules()
      setSchedules(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load schedules')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    setLoading(true)
    refetch()
  }, [refetch])

  useRefreshOnFocus(refetch)

  // Periodically refresh so next_run_at relative labels stay fresh.
  useEffect(() => {
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') refetch()
    }, 30_000)
    return () => clearInterval(id)
  }, [refetch])

  const handleToggleEnabled = useCallback(
    async (sched: TaskSchedule, enabled: boolean) => {
      setSchedules((prev) => prev.map((s) => (s.id === sched.id ? { ...s, enabled } : s)))
      try {
        await updateSchedule(sched.id, { enabled })
      } catch (err) {
        setSchedules((prev) =>
          prev.map((s) => (s.id === sched.id ? { ...s, enabled: !enabled } : s)),
        )
        setError(err instanceof Error ? err.message : 'Failed to update schedule')
      }
    },
    [],
  )

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900">Schedules</h1>
        <button
          onClick={() => setCreating(true)}
          className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800"
        >
          <Plus className="h-4 w-4" />
          Add
        </button>
      </div>

      {error && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
        </div>
      ) : schedules.length === 0 ? (
        <div className="flex h-48 flex-col items-center justify-center rounded-md border border-dashed border-zinc-200 text-center">
          <CalendarClock className="h-8 w-8 text-zinc-300" />
          <p className="mt-2 text-sm font-medium text-zinc-700">No recurring schedules.</p>
          <p className="text-sm text-zinc-500">
            Create a schedule to auto-create tasks on a cron cadence.
          </p>
        </div>
      ) : (
        <div className="space-y-2">
          {schedules.map((sched) => (
            <ScheduleRow
              key={sched.id}
              schedule={sched}
              onToggleEnabled={(enabled) => handleToggleEnabled(sched, enabled)}
              onEdit={() => setEditing(sched)}
              onDeleted={refetch}
              onRan={refetch}
              onError={(msg) => setError(msg)}
            />
          ))}
        </div>
      )}

      {creating && (
        <ScheduleFormModal
          onClose={() => setCreating(false)}
          onSaved={() => {
            setCreating(false)
            refetch()
          }}
        />
      )}

      {editing && (
        <ScheduleFormModal
          schedule={editing}
          onClose={() => setEditing(null)}
          onSaved={() => {
            setEditing(null)
            refetch()
          }}
        />
      )}
    </div>
  )
}

function ScheduleRow({
  schedule,
  onToggleEnabled,
  onEdit,
  onDeleted,
  onRan,
  onError,
}: {
  schedule: TaskSchedule
  onToggleEnabled: (enabled: boolean) => void
  onEdit: () => void
  onDeleted: () => void
  onRan: () => void
  onError: (msg: string) => void
}) {
  const navigate = useNavigate()
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [running, setRunning] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const description = describeCron(schedule.cron_expression)

  async function handleRunNow() {
    setRunning(true)
    try {
      const { task_id } = await runScheduleNow(schedule.id)
      onRan()
      navigate(`/tasks/${task_id}`)
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Failed to run schedule')
    } finally {
      setRunning(false)
    }
  }

  async function handleDelete() {
    setDeleting(true)
    try {
      await deleteSchedule(schedule.id)
      onDeleted()
    } catch (err) {
      onError(err instanceof Error ? err.message : 'Failed to delete')
      setDeleting(false)
    }
  }

  return (
    <div
      className={clsx(
        'flex flex-col gap-3 rounded-md border border-zinc-200 bg-white p-4',
        !schedule.enabled && 'opacity-70',
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-base font-semibold text-zinc-900">{schedule.title}</h3>
            {schedule.project && (
              <span className="inline-flex items-center rounded-full bg-zinc-100 px-2 py-0.5 text-xs font-medium text-zinc-600">
                {schedule.project.name}
              </span>
            )}
            {schedule.priority !== 0 && (
              <span className="inline-flex items-center rounded-full bg-amber-50 px-2 py-0.5 text-xs font-medium text-amber-700">
                P{schedule.priority}
              </span>
            )}
          </div>
          <p className="mt-1 text-sm text-zinc-600">
            {description ? (
              <>
                {description}{' '}
                <code className="ml-1 rounded bg-zinc-100 px-1 py-0.5 font-mono text-xs text-zinc-500">
                  {schedule.cron_expression}
                </code>
              </>
            ) : (
              <code className="rounded bg-zinc-100 px-1 py-0.5 font-mono text-xs text-zinc-500">
                {schedule.cron_expression}
              </code>
            )}
          </p>
        </div>

        <label
          className="flex shrink-0 items-center gap-1.5"
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            checked={schedule.enabled}
            onChange={(e) => onToggleEnabled(e.target.checked)}
            className="h-4 w-4 cursor-pointer accent-zinc-900"
          />
          <span className="text-xs text-zinc-500">Enabled</span>
        </label>
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-zinc-500">
        <div className="flex flex-wrap items-center gap-3">
          <span title={schedule.next_run_at ? formatDateTime(schedule.next_run_at) : undefined}>
            Next run:{' '}
            {schedule.enabled ? (
              <span className="font-medium text-zinc-700">
                {formatRelativeFuture(schedule.next_run_at)}
              </span>
            ) : (
              <span className="italic text-zinc-400">disabled</span>
            )}
          </span>
          <span title={schedule.last_run_at ? formatDateTime(schedule.last_run_at) : undefined}>
            Last run:{' '}
            {schedule.last_run_at ? formatDateTime(schedule.last_run_at) : 'never'}
          </span>
        </div>
        <div className="flex items-center gap-1">
          <button
            onClick={handleRunNow}
            disabled={running}
            className="inline-flex items-center gap-1 rounded-md border border-zinc-300 px-2 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-50 disabled:opacity-50"
          >
            {running ? <Loader2 className="h-3 w-3 animate-spin" /> : <Play className="h-3 w-3" />}
            Run now
          </button>
          <button
            onClick={onEdit}
            className="inline-flex items-center gap-1 rounded-md border border-zinc-300 px-2 py-1 text-xs font-medium text-zinc-700 hover:bg-zinc-50"
          >
            <Pencil className="h-3 w-3" />
            Edit
          </button>
          {confirmingDelete ? (
            <>
              <button
                onClick={handleDelete}
                disabled={deleting}
                className="inline-flex items-center gap-1 rounded-md bg-red-600 px-2 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
              >
                {deleting && <Loader2 className="h-3 w-3 animate-spin" />}
                Confirm
              </button>
              <button
                onClick={() => setConfirmingDelete(false)}
                disabled={deleting}
                className="rounded-md border border-zinc-300 px-2 py-1 text-xs font-medium text-zinc-600 hover:bg-zinc-50"
              >
                Cancel
              </button>
            </>
          ) : (
            <button
              onClick={() => setConfirmingDelete(true)}
              className="inline-flex items-center gap-1 rounded-md border border-red-200 px-2 py-1 text-xs font-medium text-red-700 hover:bg-red-50"
            >
              <Trash2 className="h-3 w-3" />
              Delete
            </button>
          )}
        </div>
      </div>

      <details className="rounded-md border border-zinc-100 bg-zinc-50 p-2 text-xs text-zinc-600">
        <summary className="cursor-pointer select-none font-medium text-zinc-700">Spec</summary>
        <pre className="mt-2 max-h-48 overflow-auto whitespace-pre-wrap font-mono text-[11px] text-zinc-700">
          {schedule.spec || <span className="italic text-zinc-400">(empty)</span>}
        </pre>
      </details>
    </div>
  )
}

function ScheduleFormModal({
  schedule,
  onClose,
  onSaved,
}: {
  schedule?: TaskSchedule
  onClose: () => void
  onSaved: () => void
}) {
  const isEdit = !!schedule
  const { projects } = useProjects()
  const activeProjects = useMemo(
    () => projects.filter((p) => p.active).sort((a, b) => a.name.localeCompare(b.name)),
    [projects],
  )

  const [title, setTitle] = useState(schedule?.title ?? '')
  const [spec, setSpec] = useState(schedule?.spec ?? '')
  const [cronExpression, setCronExpression] = useState(schedule?.cron_expression ?? '')
  const [priority, setPriority] = useState(schedule?.priority ?? 0)
  const [projectId, setProjectId] = useState(schedule?.project_id ?? '')
  const [enabled, setEnabled] = useState(schedule?.enabled ?? true)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const cronDescription = useMemo(() => describeCron(cronExpression), [cronExpression])
  const cronValid = useMemo(() => isLikelyValidCron(cronExpression), [cronExpression])

  // Close on Escape.
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (!title.trim() || !cronExpression.trim() || (!isEdit && !projectId)) {
      setError('Title, cron expression, and project are required.')
      return
    }
    if (!cronValid) {
      setError('Invalid cron expression.')
      return
    }

    setSaving(true)
    try {
      if (isEdit && schedule) {
        await updateSchedule(schedule.id, {
          title: title.trim(),
          spec,
          cron_expression: cronExpression.trim(),
          priority,
          enabled,
        })
      } else {
        await createSchedule({
          project_id: projectId,
          title: title.trim(),
          spec,
          cron_expression: cronExpression.trim(),
          priority,
          enabled,
        })
      }
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={onClose}
    >
      <div
        className="flex w-full max-w-xl flex-col rounded-xl bg-white shadow-xl max-h-[90vh] overflow-hidden"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between border-b border-zinc-100 px-5 py-3">
          <h2 className="truncate text-base font-semibold text-zinc-900">
            {isEdit ? 'Edit Schedule' : 'New Schedule'}
          </h2>
          <button
            onClick={onClose}
            className="ml-3 shrink-0 rounded-md p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600"
            aria-label="Close"
          >
            <XCircle className="h-5 w-5" />
          </button>
        </div>
        <form onSubmit={handleSubmit} className="flex-1 space-y-4 overflow-auto p-5">
          {error && (
            <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div>
          )}

          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">Title</label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              required
              placeholder="Daily housekeeping"
              className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">Cron expression</label>
            <input
              type="text"
              value={cronExpression}
              onChange={(e) => setCronExpression(e.target.value)}
              required
              placeholder="0 9 * * *"
              className={clsx(
                'w-full rounded-md border px-3 py-2 font-mono text-sm text-zinc-900 focus:outline-none focus:ring-1',
                cronExpression && !cronValid
                  ? 'border-red-300 focus:border-red-500 focus:ring-red-500'
                  : 'border-zinc-300 focus:border-zinc-500 focus:ring-zinc-500',
              )}
            />
            <p
              className={clsx(
                'mt-1 text-xs',
                !cronExpression
                  ? 'text-zinc-400'
                  : !cronValid
                    ? 'text-red-600'
                    : 'text-zinc-500',
              )}
            >
              {!cronExpression
                ? '5-field cron expression (minute hour day-of-month month day-of-week). Supports */N, ranges, lists.'
                : !cronValid
                  ? 'Invalid cron expression'
                  : cronDescription
                    ? `Runs ${cronDescription.replace(/^Every/, 'every').replace(/^Once/, 'once').replace(/^On/, 'on')}`
                    : 'Custom schedule'}
            </p>
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">Project</label>
            {isEdit ? (
              <p className="text-sm text-zinc-600">
                {schedule?.project?.name ?? activeProjects.find((p) => p.id === projectId)?.name ?? projectId}
              </p>
            ) : (
              <ProjectSelect
                projects={activeProjects}
                value={projectId}
                onChange={setProjectId}
              />
            )}
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">Spec</label>
            <textarea
              value={spec}
              onChange={(e) => setSpec(e.target.value)}
              rows={8}
              placeholder="Describe what the recurring task should do..."
              className="w-full resize-y rounded-md border border-zinc-300 px-3 py-2 font-mono text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
          </div>

          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">Priority</label>
            <input
              type="number"
              value={priority}
              onChange={(e) => setPriority(Number(e.target.value) || 0)}
              className="w-32 rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
            <p className="mt-1 text-xs text-zinc-500">
              Higher priorities are picked first by the task runner.
            </p>
          </div>

          <label className="inline-flex items-center gap-2 text-sm text-zinc-700">
            <input
              type="checkbox"
              checked={enabled}
              onChange={(e) => setEnabled(e.target.checked)}
              className="h-4 w-4 accent-zinc-900"
            />
            Enabled
          </label>

          <div className="flex items-center gap-3 border-t border-zinc-200 pt-4">
            <button
              type="submit"
              disabled={saving}
              className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
            >
              {saving && <Loader2 className="h-4 w-4 animate-spin" />}
              {isEdit ? 'Save changes' : 'Create'}
            </button>
            <button
              type="button"
              onClick={onClose}
              className="rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
            >
              Cancel
            </button>
          </div>
        </form>
      </div>
    </div>
  )
}

function ProjectSelect({
  projects,
  value,
  onChange,
}: {
  projects: Project[]
  value: string
  onChange: (id: string) => void
}) {
  return (
    <select
      value={value}
      onChange={(e) => onChange(e.target.value)}
      required
      className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
    >
      <option value="">Select a project...</option>
      {projects.map((p) => (
        <option key={p.id} value={p.id}>
          {p.name}
        </option>
      ))}
    </select>
  )
}
