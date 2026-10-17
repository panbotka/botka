import { useCallback, useEffect, useRef, useState } from 'react'
import type { MouseEvent, TouchEvent } from 'react'

/** How long a finger must rest on the element before the long-press fires. */
export const LONG_PRESS_DELAY_MS = 450
/** Finger travel (px) past which the gesture counts as a scroll, not a press. */
export const LONG_PRESS_MOVE_TOLERANCE_PX = 10

export interface LongPressHandlers {
  onTouchStart: (e: TouchEvent) => void
  onTouchMove: (e: TouchEvent) => void
  onTouchEnd: (e: TouchEvent) => void
  onTouchCancel: () => void
  onContextMenu: (e: MouseEvent) => void
  onClickCapture: (e: MouseEvent) => void
}

/**
 * useLongPress turns a resting touch into a long-press. The timer is cancelled
 * when the finger lifts early (a short tap stays a normal tap, so links and
 * buttons inside keep working) or moves more than the tolerance (a scroll).
 * Once the press has fired, the trailing touchend and the click the browser
 * synthesizes from it are swallowed so a link under the finger doesn't open.
 *
 * `pressing` is true while a finger rests on the element (including after the
 * press fired), letting the caller suppress text selection only for the
 * duration of the press.
 */
export function useLongPress(
  onLongPress: () => void,
  enabled = true,
): { handlers: LongPressHandlers; pressing: boolean } {
  const [pressing, setPressing] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined)
  const start = useRef<{ x: number; y: number } | null>(null)
  const fired = useRef(false)
  const callback = useRef(onLongPress)
  callback.current = onLongPress

  const cancel = useCallback(() => {
    clearTimeout(timer.current)
    timer.current = undefined
    start.current = null
    setPressing(false)
  }, [])

  useEffect(() => () => clearTimeout(timer.current), [])

  const onTouchStart = useCallback(
    (e: TouchEvent) => {
      if (!enabled || e.touches.length !== 1) {
        cancel()
        return
      }
      const touch = e.touches[0]!
      fired.current = false
      start.current = { x: touch.clientX, y: touch.clientY }
      setPressing(true)
      clearTimeout(timer.current)
      timer.current = setTimeout(() => {
        timer.current = undefined
        fired.current = true
        // `pressing` stays true until the finger lifts: iOS would otherwise
        // start a text selection under the still-resting finger.
        navigator.vibrate?.(10)
        callback.current()
      }, LONG_PRESS_DELAY_MS)
    },
    [enabled, cancel],
  )

  const onTouchMove = useCallback(
    (e: TouchEvent) => {
      const origin = start.current
      const touch = e.touches[0]
      if (!origin || !touch) return
      const dx = touch.clientX - origin.x
      const dy = touch.clientY - origin.y
      if (Math.hypot(dx, dy) > LONG_PRESS_MOVE_TOLERANCE_PX) cancel()
    },
    [cancel],
  )

  const onTouchEnd = useCallback(
    (e: TouchEvent) => {
      // A cancelable touchend that is prevented stops iOS from synthesizing
      // the click that would otherwise follow the long-press.
      if (fired.current && e.cancelable) e.preventDefault()
      cancel()
    },
    [cancel],
  )

  // Android raises contextmenu on a long-press; the sheet replaces it.
  const onContextMenu = useCallback(
    (e: MouseEvent) => {
      if (enabled && (fired.current || start.current)) e.preventDefault()
    },
    [enabled],
  )

  const onClickCapture = useCallback((e: MouseEvent) => {
    if (!fired.current) return
    fired.current = false
    e.preventDefault()
    e.stopPropagation()
  }, [])

  return {
    handlers: { onTouchStart, onTouchMove, onTouchEnd, onTouchCancel: cancel, onContextMenu, onClickCapture },
    pressing,
  }
}
