import { useCallback, useEffect, useMemo, useState } from 'react'
import { clsx } from 'clsx'
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Clock,
  Loader2,
  Plus,
  Timer,
  Trash2,
  XCircle,
} from 'lucide-react'

import {
  createCronJob,
  deleteCronJob,
  getModels,
  listCronExecutions,
  listCronJobs,
  triggerCronJob,
  updateCronJob,
} from '../api/client'
import { useProjects } from '../hooks/useProjects'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { describeCron, formatCronLabel, isLikelyValidCron } from '../utils/cronExpression'
import { formatDateTime } from '../utils/dateFormat'
import type { CronExecution, CronExecutionStatus, CronJob, Project } from '../types'

// ── Helpers ───────────────────────────────────────────────────────────────

function formatRelative(iso: string | null | undefined): string {
  if (!iso) return 'never'
  const date = new Date(iso)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  if (diffMs < 0) return 'just now'
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffH = Math.floor(diffMin / 60)
  if (diffH < 24) return `${diffH}h ago`
  const diffD = Math.floor(diffH / 24)
  if (diffD < 30) return `${diffD}d ago`
  const diffMo = Math.floor(diffD / 30)
  return `${diffMo}mo ago`
}

function formatDurationMs(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  const seconds = ms / 1000
  if (seconds < 60) return `${seconds.toFixed(1)}s`
  const minutes = Math.floor(seconds / 60)
  const remainSec = Math.floor(seconds % 60)
  return `${minutes}m ${remainSec}s`
}

function formatCost(cost: number): string {
  if (cost === 0) return '$0.00'
  if (cost < 0.01) return `$${cost.toFixed(4)}`
  return `$${cost.toFixed(2)}`
}

function formatTokens(input: number, output: number): string {
  const total = input + output
  if (total === 0) return '—'
  if (total < 1000) return `${total}`
  return `${(total / 1000).toFixed(1)}k`
}

// ── Status badge ──────────────────────────────────────────────────────────

interface BadgeConfig {
  icon: typeof CheckCircle2
  bg: string
  text: string
  label: string
  pulse?: boolean
}

const statusBadgeConfig: Record<CronExecutionStatus, BadgeConfig> = {
  success: { icon: CheckCircle2, bg: 'bg-emerald-50', text: 'text-emerald-700', label: 'Success' },
  failed: { icon: XCircle, bg: 'bg-red-50', text: 'text-red-700', label: 'Failed' },
  timeout: { icon: AlertTriangle, bg: 'bg-orange-50', text: 'text-orange-700', label: 'Timeout' },
  running: { icon: Loader2, bg: 'bg-blue-50', text: 'text-blue-700', label: 'Running', pulse: true },
}

const neverRunBadge: BadgeConfig = {
  icon: Clock,
  bg: 'bg-zinc-100',
  text: 'text-zinc-500',
  label: 'Never run',
}

function StatusBadge({ status }: { status: string | null | undefined }) {
  const cfg = (status && statusBadgeConfig[status as CronExecutionStatus]) || neverRunBadge
  const Icon = cfg.icon
  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
        cfg.bg,
        cfg.text,
      )}
    >
      <Icon className={clsx('h-3 w-3', cfg.pulse && 'animate-spin')} />
      {cfg.label}
    </span>
  )
}

// ── Main page ─────────────────────────────────────────────────────────────

