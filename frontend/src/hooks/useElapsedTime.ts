import { useEffect, useState } from 'react'

/**
 * Returns a live-ticking human-readable elapsed time string since `since`.
 * Updates every second. Returns null if `since` is null.
 *
 * Format:
 *  - Under 60s: "Xs ago"
 *  - 1-59 min:  "Xm Ys ago"
 *  - 60+ min:   "Xh Ym ago"
 */
export function useElapsedTime(since: Date | null): string | null {
  const [, tick] = useState(0)

  useEffect(() => {
    if (!since) return
    const id = setInterval(() => tick((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [since])

  if (!since) return null

  const elapsed = Math.max(0, Math.floor((Date.now() - since.getTime()) / 1000))

  if (elapsed < 60) {
    return `${elapsed}s ago`
  }
  if (elapsed < 3600) {
    const m = Math.floor(elapsed / 60)
    const s = elapsed % 60
    return `${m}m ${s}s ago`
  }
  const h = Math.floor(elapsed / 3600)
  const m = Math.floor((elapsed % 3600) / 60)
  return `${h}h ${m}m ago`
}
