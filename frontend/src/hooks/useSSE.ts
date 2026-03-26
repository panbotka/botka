import { useState, useEffect, useRef, useCallback } from 'react'

const MAX_OUTPUT_BYTES = 100 * 1024 // 100KB
const TRUNCATION_NOTICE = '[earlier output truncated]\n'
const RECONNECT_DELAY = 3000
const MAX_RECONNECT_ATTEMPTS = 10

export function useSSE(taskId: string | null) {
  const [output, setOutput] = useState('')
  const [connected, setConnected] = useState(false)
  const [done, setDone] = useState(false)
  const esRef = useRef<EventSource | null>(null)
  const outputRef = useRef('')
  const doneRef = useRef(false)
  const attemptsRef = useRef(0)

  const appendOutput = useCallback((text: string) => {
    outputRef.current += text
    if (outputRef.current.length > MAX_OUTPUT_BYTES) {
      outputRef.current =
        TRUNCATION_NOTICE + outputRef.current.slice(outputRef.current.length - MAX_OUTPUT_BYTES)
    }
    setOutput(outputRef.current)
  }, [])

  useEffect(() => {
    if (!taskId) return

    let reconnectTimer: ReturnType<typeof setTimeout> | null = null
    let stopped = false
    doneRef.current = false
    attemptsRef.current = 0

    function connect() {
      if (stopped || doneRef.current) return

      const es = new EventSource(`/api/v1/tasks/${taskId}/output`)
      esRef.current = es

      es.onopen = () => {
        setConnected(true)
        attemptsRef.current = 0
      }

      es.onmessage = (event) => {
        try {
          const text = atob(event.data)
          appendOutput(text)
        } catch {
          // non-base64 data, append as-is
          appendOutput(event.data)
        }
      }

      es.addEventListener('done', () => {
        doneRef.current = true
        setDone(true)
        setConnected(false)
        es.close()
        esRef.current = null
      })

      es.onerror = () => {
        setConnected(false)
        es.close()
        esRef.current = null
        if (!stopped && !doneRef.current && attemptsRef.current < MAX_RECONNECT_ATTEMPTS) {
          attemptsRef.current++
          reconnectTimer = setTimeout(connect, RECONNECT_DELAY)
        }
      }
    }

    // Reset state for new task
    outputRef.current = ''
    setOutput('')
    setDone(false)
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
  }, [taskId, appendOutput])

  return { output, connected, done }
}