export default function CronJobsPage() {
  useDocumentTitle('Cron Jobs')

  const [jobs, setJobs] = useState<CronJob[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const [editingJob, setEditingJob] = useState<CronJob | null>(null) // existing job being edited
  const [creating, setCreating] = useState(false) // new job being created
  const [detailJobId, setDetailJobId] = useState<number | null>(null)

  const refetch = useCallback(async () => {
    try {
      setError(null)
      const data = await listCronJobs()
      setJobs(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load cron jobs')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    setLoading(true)
    refetch()
  }, [refetch])

  useRefreshOnFocus(refetch)

  // Poll every 15s while visible — captures last_run_at / last_status updates.
  useEffect(() => {
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') refetch()
    }, 15_000)
    return () => clearInterval(id)
  }, [refetch])

  const handleToggleEnabled = useCallback(
    async (job: CronJob, enabled: boolean) => {
      // Optimistic update
      setJobs((prev) => prev.map((j) => (j.id === job.id ? { ...j, enabled } : j)))
      try {
        await updateCronJob(job.id, { enabled })
      } catch (err) {
        // Revert
        setJobs((prev) => prev.map((j) => (j.id === job.id ? { ...j, enabled: !enabled } : j)))
        setError(err instanceof Error ? err.message : 'Failed to update cron job')
      }
    },
    [],
  )

  const detailJob = useMemo(
    () => (detailJobId == null ? null : jobs.find((j) => j.id === detailJobId) ?? null),
    [detailJobId, jobs],
  )

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <h1 className="text-2xl font-bold text-zinc-900">Cron Jobs</h1>
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
      ) : jobs.length === 0 ? (
        <div className="flex h-48 flex-col items-center justify-center rounded-md border border-dashed border-zinc-200 text-center">
          <Timer className="h-8 w-8 text-zinc-300" />
          <p className="mt-2 text-sm font-medium text-zinc-700">No cron jobs configured.</p>
          <p className="text-sm text-zinc-500">Schedule your first AI task.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {jobs.map((job) => (
            <CronJobCard
              key={job.id}
              job={job}
              onClick={() => setDetailJobId(job.id)}
              onToggleEnabled={(enabled) => handleToggleEnabled(job, enabled)}
            />
          ))}
        </div>
      )}

      {/* Add modal */}
      {creating && (
        <CronJobFormModal
          onClose={() => setCreating(false)}
          onSaved={() => {
            setCreating(false)
            refetch()
          }}
        />
      )}

      {/* Edit modal */}
      {editingJob && (
        <CronJobFormModal
          job={editingJob}
          onClose={() => setEditingJob(null)}
          onSaved={() => {
            setEditingJob(null)
            refetch()
          }}
        />
      )}

      {/* Detail modal */}
      {detailJob && (
        <CronJobDetailModal
          job={detailJob}
          onClose={() => setDetailJobId(null)}
          onEdit={() => {
            setEditingJob(detailJob)
            setDetailJobId(null)
          }}
          onChanged={refetch}
          onDeleted={() => {
            setDetailJobId(null)
            refetch()
          }}
        />
      )}
    </div>
  )
}

// ── Card ──────────────────────────────────────────────────────────────────

