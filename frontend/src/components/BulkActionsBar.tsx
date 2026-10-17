import { useState, useEffect, useRef } from 'react'
import { clsx } from 'clsx'
import {
  ArrowUpDown,
  Trash2,
  X,
  AlertTriangle,
  Ban,
  RefreshCw,
  Inbox,
  MoreHorizontal,
} from 'lucide-react'

import type { BulkOperation, BulkPayload } from '../api/client'
import { useIsMobile } from '../hooks/useIsMobile'
import { BOTTOM_NAV_HEIGHT } from '../utils/layout'

type ActiveModal = 'priority' | 'delete' | null

interface BulkActionsBarProps {
  count: number
  /** Apply an operation; rejecting the promise leaves the modal open with the error. */
  onAction: (operation: BulkOperation, payload?: BulkPayload) => Promise<void>
  onClear: () => void
}

export function BulkActionsBar({ count, onAction, onClear }: BulkActionsBarProps) {
  const isMobile = useIsMobile()
  const [modal, setModal] = useState<ActiveModal>(null)
  const [pending, setPending] = useState<BulkOperation | null>(null)
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  // Close any open modal automatically if the selection drains.
  useEffect(() => {
    if (count === 0) {
      setModal(null)
      setPending(null)
      setMenuOpen(false)
    }
  }, [count])

  // Dismiss the overflow menu on any tap outside it.
  useEffect(() => {
    if (!menuOpen) return
    const handler = (e: PointerEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', handler, true)
    return () => document.removeEventListener('pointerdown', handler, true)
  }, [menuOpen])

  const close = () => setModal(null)

  // applyDirect runs operations that have no payload (cancel/requeue/set_pending).
  // The button shows a brief disabled/spinner state to swallow double-clicks.
  const applyDirect = async (operation: BulkOperation) => {
    if (pending) return
    setMenuOpen(false)
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

  const openModal = (m: Exclude<ActiveModal, null>) => {
    setMenuOpen(false)
    setModal(m)
  }

  // Requeue (pending → queued) is by far the most common bulk action, so it
  // is the filled primary button and always comes first.
  const primary = (
    <button
      onClick={() => applyDirect('requeue')}
      disabled={pending === 'requeue'}
      className={clsx(
        'inline-flex items-center justify-center gap-1.5 whitespace-nowrap bg-blue-600 font-semibold text-white shadow-sm transition-colors hover:bg-blue-700 cursor-pointer disabled:cursor-not-allowed disabled:opacity-60',
        isMobile ? 'min-h-[44px] rounded-md px-4 text-sm' : 'rounded-full px-3 py-1 text-xs',
      )}
    >
      <RefreshCw className={isMobile ? 'h-4 w-4' : 'h-3.5 w-3.5'} />
      Do fronty
    </button>
  )

  const secondary: { key: string; label: string; icon: React.ReactNode; onClick: () => void; disabled?: boolean; tone?: 'danger' }[] = [
    {
      key: 'set_pending',
      label: 'Do Pending',
      icon: <Inbox className="h-3.5 w-3.5" />,
      onClick: () => applyDirect('set_pending'),
      disabled: pending === 'set_pending',
    },
    {
      key: 'priority',
      label: 'Priorita',
      icon: <ArrowUpDown className="h-3.5 w-3.5" />,
      onClick: () => openModal('priority'),
    },
    {
      key: 'cancel',
      label: 'Zrušit úlohy',
      icon: <Ban className="h-3.5 w-3.5" />,
      onClick: () => applyDirect('cancel'),
      disabled: pending === 'cancel',
    },
    {
      key: 'delete',
      label: 'Smazat',
      icon: <Trash2 className="h-3.5 w-3.5" />,
      onClick: () => openModal('delete'),
      tone: 'danger',
    },
  ]

  const clearButton = (
    <button
      onClick={onClear}
      className={clsx(
        'inline-flex items-center justify-center rounded-full text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700 cursor-pointer',
        isMobile ? 'h-11 w-11' : 'ml-1 p-1',
      )}
      aria-label="Zrušit výběr"
      title="Zrušit výběr"
    >
      <X className={isMobile ? 'h-5 w-5' : 'h-4 w-4'} />
    </button>
  )

  return (
    <>
      {isMobile ? (
        // Docked full-width directly above the in-flow BottomNav. The nav
        // itself absorbs the home-indicator inset, so offsetting by its full
        // height keeps the bar clear of both. It only appears after a user
        // interaction, long after iOS has resolved the viewport bottom.
        <div
          className="fixed inset-x-0 z-40 border-t border-zinc-200 bg-white dark:bg-zinc-100 shadow-[0_-4px_12px_rgba(0,0,0,0.08)]"
          style={{ bottom: BOTTOM_NAV_HEIGHT }}
          role="region"
          aria-label="Hromadné akce"
        >
          <div className="flex items-center gap-2 px-3 py-2">
            {clearButton}
            <span className="min-w-0 flex-1 truncate text-sm font-medium text-zinc-800">
              {count} vybráno
            </span>
            {primary}
            <div ref={menuRef} className="relative">
              <button
                onClick={() => setMenuOpen((o) => !o)}
                className="inline-flex h-11 w-11 items-center justify-center rounded-md border border-zinc-200 text-zinc-600 hover:bg-zinc-100 cursor-pointer"
                aria-label="Další akce"
                aria-haspopup="menu"
                aria-expanded={menuOpen}
              >
                <MoreHorizontal className="h-5 w-5" />
              </button>
              {menuOpen && (
                <div
                  role="menu"
                  className="absolute bottom-full right-0 mb-2 min-w-[180px] overflow-hidden rounded-md border border-zinc-200 bg-white dark:bg-zinc-100 shadow-lg"
                >
                  {secondary.map((a) => (
                    <button
                      key={a.key}
                      role="menuitem"
                      onClick={a.onClick}
                      disabled={a.disabled}
                      className={clsx(
                        'flex min-h-[44px] w-full items-center gap-2 px-4 text-left text-sm font-medium disabled:opacity-50 cursor-pointer',
                        a.tone === 'danger' ? 'text-red-700 hover:bg-red-50' : 'text-zinc-700 hover:bg-zinc-50',
                      )}
                    >
                      {a.icon}
                      {a.label}
                    </button>
                  ))}
                </div>
              )}
            </div>
          </div>
        </div>
      ) : (
        <div
          className="fixed inset-x-0 bottom-0 z-40 flex justify-center pointer-events-none px-3 pb-4"
          role="region"
          aria-label="Hromadné akce"
        >
          <div className="pointer-events-auto flex items-center gap-2 rounded-full border border-zinc-200 bg-white dark:bg-zinc-100 px-4 py-2 shadow-xl shadow-black/10">
            <span className="whitespace-nowrap text-sm font-medium text-zinc-800">
              {count} vybráno
            </span>
            <span className="h-4 w-px bg-zinc-200" aria-hidden="true" />
            {primary}
            {secondary.map((a) => (
              <BarButton key={a.key} icon={a.icon} onClick={a.onClick} disabled={a.disabled} tone={a.tone}>
                {a.label}
              </BarButton>
            ))}
            {clearButton}
          </div>
        </div>
      )}

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
        'inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-3 py-1 text-xs font-medium transition-colors cursor-pointer disabled:cursor-not-allowed disabled:opacity-50',
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
