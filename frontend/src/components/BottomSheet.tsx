import { useEffect, useRef, useState, type PointerEvent, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { clsx } from 'clsx'

/** Downward drag distance (px) on the handle past which releasing closes the sheet. */
const DRAG_CLOSE_THRESHOLD = 80

interface Props {
  open: boolean
  onClose: () => void
  /** Accessible name of the dialog. */
  ariaLabel: string
  children: ReactNode
  /** Optional fixed content between the drag handle and the scrolling body. */
  header?: ReactNode
  /** Close on Escape. Disable when the content handles Escape itself (nested modals). */
  closeOnEscape?: boolean
  /** data-testid of the backdrop, for tests that close the sheet by tapping it. */
  backdropTestId?: string
  /** Stacking order of the whole sheet (backdrop included). */
  zIndexClass?: string
}

/**
 * BottomSheet is the mobile modal surface: it slides up from the bottom, caps at
 * ~85% of the screen with a scrollable body, and closes on a backdrop tap, Escape
 * or a downward drag of the handle. It is portalled to `document.body` so neither
 * a transformed ancestor nor the chat header can clip or re-anchor it.
 */
export default function BottomSheet({
  open,
  onClose,
  ariaLabel,
  children,
  header,
  closeOnEscape = true,
  backdropTestId,
  zIndexClass = 'z-50',
}: Props) {
  const [dragY, setDragY] = useState(0)
  const dragStart = useRef<number | null>(null)

  useEffect(() => {
    if (!open || !closeOnEscape) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [open, closeOnEscape, onClose])

  useEffect(() => {
    if (!open) setDragY(0)
  }, [open])

  if (!open) return null

  const onPointerDown = (e: PointerEvent<HTMLDivElement>) => {
    dragStart.current = e.clientY
    e.currentTarget.setPointerCapture?.(e.pointerId)
  }
  const onPointerMove = (e: PointerEvent<HTMLDivElement>) => {
    if (dragStart.current === null) return
    setDragY(Math.max(0, e.clientY - dragStart.current))
  }
  const onPointerUp = () => {
    if (dragStart.current === null) return
    dragStart.current = null
    if (dragY > DRAG_CLOSE_THRESHOLD) onClose()
    setDragY(0)
  }

  return createPortal(
    <div className={clsx('fixed inset-0', zIndexClass)} role="presentation">
      <div
        data-testid={backdropTestId}
        className="absolute inset-0 bg-black/40 animate-fade-in"
        onClick={onClose}
      />
      <div
        role="dialog"
        aria-modal="true"
        aria-label={ariaLabel}
        className="absolute inset-x-0 bottom-0 flex max-h-[85lvh] flex-col rounded-t-2xl border-t border-zinc-200
                   bg-white shadow-xl animate-sheet-up dark:bg-zinc-100 dark:border-zinc-300"
        style={{
          paddingBottom: 'env(safe-area-inset-bottom, 0px)',
          transform: dragY > 0 ? `translateY(${dragY}px)` : undefined,
        }}
      >
        <div
          data-testid="bottom-sheet-handle"
          className="flex-shrink-0 cursor-grab touch-none pt-2 pb-1"
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
        >
          <div className="mx-auto h-1 w-10 rounded-full bg-zinc-300" aria-hidden="true" />
        </div>
        {header && <div className="flex-shrink-0">{header}</div>}
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain">{children}</div>
      </div>
    </div>,
    document.body,
  )
}