function CronJobCard({
  job,
  onClick,
  onToggleEnabled,
}: {
  job: CronJob
  onClick: () => void
  onToggleEnabled: (enabled: boolean) => void
}) {
  const description = describeCron(job.schedule)
  const lastRunIso = job.last_run_at ?? null

  return (
    <div
      onClick={onClick}
      className={clsx(
        'group flex cursor-pointer flex-col gap-3 rounded-md border border-zinc-200 bg-white p-4 transition-colors hover:border-zinc-300 hover:bg-zinc-50',
        !job.enabled && 'opacity-70',
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h3 className="truncate text-base font-semibold text-zinc-900">{job.name}</h3>
            {job.project && (
              <span className="inline-flex items-center rounded-full bg-zinc-100 px-2 py-0.5 text-xs font-medium text-zinc-600">
                {job.project.name}
              </span>
            )}
          </div>
          <p className="mt-1 text-sm text-zinc-600">
            {description ? (
              <>
                {description}{' '}
                <code className="ml-1 rounded bg-zinc-100 px-1 py-0.5 font-mono text-xs text-zinc-500">
                  {job.schedule}
                </code>
              </>
            ) : (
              <code className="rounded bg-zinc-100 px-1 py-0.5 font-mono text-xs text-zinc-500">
                {job.schedule}
              </code>
            )}
          </p>
          {job.prompt && (
            <p className="mt-1 truncate text-xs text-zinc-400" title={job.prompt}>
              {job.prompt}
            </p>
          )}
        </div>

        <label
          className="flex shrink-0 items-center gap-1.5"
          onClick={(e) => e.stopPropagation()}
        >
          <input
            type="checkbox"
            checked={job.enabled}
            onChange={(e) => onToggleEnabled(e.target.checked)}
            className="h-4 w-4 cursor-pointer accent-zinc-900"
          />
          <span className="text-xs text-zinc-500">Enabled</span>
        </label>
      </div>

      <div className="flex items-center gap-3 text-xs text-zinc-500">
        <span title={lastRunIso ? formatDateTime(lastRunIso) : undefined}>
          Last run: {formatRelative(lastRunIso)}
        </span>
        <StatusBadge status={job.last_status} />
      </div>
    </div>
  )
}

// ── Add / Edit Form Modal ────────────────────────────────────────────────

function CronJobFormModal({
  job,
  onClose,
  onSaved,
}: {
  job?: CronJob
  onClose: () => void
  onSaved: () => void
}) {
  const isEdit = !!job
  const { projects } = useProjects()
  const activeProjects = useMemo(
    () => projects.filter((p) => p.active).sort((a, b) => a.name.localeCompare(b.name)),
    [projects],
  )

  const [models, setModels] = useState<string[]>([])
  const [name, setName] = useState(job?.name ?? '')
  const [schedule, setSchedule] = useState(job?.schedule ?? '')
  const [prompt, setPrompt] = useState(job?.prompt ?? '')
  const [projectId, setProjectId] = useState(job?.project_id ?? '')
  const [model, setModel] = useState(job?.model ?? '')
  const [timeoutMinutes, setTimeoutMinutes] = useState(job?.timeout_minutes ?? 30)
  const [enabled, setEnabled] = useState(job?.enabled ?? true)

  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    getModels()
      .then((r) => setModels(r.models))
      .catch(() => {})
  }, [])

  const scheduleDescription = useMemo(() => describeCron(schedule), [schedule])
  const scheduleValid = useMemo(() => isLikelyValidCron(schedule), [schedule])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setError(null)

    if (!name.trim() || !schedule.trim() || !prompt.trim() || !projectId) {
      setError('Name, schedule, project, and prompt are required.')
      return
    }
    if (!scheduleValid) {
      setError('Invalid cron expression.')
      return
    }

    setSaving(true)
    try {
      if (isEdit && job) {
        await updateCronJob(job.id, {
          name: name.trim(),
          schedule: schedule.trim(),
          prompt,
          enabled,
          timeout_minutes: timeoutMinutes,
          model: model || '',
        })
      } else {
        await createCronJob({
          name: name.trim(),
          schedule: schedule.trim(),
          prompt,
          project_id: projectId,
          enabled,
          timeout_minutes: timeoutMinutes,
          model: model || undefined,
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
    <ModalShell title={isEdit ? 'Edit Cron Job' : 'New Cron Job'} onClose={onClose}>
      <form onSubmit={handleSubmit} className="space-y-4 p-5">
        {error && (
          <div className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div>
        )}

        {/* Name */}
        <div>
          <label className="mb-1 block text-sm font-medium text-zinc-700">Name</label>
          <input
            type="text"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder="Daily standup digest"
            className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
          />
        </div>

        {/* Schedule */}
        <div>
          <label className="mb-1 block text-sm font-medium text-zinc-700">Schedule</label>
          <input
            type="text"
            value={schedule}
            onChange={(e) => setSchedule(e.target.value)}
            required
            placeholder="0 9 * * 1"
            className={clsx(
              'w-full rounded-md border px-3 py-2 font-mono text-sm text-zinc-900 focus:outline-none focus:ring-1',
              schedule && !scheduleValid
                ? 'border-red-300 focus:border-red-500 focus:ring-red-500'
                : 'border-zinc-300 focus:border-zinc-500 focus:ring-zinc-500',
            )}
          />
          <p
            className={clsx(
              'mt-1 text-xs',
              !schedule
                ? 'text-zinc-400'
                : !scheduleValid
                  ? 'text-red-600'
                  : 'text-zinc-500',
            )}
          >
            {!schedule
              ? '5-field cron expression (minute hour day-of-month month day-of-week)'
              : !scheduleValid
                ? 'Invalid cron expression'
                : scheduleDescription
                  ? `Runs ${scheduleDescription.replace(/^Every/, 'every').replace(/^Once/, 'once').replace(/^On/, 'on')}`
                  : 'Custom schedule'}
          </p>
        </div>

        {/* Project */}
        <div>
          <label className="mb-1 block text-sm font-medium text-zinc-700">Project</label>
          {isEdit ? (
            <p className="text-sm text-zinc-600">
              {activeProjects.find((p) => p.id === projectId)?.name ?? job?.project?.name ?? projectId}
            </p>
          ) : (
            <ProjectSelect
              projects={activeProjects}
              value={projectId}
              onChange={setProjectId}
            />
          )}
        </div>

        {/* Prompt */}
        <div>
          <label className="mb-1 block text-sm font-medium text-zinc-700">Prompt</label>
          <textarea
            value={prompt}
            onChange={(e) => setPrompt(e.target.value)}
            required
            rows={10}
            placeholder="Write a daily summary of recent commits and open PRs..."
            className="w-full resize-y rounded-md border border-zinc-300 px-3 py-2 font-mono text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
          />
        </div>

        {/* Model + Timeout row */}
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">Model</label>
            <select
              value={model ?? ''}
              onChange={(e) => setModel(e.target.value)}
              className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            >
              <option value="">Default</option>
              {models.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </div>
          <div>
            <label className="mb-1 block text-sm font-medium text-zinc-700">
              Timeout (minutes)
            </label>
            <input
              type="number"
              min={1}
              value={timeoutMinutes}
              onChange={(e) => setTimeoutMinutes(Math.max(1, Number(e.target.value) || 1))}
              className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
          </div>
        </div>

        {/* Enabled */}
        <label className="inline-flex items-center gap-2 text-sm text-zinc-700">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
            className="h-4 w-4 accent-zinc-900"
          />
          Enabled
        </label>

        {/* Buttons */}
        <div className="flex items-center gap-3 border-t border-zinc-200 pt-4">
          <button
            type="submit"
            disabled={saving}
            className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
          >
            {saving && <Loader2 className="h-4 w-4 animate-spin" />}
            {isEdit ? 'Save Changes' : 'Create'}
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
    </ModalShell>
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

// ── Detail Modal ─────────────────────────────────────────────────────────

const EXECUTION_PAGE_SIZE = 20

function CronJobDetailModal({
  job,
  onClose,
  onEdit,
  onChanged,
  onDeleted,
}: {
  job: CronJob
  onClose: () => void
  onEdit: () => void
  onChanged: () => void
  onDeleted: () => void
}) {
  const [executions, setExecutions] = useState<CronExecution[]>([])
  const [executionsTotal, setExecutionsTotal] = useState(0)
  const [executionsLoading, setExecutionsLoading] = useState(true)
  const [executionsError, setExecutionsError] = useState<string | null>(null)
  const [expandedExecutionId, setExpandedExecutionId] = useState<number | null>(null)

  const [triggering, setTriggering] = useState(false)
  const [triggerInfo, setTriggerInfo] = useState<string | null>(null)
  const [confirmingDelete, setConfirmingDelete] = useState(false)
  const [deleting, setDeleting] = useState(false)

  const refetchExecutions = useCallback(async () => {
    try {
      setExecutionsError(null)
      const res = await listCronExecutions(job.id, { limit: EXECUTION_PAGE_SIZE, offset: 0 })
      setExecutions(res.data)
      setExecutionsTotal(res.total)
    } catch (err) {
      setExecutionsError(err instanceof Error ? err.message : 'Failed to load executions')
    } finally {
      setExecutionsLoading(false)
    }
  }, [job.id])

  useEffect(() => {
    setExecutionsLoading(true)
    refetchExecutions()
  }, [refetchExecutions])

  // Re-poll while detail modal is open in case a manual run finishes.
  useEffect(() => {
    const id = setInterval(() => {
      if (document.visibilityState === 'visible') refetchExecutions()
    }, 5_000)
    return () => clearInterval(id)
  }, [refetchExecutions])

  async function loadMore() {
    try {
      const res = await listCronExecutions(job.id, {
        limit: EXECUTION_PAGE_SIZE,
        offset: executions.length,
      })
      setExecutions((prev) => [...prev, ...res.data])
      setExecutionsTotal(res.total)
    } catch (err) {
      setExecutionsError(err instanceof Error ? err.message : 'Failed to load more')
    }
  }

  async function handleRunNow() {
    setTriggering(true)
    setTriggerInfo(null)
    try {
      await triggerCronJob(job.id)
      setTriggerInfo('Execution started.')
      await refetchExecutions()
      onChanged()
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Failed to trigger'
      // Surface the "already running" feedback explicitly.
      setTriggerInfo(/already running|already in progress/i.test(msg) ? 'Already running.' : msg)
    } finally {
      setTriggering(false)
    }
  }

  async function handleDelete() {
    setDeleting(true)
    try {
      await deleteCronJob(job.id)
      onDeleted()
    } catch (err) {
      setExecutionsError(err instanceof Error ? err.message : 'Failed to delete')
      setDeleting(false)
    }
  }

  return (
    <ModalShell title={job.name} subtitle={formatCronLabel(job.schedule)} onClose={onClose} wide>
      <div className="flex flex-col gap-5 p-5">
        {/* Top action bar */}
        <div className="flex flex-wrap items-center gap-2">
          <button
            onClick={handleRunNow}
            disabled={triggering}
            className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
          >
            {triggering && <Loader2 className="h-3.5 w-3.5 animate-spin" />}
            Run Now
          </button>
          <button
            onClick={onEdit}
            className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
          >
            Edit
          </button>
          <div className="ml-auto">
            {confirmingDelete ? (
              <div className="flex items-center gap-2">
                <span className="text-xs text-zinc-600">Delete this cron job?</span>
                <button
                  onClick={handleDelete}
                  disabled={deleting}
                  className="inline-flex items-center gap-1 rounded-md bg-red-600 px-2.5 py-1 text-xs font-medium text-white hover:bg-red-700 disabled:opacity-50"
                >
                  {deleting && <Loader2 className="h-3 w-3 animate-spin" />}
                  Confirm
                </button>
                <button
                  onClick={() => setConfirmingDelete(false)}
                  disabled={deleting}
                  className="rounded-md border border-zinc-300 px-2.5 py-1 text-xs font-medium text-zinc-600 hover:bg-zinc-50"
                >
                  Cancel
                </button>
              </div>
            ) : (
              <button
                onClick={() => setConfirmingDelete(true)}
                className="inline-flex items-center gap-1 rounded-md border border-red-200 px-3 py-1.5 text-sm font-medium text-red-700 hover:bg-red-50"
              >
                <Trash2 className="h-3.5 w-3.5" />
                Delete
              </button>
            )}
          </div>
        </div>

        {triggerInfo && (
          <div className="rounded-md bg-blue-50 px-3 py-2 text-sm text-blue-700">{triggerInfo}</div>
        )}

        {/* Job summary */}
        <dl className="grid grid-cols-1 gap-3 rounded-md border border-zinc-200 bg-zinc-50 p-4 text-sm sm:grid-cols-2">
          <Field label="Schedule">
            <code className="font-mono text-xs text-zinc-700">{job.schedule}</code>
          </Field>
          <Field label="Project">{job.project?.name ?? '—'}</Field>
          <Field label="Model">{job.model ?? 'Default'}</Field>
          <Field label="Timeout">{job.timeout_minutes}m</Field>
          <Field label="Enabled">{job.enabled ? 'Yes' : 'No'}</Field>
          <Field label="Last run">
            {job.last_run_at ? (
              <span title={formatDateTime(job.last_run_at)}>
                {formatRelative(job.last_run_at)}
              </span>
            ) : (
              'never'
            )}{' '}
            <StatusBadge status={job.last_status} />
          </Field>
        </dl>

        {/* Prompt */}
        <div>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">
            Prompt
          </h3>
          <pre className="max-h-64 overflow-auto whitespace-pre-wrap rounded-md border border-zinc-200 bg-zinc-50 p-3 font-mono text-xs text-zinc-800">
            {job.prompt}
          </pre>
        </div>

        {/* Executions */}
        <div>
          <h3 className="mb-2 text-xs font-semibold uppercase tracking-wide text-zinc-500">
            Execution history{' '}
            <span className="ml-1 text-zinc-400">({executionsTotal})</span>
          </h3>

          {executionsError && (
            <div className="mb-2 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
              {executionsError}
            </div>
          )}

          {executionsLoading ? (
            <div className="flex h-24 items-center justify-center">
              <Loader2 className="h-5 w-5 animate-spin text-zinc-400" />
            </div>
          ) : executions.length === 0 ? (
            <p className="rounded-md border border-dashed border-zinc-200 p-4 text-center text-sm text-zinc-400">
              No executions yet.
            </p>
          ) : (
            <ul className="divide-y divide-zinc-200 rounded-md border border-zinc-200">
              {executions.map((exec) => (
                <ExecutionRow
                  key={exec.id}
                  execution={exec}
                  expanded={expandedExecutionId === exec.id}
                  onToggle={() =>
                    setExpandedExecutionId((prev) => (prev === exec.id ? null : exec.id))
                  }
                />
              ))}
            </ul>
          )}

          {executions.length < executionsTotal && (
            <div className="mt-3 flex justify-center">
              <button
                onClick={loadMore}
                className="rounded-md border border-zinc-300 px-3 py-1.5 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
              >
                Load more
              </button>
            </div>
          )}
        </div>
      </div>
    </ModalShell>
  )
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="flex flex-col">
      <dt className="text-xs font-medium uppercase tracking-wide text-zinc-400">{label}</dt>
      <dd className="mt-0.5 text-sm text-zinc-800">{children}</dd>
    </div>
  )
}

function ExecutionRow({
  execution,
  expanded,
  onToggle,
}: {
  execution: CronExecution
  expanded: boolean
  onToggle: () => void
}) {
  const Chevron = expanded ? ChevronDown : ChevronRight
  return (
    <li>
      <button
        onClick={onToggle}
        className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-zinc-50"
      >
        <Chevron className="h-3.5 w-3.5 shrink-0 text-zinc-400" />
        <span
          className="w-44 shrink-0 truncate text-zinc-700"
          title={formatDateTime(execution.started_at)}
        >
          {formatRelative(execution.started_at)}
        </span>
        <span className="w-20 shrink-0 text-xs text-zinc-500">
          {formatDurationMs(execution.duration_ms)}
        </span>
        <span className="shrink-0">
          <StatusBadge status={execution.status} />
        </span>
        <span className="ml-auto flex items-center gap-3 text-xs text-zinc-500">
          <span>{formatCost(execution.cost_usd)}</span>
          <span>{formatTokens(execution.input_tokens, execution.output_tokens)} tok</span>
        </span>
      </button>
      {expanded && (
        <div className="border-t border-zinc-200 bg-zinc-50 p-3 text-xs">
          {execution.error_message && (
            <div className="mb-2 rounded-md bg-red-50 px-3 py-2 text-red-700">
              {execution.error_message}
            </div>
          )}
          <pre className="max-h-96 overflow-auto whitespace-pre-wrap font-mono text-zinc-800">
            {execution.output ?? '(no output)'}
          </pre>
        </div>
      )}
    </li>
  )
}

// ── Modal shell ──────────────────────────────────────────────────────────

function ModalShell({
  title,
  subtitle,
  onClose,
  wide,
  children,
}: {
  title: string
  subtitle?: string
  onClose: () => void
  wide?: boolean
  children: React.ReactNode
}) {
  // Close on Escape.
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/40 p-4"
      onClick={onClose}
    >
      <div
        className={clsx(
          'flex w-full flex-col rounded-xl bg-white shadow-xl dark:bg-zinc-100',
          wide ? 'max-w-3xl' : 'max-w-xl',
          'max-h-[90vh] overflow-hidden',
        )}
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex items-start justify-between border-b border-zinc-100 px-5 py-3">
          <div className="min-w-0">
            <h2 className="truncate text-base font-semibold text-zinc-900">{title}</h2>
            {subtitle && (
              <p className="mt-0.5 truncate text-xs text-zinc-500">{subtitle}</p>
            )}
          </div>
          <button
            onClick={onClose}
            className="ml-3 shrink-0 rounded-md p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600"
            aria-label="Close"
          >
            <XCircle className="h-5 w-5" />
          </button>
        </div>
        <div className="flex-1 overflow-auto">{children}</div>
      </div>
    </div>
  )
}
