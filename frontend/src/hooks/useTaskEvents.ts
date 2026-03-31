import { useEffect, useRef } from 'react'

/**
 * Connects to the task status SSE endpoint and calls `onEvent` whenever
 * a task status change is received. Automatically reconnects on errors.
 */
export function useTaskEvents(onEvent: () => void) {
  const onEventRef = useRef(onEvent)
  onEventRef.current = onEvent

  useEffect(() => {
    let es: EventSource | null = null
    let reconnectTimer: ReturnType<typeof setTimeout> | null = null

    function connect() {
      es = new EventSource('/api/v1/tasks/events')

      es.addEventListener('task_status', () => {
        onEventRef.current()
      })

      es.onerror = () => {
        es?.close()
        // Reconnect after 5 seconds on error
        reconnectTimer = setTimeout(connect, 5000)
      }
    }

    connect()

    return () => {
      es?.close()
      if (reconnectTimer) clearTimeout(reconnectTimer)
    }
  }, [])
}
