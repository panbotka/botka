import { useCallback, useEffect, useRef, useState } from 'react'
import { updateServerSettings } from '../api/client'

/** Lowest worker count the Dashboard stepper allows (0 = disabled is Settings-only). */
export const MIN_WORKERS = 1
/** Upper bound enforced by the backend (`internal/handlers/settings.go`). */
export const MAX_WORKERS = 10
/** Rapid clicks within this window collapse into a single save. */
export const SAVE_DEBOUNCE_MS = 400

export interface UseWorkerCountResult {
  value: number
  saving: boolean
  error: string | null
  canDecrement: boolean
  canIncrement: boolean
  decrement: () => void
  increment: () => void
}

/**
 * Optimistic, debounced editor for the `max_workers` server setting — the same
 * setting the Settings page writes, which the running runner applies live.
 *
 * `serverValue` is the last value reported by the server; it is adopted
 * whenever no local edit is pending. A failed save rolls back to the last
 * confirmed value and exposes an error message.
 */
export function useWorkerCount(
  serverValue: number,
  onSaved?: (value: number) => void,
): UseWorkerCountResult {
  const [value, setValue] = useState(serverValue)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const valueRef = useRef(serverValue)
  const confirmedRef = useRef(serverValue)
  const dirtyRef = useRef(false)
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)
  const seqRef = useRef(0)
  const onSavedRef = useRef(onSaved)
  useEffect(() => { onSavedRef.current = onSaved }, [onSaved])

  useEffect(() => {
    if (dirtyRef.current) return
    confirmedRef.current = serverValue
    valueRef.current = serverValue
    setValue(serverValue)
  }, [serverValue])

  // Flush a pending edit on unmount so navigating away right after a click
  // does not silently drop the change.
  useEffect(() => () => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
      updateServerSettings({ max_workers: valueRef.current }).catch(() => {})
    }
  }, [])

  const save = useCallback(async (target: number) => {
    timerRef.current = null
    const seq = ++seqRef.current
    setSaving(true)
    // A newer click (pending timer) or a newer save makes this result stale.
    const superseded = () => seq !== seqRef.current || timerRef.current !== null
    try {
      const updated = await updateServerSettings({ max_workers: target })
      confirmedRef.current = updated.max_workers
      if (superseded()) return
      dirtyRef.current = false
      valueRef.current = updated.max_workers
      setValue(updated.max_workers)
      onSavedRef.current?.(updated.max_workers)
    } catch {
      if (superseded()) return
      dirtyRef.current = false
      valueRef.current = confirmedRef.current
      setValue(confirmedRef.current)
      setError('Failed to save worker count')
    } finally {
      if (seq === seqRef.current) setSaving(false)
    }
  }, [])

  const setTarget = useCallback((next: number) => {
    const clamped = Math.min(MAX_WORKERS, Math.max(MIN_WORKERS, next))
    if (clamped === valueRef.current) return
    valueRef.current = clamped
    dirtyRef.current = true
    setValue(clamped)
    setError(null)
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => { void save(clamped) }, SAVE_DEBOUNCE_MS)
  }, [save])

  const decrement = useCallback(() => setTarget(valueRef.current - 1), [setTarget])
  const increment = useCallback(() => setTarget(valueRef.current + 1), [setTarget])

  return {
    value,
    saving,
    error,
    canDecrement: value > MIN_WORKERS,
    canIncrement: value < MAX_WORKERS,
    decrement,
    increment,
  }
}
