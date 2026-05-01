import { useState, useEffect, useMemo } from 'react'
import { clsx } from 'clsx'
import { ArrowUpDown, ListChecks, FolderOpen, Trash2, X, AlertTriangle } from 'lucide-react'

import type { BulkTaskAction } from '../api/client'
import type { Project, TaskStatus } from '../types'

const SETTABLE_STATUSES: { value: TaskStatus; label: string }[] = [
  { value: 'pending', label: 'Pending' },
  { value: 'queued', label: 'Queued' },
  { value: 'cancelled', label: 'Cancelled' },
]

type ActiveModal = 'priority' | 'status' | 'project' | 'delete' | null

interface BulkActionsBarProps {
  count: number
  projects: Project[]
  /** Apply an action; rejecting the promise leaves the modal open with the error. */
  onAction: (action: BulkTaskAction, value?: number | string) => Promise<void>
  onClear: () => void
}

export function BulkActionsBar({ count, projects, onAction, onClear }: BulkActionsBarProps) {
  const [modal, setModal] = useState<ActiveModal>(null)

  // Close modal automatically if selection clears (e.g., via Esc / Deselect).
  useEffect(() => {
    if (count === 0) setModal(null)
  }, [count])

  const close = () => setModal(null)

  const apply = async (action: BulkTaskAction, value?: number | string) => {
    await onAction(action, value)
    close()
  }

  return (
    <>
      <div
        className="fixed inset-x-0 bottom-0 z-40 flex justify-center pointer-events-none px-3 pb-4"
        role="region"
        aria-label="Bulk actions"
      >
        <div className="pointer-events-auto flex flex-wrap items-center gap-2 rounded-full border border-zinc-200 bg-white dark:bg-zinc-100 px-4 py-2 shadow-xl shadow-black/10">
          <span className="text-sm font-medium text-zinc-800">
            {count} selected
          </span>
          <span className="h-4 w-px bg-zinc-200" aria-hidden="true" />
          <BarButton icon={<ArrowUpDown className="h-3.5 w-3.5" />} onClick={() => setModal('priority')}>
            Priority
          </BarButton>
          <BarButton icon={<ListChecks className="h-3.5 w-3.5" />} onClick={() => setModal('status')}>
            Status
          </BarButton>
          <BarButton icon={<FolderOpen className="h-3.5 w-3.5" />} onClick={() => setModal('project')}>
            Project
          </BarButton>
          <BarButton
            icon={<Trash2 className="h-3.5 w-3.5" />}
            onClick={() => setModal('delete')}
            tone="danger"
          >
            Delete
          </BarButton>
          <button
            onClick={onClear}
            className="ml-1 inline-flex items-center justify-center rounded-full p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700"
            aria-label="Clear selection"
            title="Clear selection"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
      </div>

      {modal === 'priority' && (
        <PriorityModal
          count={count}
          onCancel={close}
          onConfirm={(p) => apply('set_priority', p)}
        />
      )}
      {modal === 'status' && (
        <StatusModal
          count={count}
          onCancel={close}
          onConfirm={(s) => apply('set_status', s)}
        />
      )}
      {modal === 'project' && (
        <ProjectModal
          count={count}
          projects={projects}
          onCancel={close}
          onConfirm={(p) => apply('set_project', p)}
        />
      )}
      {modal === 'delete' && (
        <DeleteModal
          count={count}
          onCancel={close}
          onConfirm={() => apply('delete')}
        />
      )}
    </>
  )
}

function BarButton({
  children,
  icon,
  onClick,
  tone = 'default',
}: {
  children: React.ReactNode
  icon: React.ReactNode
  onClick: () => void
  tone?: 'default' | 'danger'
}) {
  return (
    <button
      onClick={onClick}
      className={clsx(
        'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors cursor-pointer',
        tone === 'danger'
          ? 'border border-red-200 bg-red-50 text-red-700 hover:bg-red-100'
          : 'border border-zinc-200 bg-zinc-50 text-zinc-700 hover:bg-zinc-100',
      )}
    >
      {icon}
      {children}
    </button>
  )
}

