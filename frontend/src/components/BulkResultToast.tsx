import { useEffect, useState } from 'react'
import { clsx } from 'clsx'
import { CheckCircle2, AlertTriangle, X } from 'lucide-react'

import type { BulkFailure } from '../api/client'

export interface BulkResultSummary {
  /** User-friendly label for the operation, e.g. "Cancel". */
  operation: string
  succeeded: number
  failed: BulkFailure[]
}

interface BulkResultToastProps {
  summary: BulkResultSummary
  onDismiss: () => void
}

// BulkResultToast surfaces the outcome of a bulk operation in a corner toast.
// Clicking the toast opens a modal listing per-task failures so the user can
// retry or investigate without losing the rest of the result.
export function BulkResultToast({ summary, onDismiss }: BulkResultToastProps) {
  const [showPanel, setShowPanel] = useState(false)

  // Auto-dismiss the toast after a few seconds — only when the panel is closed
  // so the user can still drill into failures without it disappearing.
  useEffect(() => {
    if (showPanel) return
    const id = setTimeout(onDismiss, 6000)
    return () => clearTimeout(id)
  }, [onDismiss, showPanel])

  const hasFailures = summary.failed.length > 0
  const tone = hasFailures ? 'warn' : 'ok'

  const open = () => {
    if (hasFailures) setShowPanel(true)
  }

  return (
    <>
      <div
        className="fixed right-4 top-4 z-40 max-w-sm"
        role="status"
        aria-live="polite"
      >
        <button
          type="button"
          onClick={open}
          disabled={!hasFailures}
          className={clsx(
            'flex w-full items-start gap-2 rounded-lg border px-3 py-2 text-left text-sm shadow-lg transition-colors',
            tone === 'ok'
              ? 'border-emerald-200 bg-emerald-50 text-emerald-900'
              : 'border-amber-200 bg-amber-50 text-amber-900 cursor-pointer hover:bg-amber-100',
            !hasFailures && 'cursor-default',
          )}
        >
          {tone === 'ok' ? (
            <CheckCircle2 className="mt-0.5 h-4 w-4 flex-shrink-0" />
          ) : (
            <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0" />
          )}
          <span className="flex-1">
            <span className="block font-medium">
              {summary.operation}: {summary.succeeded} succeeded
              {hasFailures && `, ${summary.failed.length} failed`}
            </span>
            {hasFailures && (
              <span className="block text-xs opacity-75">Click for details</span>
            )}
          </span>
          <span
            role="button"
            tabIndex={0}
            onClick={(e) => {
              e.stopPropagation()
              onDismiss()
            }}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                e.stopPropagation()
                onDismiss()
              }
            }}
            className="ml-1 cursor-pointer rounded p-0.5 text-current opacity-60 hover:opacity-100"
            aria-label="Dismiss"
          >
            <X className="h-3.5 w-3.5" />
          </span>
        </button>
      </div>
      {showPanel && (
        <FailurePanel summary={summary} onClose={() => setShowPanel(false)} />
      )}
    </>
  )
}

function FailurePanel({
  summary,
  onClose,
}: {
  summary: BulkResultSummary
  onClose: () => void
}) {
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
      aria-label="Bulk operation failures"
    >
      <div
        className="mx-4 w-full max-w-lg rounded-lg bg-white dark:bg-zinc-100 p-6 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <h3 className="text-lg font-semibold text-zinc-900">
          {summary.operation}: failures ({summary.failed.length})
        </h3>
        <p className="mt-1 text-sm text-zinc-600">
          {summary.succeeded} task{summary.succeeded === 1 ? '' : 's'} succeeded.
          The tasks below were skipped.
        </p>
        <div className="mt-3 max-h-80 overflow-auto rounded-md border border-zinc-200">
          <table className="w-full text-left text-xs">
            <thead className="bg-zinc-50 text-[11px] uppercase tracking-wide text-zinc-500">
              <tr>
                <th className="px-3 py-2 font-medium">Task ID</th>
                <th className="px-3 py-2 font-medium">Reason</th>
              </tr>
            </thead>
            <tbody>
              {summary.failed.map((f) => (
                <tr key={f.id} className="border-t border-zinc-100">
                  <td className="px-3 py-1.5 font-mono text-zinc-600">
                    {f.id.slice(0, 8)}…
                  </td>
                  <td className="px-3 py-1.5 text-zinc-700">{f.error}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="mt-4 flex justify-end">
          <button
            onClick={onClose}
            className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 cursor-pointer"
          >
            Close
          </button>
        </div>
      </div>
    </div>
  )
}
