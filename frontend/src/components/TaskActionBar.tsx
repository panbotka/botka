import { useEffect, useRef, useState } from 'react'
import { clsx } from 'clsx'
import {
  Ban,
  CheckCircle2,
  MoreHorizontal,
  Pencil,
  Play,
  RotateCcw,
  StopCircle,
  Trash2,
  Undo2,
  Zap,
} from 'lucide-react'
import type { TaskStatus } from '../types'

type Tone = 'blue' | 'amber' | 'red' | 'emerald' | 'neutral'

export interface TaskAction {
  key: string
  label: string
  icon: typeof Play
  onClick: () => void
  tone: Tone
  title?: string
}

export interface TaskActionHandlers {
  onForceRun: () => void
  onQueue: () => void
  onCancel: () => void
  onRetry: () => void
  onMarkDone: () => void
  onRestore: () => void
  onDelete: () => void
  onEdit: () => void
  onKill: () => void
}

// taskActions lists the actions available for a status, ordered by importance:
// the status's primary action first, then secondary actions, destructive last.
// Which actions exist per status is unchanged from the old bottom button row.
export function taskActions(status: TaskStatus, h: TaskActionHandlers): TaskAction[] {
  const edit: TaskAction = { key: 'edit', label: 'Edit', icon: Pencil, onClick: h.onEdit, tone: 'neutral' }
  const del: TaskAction = { key: 'delete', label: 'Delete', icon: Trash2, onClick: h.onDelete, tone: 'red' }
  const retry: TaskAction = { key: 'retry', label: 'Retry', icon: RotateCcw, onClick: h.onRetry, tone: 'blue' }

  switch (status) {
    case 'pending':
      return [
        { key: 'queue', label: 'Queue', icon: Play, onClick: h.onQueue, tone: 'blue' },
        edit,
        { key: 'cancel', label: 'Cancel', icon: Ban, onClick: h.onCancel, tone: 'red' },
        del,
      ]
    case 'queued':
      return [
        {
          key: 'force-run',
          label: 'Spustit teď',
          icon: Zap,
          onClick: h.onForceRun,
          tone: 'amber',
          title: 'Obejde rate-limit bránu a spustí task hned',
        },
        edit,
        del,
      ]
    case 'running':
      return [{ key: 'kill', label: 'Kill Task', icon: StopCircle, onClick: h.onKill, tone: 'red' }, edit]
    case 'failed':
      return [retry, edit, del]
    case 'needs_review':
      return [
        retry,
        { key: 'mark-done', label: 'Mark Done', icon: CheckCircle2, onClick: h.onMarkDone, tone: 'emerald' },
        edit,
        del,
      ]
    case 'deleted':
      return [{ key: 'restore', label: 'Restore', icon: Undo2, onClick: h.onRestore, tone: 'blue' }, edit]
    default:
      // done, cancelled: nothing to drive forward, so Edit leads.
      return [edit, del]
  }
}

// The leading action of a status that has a real "next step" is filled; Edit
// leading (done/cancelled) stays an outline button.
const filled: Record<Tone, string> = {
  blue: 'border-blue-600 bg-blue-600 text-white hover:bg-blue-700',
  amber: 'border-amber-600 bg-amber-600 text-white hover:bg-amber-700',
  red: 'border-red-600 bg-red-600 text-white hover:bg-red-700',
  emerald: 'border-emerald-600 bg-emerald-600 text-white hover:bg-emerald-700',
  neutral: 'border-zinc-300 bg-white text-zinc-700 hover:bg-zinc-50',
}

const outline: Record<Tone, string> = {
  blue: 'border-blue-300 bg-white text-blue-700 hover:bg-blue-50',
  amber: 'border-amber-300 bg-white text-amber-700 hover:bg-amber-50',
  red: 'border-red-300 bg-white text-red-700 hover:bg-red-50',
  emerald: 'border-emerald-300 bg-white text-emerald-700 hover:bg-emerald-50',
  neutral: 'border-zinc-300 bg-white text-zinc-700 hover:bg-zinc-50',
}

