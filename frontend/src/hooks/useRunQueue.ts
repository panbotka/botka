import { useCallback, useEffect, useState } from 'react'
import { fetchRunnerStatus, fetchTasks } from '../api/client'
import type { RunnerStatus, Task } from '../types'

// The panel only shows the first few queued tasks, but it fetches a wider
// slice so the "čeká na projekt" badge and the position numbers stay right.
const QUEUE_FETCH_LIMIT = 50

interface UseRunQueueResult {
  running: Task[]
  queued: Task[]
  queuedTotal: number
  runner: RunnerStatus | null
  loaded: boolean
  refetch: () => Promise<void>
}

/**
 * Loads what the "Běží teď / Následuje" panel needs: running and queued tasks
 * (optionally scoped to one project) plus the runner status. It never polls on
 * its own — the caller wires `refetch` into its SSE / focus refresh.
 */
export function useRunQueue(projectId?: string): UseRunQueueResult {
  const [running, setRunning] = useState<Task[]>([])
  const [queued, setQueued] = useState<Task[]>([])
  const [queuedTotal, setQueuedTotal] = useState(0)
  const [runner, setRunner] = useState<RunnerStatus | null>(null)
  const [loaded, setLoaded] = useState(false)

  const refetch = useCallback(async () => {
    const [runningRes, queuedRes, runnerRes] = await Promise.allSettled([
      fetchTasks({ status: 'running', project_id: projectId, limit: QUEUE_FETCH_LIMIT }),
      fetchTasks({ status: 'queued', project_id: projectId, limit: QUEUE_FETCH_LIMIT }),
      fetchRunnerStatus(),
    ])
    // Keep the previous value of any request that failed.
    if (runningRes.status === 'fulfilled') setRunning(runningRes.value.data)
    if (queuedRes.status === 'fulfilled') {
      setQueued(queuedRes.value.data)
      setQueuedTotal(queuedRes.value.total)
    }
    if (runnerRes.status === 'fulfilled') setRunner(runnerRes.value)
    setLoaded(true)
  }, [projectId])

  useEffect(() => {
    refetch()
  }, [refetch])

  return { running, queued, queuedTotal, runner, loaded, refetch }
}
