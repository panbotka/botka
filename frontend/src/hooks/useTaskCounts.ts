import { useEffect, useState, useCallback } from 'react'
import { fetchTaskStats, fetchTasks } from '../api/client'

interface TaskCounts {
  all: number
  pending: number
  queued: number
  running: number
  done: number
  failed: number
  needs_review: number
  deleted: number
}

const zeroCounts: TaskCounts = {
  all: 0,
  pending: 0,
  queued: 0,
  running: 0,
  done: 0,
  failed: 0,
  needs_review: 0,
  deleted: 0,
}

export function useTaskCounts(): {
  counts: TaskCounts
  loading: boolean
  refetch: () => Promise<void>
} {
  const [counts, setCounts] = useState<TaskCounts>(zeroCounts)
  const [loading, setLoading] = useState(true)

  const refetch = useCallback(async () => {
    try {
      const [stats, deletedResult] = await Promise.all([
        fetchTaskStats(),
        fetchTasks({ status: 'deleted', limit: 1 }),
      ])
      const s = stats.by_status
      setCounts({
        all: stats.total,
        pending: s.pending ?? 0,
        queued: s.queued ?? 0,
        running: s.running ?? 0,
        done: s.done ?? 0,
        failed: s.failed ?? 0,
        needs_review: s.needs_review ?? 0,
        deleted: deletedResult.total,
      })
    } catch {
      // Keep previous counts on error
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    refetch()
  }, [refetch])

  return { counts, loading, refetch }
}
