import { useCallback, useEffect, useRef, useState } from 'react'
import { api } from '../api/client'
import type { BoxStatus } from '../types'

/** A simplified Box connectivity state for UI badges. */
export type BoxConnectivity = 'online' | 'offline' | 'booting' | 'unknown'

export interface UseBoxStatusResult {
  /** Last known status payload, or null while loading. */
  status: BoxStatus | null
  /** High-level state used by badges. "booting" is set after a wake until online. */
  state: BoxConnectivity
  /** True while a wake-on-LAN / wait cycle is in progress. */
  waking: boolean
  /** Force an immediate refresh. Safe to call while a previous call is pending. */
  refresh: () => Promise<void>
  /** Kick off wake-on-LAN and poll status until online or timeout. */
  wake: () => Promise<void>
}

const DEFAULT_POLL_INTERVAL_MS = 30_000
const WAKING_POLL_INTERVAL_MS = 5_000
const WAKE_TIMEOUT_MS = 90_000

/**
 * useBoxStatus polls the Box status endpoint and exposes simple state for
 * header/sidebar indicators. It supports an optional wake flow that triggers
 * wake-on-LAN and fast-polls until Box comes online or a timeout elapses.
 */
export function useBoxStatus(pollIntervalMs: number = DEFAULT_POLL_INTERVAL_MS): UseBoxStatusResult {
  const [status, setStatus] = useState<BoxStatus | null>(null)
  const [waking, setWaking] = useState(false)
  const wakingRef = useRef(false)
  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const s = await api.fetchBoxStatus()
      setStatus(s)
      if (wakingRef.current && s.online) {
        wakingRef.current = false
        setWaking(false)
      }
    } catch {
      // ignore — a failed probe leaves the previous state in place
    } finally {
      inFlight.current = false
    }
  }, [])

  // Poll on mount and on interval. Interval shortens while waking.
  useEffect(() => {
    refresh()
    let timer: ReturnType<typeof setInterval> | null = null
    const startTimer = (ms: number) => {
      if (timer) clearInterval(timer)
      timer = setInterval(refresh, ms)
    }
    startTimer(waking ? WAKING_POLL_INTERVAL_MS : pollIntervalMs)
    return () => {
      if (timer) clearInterval(timer)
    }
  }, [refresh, waking, pollIntervalMs])

  const wake = useCallback(async () => {
    if (wakingRef.current) return
    wakingRef.current = true
    setWaking(true)
    try {
      await api.wakeBox()
    } catch {
      // Even on error, we still poll; the box may wake from a retry or earlier WoL.
    }
    // Safety: stop waking after a hard timeout so the UI doesn't hang.
    setTimeout(() => {
      if (wakingRef.current) {
        wakingRef.current = false
        setWaking(false)
      }
    }, WAKE_TIMEOUT_MS)
  }, [])

  const state: BoxConnectivity = (() => {
    if (status == null) return 'unknown'
    if (waking && !status.online) return 'booting'
    return status.online ? 'online' : 'offline'
  })()

  return { status, state, waking, refresh, wake }
}
