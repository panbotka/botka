import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchBoxAutoOff, updateServerSettings } from '../api/client'
import type { BoxAutoOffStatus } from '../types'

/** How often the auto-off state is re-fetched. */
const POLL_INTERVAL_MS = 15_000
/** How often the countdown re-renders between fetches. */
const TICK_INTERVAL_MS = 1_000

export interface UseBoxAutoOffResult {
  status: BoxAutoOffStatus | null
  loading: boolean
  error: string | null
  /** Seconds until the earliest possible shutdown, or null when there is none. */
  countdown: number | null
  refresh: () => Promise<void>
  setEnabled: (on: boolean) => Promise<void>
}

/**
 * useBoxAutoOff polls the auto-off endpoint and derives a live countdown from
 * `earliest_shutdown_at`.
 *
 * The countdown is computed locally on a one-second tick rather than being
 * fetched: the server only revises its estimate once per evaluation, so polling
 * at that resolution would be pointless traffic for a clock the browser can run
 * itself.
 */
export function useBoxAutoOff(): UseBoxAutoOffResult {
  const [status, setStatus] = useState<BoxAutoOffStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [countdown, setCountdown] = useState<number | null>(null)
  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const data = await fetchBoxAutoOff()
      setStatus(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load auto-off status')
    } finally {
      inFlight.current = false
      setLoading(false)
    }
  }, [])

  const setEnabled = useCallback(
    async (on: boolean) => {
      await updateServerSettings({ box_auto_off: on })
      await refresh()
    },
    [refresh],
  )

  useEffect(() => {
    void refresh()
    const timer = setInterval(() => void refresh(), POLL_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [refresh])

  // Derive the countdown from the target timestamp on every tick rather than
  // decrementing a counter: a background tab throttles timers, and a
  // decremented counter would drift out of step with the real clock.
  useEffect(() => {
    const target = status?.earliest_shutdown_at
    if (!target) {
      setCountdown(null)
      return
    }
    const targetMs = new Date(target).getTime()
    const tick = () => setCountdown(Math.max(0, Math.round((targetMs - Date.now()) / 1000)))
    tick()
    const timer = setInterval(tick, TICK_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [status?.earliest_shutdown_at])

  return { status, loading, error, countdown, refresh, setEnabled }
}
