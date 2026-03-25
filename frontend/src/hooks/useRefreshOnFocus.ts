import { useEffect, useRef } from 'react'

/**
 * Calls `onRefresh` when the browser tab becomes visible again,
 * but only if at least `minStaleMs` have elapsed since the last refresh.
 */
export function useRefreshOnFocus(onRefresh: () => void, minStaleMs = 2000) {
  const lastRefresh = useRef(Date.now())

  useEffect(() => {
    lastRefresh.current = Date.now()
  }, [onRefresh])

  useEffect(() => {
    const handler = () => {
      if (document.visibilityState === 'visible' && Date.now() - lastRefresh.current > minStaleMs) {
        lastRefresh.current = Date.now()
        onRefresh()
      }
    }
    document.addEventListener('visibilitychange', handler)
    return () => document.removeEventListener('visibilitychange', handler)
  }, [onRefresh, minStaleMs])
}