// ─── Modals ─────────────────────────────────────────────────────────────────

function ModalShell({ title, children, onClose }: { title: string; children: React.ReactNode; onClose: () => void }) {
  // Esc closes the modal.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onClose])

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50"
      onClick={onClose}
      role="dialog"
      aria-modal="true"
      aria-label={title}
    >
      <div
        className="mx-4 w-full max-w-md rounded-lg bg-white dark:bg-zinc-100 p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-lg font-semibold text-zinc-900">{title}</h3>
        {children}
      </div>
    </div>
  )
}

function PriorityModal({
  count,
  onCancel,
  onConfirm,
}: {
  count: number
  onCancel: () => void
  onConfirm: (priority: number) => Promise<void>
}) {
  const [priority, setPriority] = useState('0')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const parsed = Number.parseInt(priority, 10)
  const valid = Number.isFinite(parsed)

  const submit = async () => {
    if (!valid) return
    setError(null)
    setSubmitting(true)
    try {
      await onConfirm(parsed)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update tasks')
      setSubmitting(false)
    }
  }

  return (
    <ModalShell title="Change priority" onClose={onCancel}>
      <p className="mt-2 text-sm text-zinc-600">
        Set priority for {count} task{count === 1 ? '' : 's'}. Higher numbers run first.
      </p>
      <input
        type="number"
        value={priority}
        onChange={(e) => setPriority(e.target.value)}
        autoFocus
        onKeyDown={(e) => {
          if (e.key === 'Enter' && valid && !submitting) submit()
        }}
        className="mt-3 w-full rounded-md border border-zinc-200 bg-white dark:bg-zinc-50 px-3 py-2 text-sm text-zinc-900 focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400"
      />
      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      <ModalButtons
        onCancel={onCancel}
        onConfirm={submit}
        confirmLabel={submitting ? 'Applying…' : 'Apply'}
        confirmDisabled={!valid || submitting}
      />
    </ModalShell>
  )
}

function StatusModal({
  count,
  onCancel,
  onConfirm,
}: {
  count: number
  onCancel: () => void
  onConfirm: (status: TaskStatus) => Promise<void>
}) {
  const [status, setStatus] = useState<TaskStatus>('queued')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    setError(null)
    setSubmitting(true)
    try {
      await onConfirm(status)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update tasks')
      setSubmitting(false)
    }
  }

  return (
    <ModalShell title="Change status" onClose={onCancel}>
      <p className="mt-2 text-sm text-zinc-600">
        Set status for {count} task{count === 1 ? '' : 's'}.
        Running tasks will be skipped automatically.
      </p>
      <div className="mt-3 flex flex-col gap-2">
        {SETTABLE_STATUSES.map((s) => (
          <label
            key={s.value}
            className={clsx(
              'flex items-center gap-3 rounded-md border px-3 py-2 text-sm cursor-pointer transition-colors',
              status === s.value
                ? 'border-blue-400 bg-blue-50 text-blue-900'
                : 'border-zinc-200 bg-white dark:bg-zinc-50 text-zinc-700 hover:bg-zinc-50',
            )}
          >
            <input
              type="radio"
              name="bulk-status"
              value={s.value}
              checked={status === s.value}
              onChange={() => setStatus(s.value)}
              className="h-3.5 w-3.5"
            />
            {s.label}
          </label>
        ))}
      </div>
      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      <ModalButtons
        onCancel={onCancel}
        onConfirm={submit}
        confirmLabel={submitting ? 'Applying…' : 'Apply'}
        confirmDisabled={submitting}
      />
    </ModalShell>
  )
}

