import { useState, useEffect, useRef, useCallback } from 'react'
import { NDJSONParser, type TaskOutputEvent } from '../utils/parseNDJSON'

const MAX_EVENTS = 5000
const RECONNECT_DELAY = 3000
const MAX_RECONNECT_ATTEMPTS = 10
const MAX_ERROR_RETRIES = 5
const ERROR_RETRY_DELAY = 3000

export function useSSE(taskId: string | null) {
  const [events, setEvents] = useState<TaskOutputEvent[]>([])
  const [connected, setConnected] = useState(false)
  const [done, setDone] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const esRef = useRef<EventSource | null>(null)
  const parserRef = useRef<NDJSONParser>(new NDJSONParser())
  const doneRef = useRef(false)
  const errorRef = useRef(false)
  const attemptsRef = useRef(0)
  const errorRetryRef = useRef(0)
  const receivedDataRef = useRef(false)

  const appendChunk = useCallback((chunk: string) => {
    const parser = parserRef.current
    const newEvents = parser.append(chunk)
    if (newEvents.length > 0) {
      receivedDataRef.current = true
      setEvents(prev => {
        const combined = [...prev, ...newEvents]
        // Cap events to prevent unbounded growth
        if (combined.length > MAX_EVENTS) {
          return combined.slice(combined.length - MAX_EVENTS)
        }
        return combined
      })
    }
  }, [])

  useEffect(() => {
    if (!taskId) return

    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let stopped = false
    doneRef.current = false
    errorRef.current = false
    attemptsRef.current = 0
    errorRetryRef.current = 0
    receivedDataRef.current = false
    parserRef.current = new NDJSONParser()

    function connect() {
      if (stopped || doneRef.current || errorRef.current) return

      const es = new EventSource(`/api/v1/tasks/${taskId}/output`)
      esRef.current = es

      es.onopen = () => {
        setConnected(true)
        attemptsRef.current = 0
      }

      es.onmessage = (event) => {
        let text: string
        try {
          text = atob(event.data)
        } catch {
          text = event.data
        }
        appendChunk(text)
      }

      es.addEventListener('done', () => {
        // Flush any remaining partial line
        const parser = parserRef.current
        const remaining = parser.flush()
        if (remaining.length > 0) {
          setEvents(prev => [...prev, ...remaining])
        }
        doneRef.current = true
        setDone(true)
        setConnected(false)
        es.close()
        esRef.current = null
      })

      es.addEventListener('error', ((event: MessageEvent) => {
        // Server-sent error event (distinct from EventSource connection errors).
        // This happens when the task is running in DB but has no executor (orphaned).
        let message = 'Output not available'
        try {
          const data = JSON.parse(event.data)
          if (data.message) message = data.message
        } catch {
          // use default message
        }
        es.close()
        esRef.current = null
        setConnected(false)

        // Retry a few times — the buffer may appear shortly after.
        if (!stopped && !doneRef.current && errorRetryRef.current < MAX_ERROR_RETRIES) {
          errorRetryRef.current++
          reconnectTimer = setTimeout(connect, ERROR_RETRY_DELAY)
        } else {
          errorRef.current = true
          setError(message)
        }
      }) as EventListener)

      es.onerror = () => {
        setConnected(false)
        es.close()
        esRef.current = null
        if (!stopped && !doneRef.current && !errorRef.current && attemptsRef.current < MAX_RECONNECT_ATTEMPTS) {
          attemptsRef.current++
          reconnectTimer = setTimeout(connect, RECONNECT_DELAY)
        }
      }
    }

    // Reset state for new task
    setEvents([])
    setDone(false)
    setError(null)
    setConnected(false)

    connect()

    return () => {
      stopped = true
      if (reconnectTimer) clearTimeout(reconnectTimer)
      if (esRef.current) {
        esRef.current.close()
        esRef.current = null
      }
    }
  }, [taskId, appendChunk])

  return { events, connected, done, error }
}
