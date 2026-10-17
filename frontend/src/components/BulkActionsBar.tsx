import { useState, useEffect } from 'react'
import { clsx } from 'clsx'
import {
  ArrowUpDown,
  Trash2,
  X,
  AlertTriangle,
  Ban,
  RefreshCw,
  Inbox,
} from 'lucide-react'

import type { BulkOperation, BulkPayload } from '../api/client'

type ActiveModal = 'priority' | 'delete' | null

interface BulkActionsBarProps {
  count: number
  /** Apply an operation; rejecting the promise leaves the modal open with the error. */
  onAction: (operation: BulkOperation, payload?: BulkPayload) => Promise<void>
  onClear: () => void
}

export function BulkActionsBar({ count, onAction, onClear }: BulkActionsBarProps) {
  const [modal, setModal] = useState<ActiveModal>(null)
  const [pending, setPending] = useState<BulkOperation | null>(null)

  // Close any open modal automatically if the selection drains.
  useEffect(() => {
    if (count === 0) {
      setModal(null)
      setPending(null)
    }
  }, [count])

  const close = () => setModal(null)

  // applyDirect runs operations that have no payload (cancel/requeue/set_pending).
  // The button shows a brief disabled/spinner state to swallow double-clicks.
  const applyDirect = async (operation: BulkOperation) => {
    if (pending) return
    setPending(operation)
    try {
      await onAction(operation)
    } finally {
      setPending(null)
    }
  }

  const applyWithPayload = async (operation: BulkOperation, payload: BulkPayload) => {
    await onAction(operation, payload)
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
          <BarButton
            icon={<Ban className="h-3.5 w-3.5" />}
            disabled={pending === 'cancel'}
            onClick={() => applyDirect('cancel')}
          >
            Cancel
          </BarButton>
          <BarButton
            icon={<RefreshCw className="h-3.5 w-3.5" />}
            disabled={pending === 'requeue'}
            onClick={() => applyDirect('requeue')}
          >
            Requeue
          </BarButton>
          <BarButton
            icon={<Inbox className="h-3.5 w-3.5" />}
            disabled={pending === 'set_pending'}
            onClick={() => applyDirect('set_pending')}
          >
            Move to pending
          </BarButton>
          <BarButton
            icon={<ArrowUpDown className="h-3.5 w-3.5" />}
            onClick={() => setModal('priority')}
          >
            Priority
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
          onConfirm={(p) => applyWithPayload('set_priority', { priority: p })}
        />
      )}
      {modal === 'delete' && (
        <DeleteModal
          count={count}
          onCancel={close}
          onConfirm={() => applyWithPayload('delete', undefined)}
        />
      )}
    </>
  )
}

function BarButton({
  children,
  icon,
  onClick,
  disabled,
  tone = 'default',
}: {
  children: React.ReactNode
  icon: React.ReactNode
  onClick: () => void
  disabled?: boolean
  tone?: 'default' | 'danger'
}) {
  return (
    <button
      onClick={onClick}
      disabled={disabled}
      className={clsx(
        'inline-flex items-center gap-1.5 rounded-full px-3 py-1 text-xs font-medium transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-50',
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