// MOBILE_VISIBLE is how many actions show as buttons below the md breakpoint:
// the primary plus one secondary; the rest go into the "⋯" overflow menu.
const MOBILE_VISIBLE = 2

interface TaskActionBarProps extends TaskActionHandlers {
  status: TaskStatus
  disabled: boolean
}

/**
 * The single home of every task action on the detail page. Sticky at the top of
 * the scroll container so the controls stay reachable while reading a long
 * spec or live output. On narrow screens only the first two actions are
 * buttons; the rest live in an overflow menu. All buttons are ≥44px tall on
 * mobile.
 */
export default function TaskActionBar({ status, disabled, ...handlers }: TaskActionBarProps) {
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)
  const actions = taskActions(status, handlers)
  const overflow = actions.slice(MOBILE_VISIBLE)

  // Dismiss the overflow menu on any tap outside it.
  useEffect(() => {
    if (!menuOpen) return
    const handler = (e: PointerEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) setMenuOpen(false)
    }
    document.addEventListener('pointerdown', handler, true)
    return () => document.removeEventListener('pointerdown', handler, true)
  }, [menuOpen])

  // A status change (after an action) can shrink the list; never leave a stale menu open.
  useEffect(() => setMenuOpen(false), [status])

  return (
    <div
      role="toolbar"
      aria-label="Akce úlohy"
      data-testid="task-action-bar"
      className="sticky -top-6 z-20 -mx-6 border-b border-zinc-200 bg-white/95 px-6 py-2 backdrop-blur"
    >
      <div className="flex items-center gap-2">
        {actions.map((a, i) => {
          const Icon = a.icon
          return (
            <button
              key={a.key}
              onClick={a.onClick}
              disabled={disabled}
              title={a.title}
              className={clsx(
                'items-center justify-center gap-1.5 whitespace-nowrap rounded-md border px-3 text-sm font-medium cursor-pointer disabled:cursor-not-allowed disabled:opacity-50',
                'min-h-[44px] md:min-h-0 md:py-1.5',
                i === 0 ? filled[a.tone] : outline[a.tone],
                i === 0 && 'font-semibold',
                i < MOBILE_VISIBLE ? 'inline-flex' : 'hidden md:inline-flex',
                i === 0 && 'flex-1 md:flex-none',
              )}
            >
              <Icon className="h-4 w-4 md:h-3.5 md:w-3.5" />
              {a.label}
            </button>
          )
        })}
        {overflow.length > 0 && (
          <div ref={menuRef} className="relative ml-auto md:hidden">
            <button
              onClick={() => setMenuOpen((o) => !o)}
              className="inline-flex h-11 w-11 items-center justify-center rounded-md border border-zinc-300 bg-white text-zinc-600 hover:bg-zinc-100 cursor-pointer"
              aria-label="Další akce"
              aria-haspopup="menu"
              aria-expanded={menuOpen}
            >
              <MoreHorizontal className="h-5 w-5" />
            </button>
            {menuOpen && (
              <div
                role="menu"
                className="absolute right-0 top-full z-30 mt-2 min-w-[180px] overflow-hidden rounded-md border border-zinc-200 bg-white shadow-lg"
              >
                {overflow.map((a) => {
                  const Icon = a.icon
                  return (
                    <button
                      key={a.key}
                      role="menuitem"
                      onClick={() => {
                        setMenuOpen(false)
                        a.onClick()
                      }}
                      disabled={disabled}
                      className={clsx(
                        'flex min-h-[44px] w-full items-center gap-2 px-4 text-left text-sm font-medium cursor-pointer disabled:opacity-50',
                        a.tone === 'red' ? 'text-red-700 hover:bg-red-50' : 'text-zinc-700 hover:bg-zinc-50',
                      )}
                    >
                      <Icon className="h-4 w-4" />
                      {a.label}
                    </button>
                  )
                })}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