function ProjectModal({
  count,
  projects,
  onCancel,
  onConfirm,
}: {
  count: number
  projects: Project[]
  onCancel: () => void
  onConfirm: (projectId: string) => Promise<void>
}) {
  const activeProjects = useMemo(
    () => projects.filter((p) => p.active).sort((a, b) => a.name.localeCompare(b.name)),
    [projects],
  )
  const [projectId, setProjectId] = useState(activeProjects[0]?.id ?? '')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const submit = async () => {
    if (!projectId) return
    setError(null)
    setSubmitting(true)
    try {
      await onConfirm(projectId)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to update tasks')
      setSubmitting(false)
    }
  }

  return (
    <ModalShell title="Move to project" onClose={onCancel}>
      <p className="mt-2 text-sm text-zinc-600">
        Move {count} task{count === 1 ? '' : 's'} to a different project. Running tasks will be skipped.
      </p>
      <select
        value={projectId}
        onChange={(e) => setProjectId(e.target.value)}
        autoFocus
        className="mt-3 w-full rounded-md border border-zinc-200 bg-white dark:bg-zinc-50 px-3 py-2 text-sm text-zinc-900 focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400"
      >
        {activeProjects.length === 0 && <option value="">No active projects</option>}
        {activeProjects.map((p) => (
          <option key={p.id} value={p.id}>
            {p.name}
          </option>
        ))}
      </select>
      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      <ModalButtons
        onCancel={onCancel}
        onConfirm={submit}
        confirmLabel={submitting ? 'Applying…' : 'Apply'}
        confirmDisabled={!projectId || submitting}
      />
    </ModalShell>
  )
}

function DeleteModal({
  count,
  onCancel,
  onConfirm,
}: {
  count: number
  onCancel: () => void
  onConfirm: () => Promise<void>
}) {
  const [confirmation, setConfirmation] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const expected = String(count)
  const matches = confirmation.trim() === expected

  const submit = async () => {
    if (!matches) return
    setError(null)
    setSubmitting(true)
    try {
      await onConfirm()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete tasks')
      setSubmitting(false)
    }
  }

  return (
    <ModalShell title="Delete tasks" onClose={onCancel}>
      <div className="mt-2 flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800">
        <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0" />
        <p>
          You are about to delete {count} task{count === 1 ? '' : 's'}.
          Running tasks will be skipped. Type the count <code className="font-mono font-semibold">{expected}</code> to confirm.
        </p>
      </div>
      <input
        type="text"
        value={confirmation}
        onChange={(e) => setConfirmation(e.target.value)}
        autoFocus
        onKeyDown={(e) => {
          if (e.key === 'Enter' && matches && !submitting) submit()
        }}
        placeholder={`Type ${expected} to confirm`}
        className="mt-3 w-full rounded-md border border-zinc-200 bg-white dark:bg-zinc-50 px-3 py-2 text-sm text-zinc-900 focus:border-red-400 focus:outline-none focus:ring-1 focus:ring-red-400"
      />
      {error && <p className="mt-2 text-sm text-red-600">{error}</p>}
      <ModalButtons
        onCancel={onCancel}
        onConfirm={submit}
        confirmLabel={submitting ? 'Deleting…' : 'Delete'}
        confirmDisabled={!matches || submitting}
        confirmTone="danger"
      />
    </ModalShell>
  )
}

function ModalButtons({
  onCancel,
  onConfirm,
  confirmLabel,
  confirmDisabled,
  confirmTone = 'primary',
}: {
  onCancel: () => void
  onConfirm: () => void
  confirmLabel: string
  confirmDisabled: boolean
  confirmTone?: 'primary' | 'danger'
}) {
  return (
    <div className="mt-4 flex justify-end gap-3">
      <button
        onClick={onCancel}
        className="rounded-md px-3 py-1.5 text-sm font-medium text-zinc-600 hover:bg-zinc-100 cursor-pointer"
      >
        Cancel
      </button>
      <button
        onClick={onConfirm}
        disabled={confirmDisabled}
        className={clsx(
          'rounded-md px-3 py-1.5 text-sm font-medium text-white cursor-pointer disabled:cursor-not-allowed disabled:opacity-50',
          confirmTone === 'danger'
            ? 'bg-red-600 hover:bg-red-700'
            : 'bg-blue-600 hover:bg-blue-700',
        )}
      >
        {confirmLabel}
      </button>
    </div>
  )
}
