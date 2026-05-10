import { useEffect, useState, useCallback } from 'react'
import { fetchTasks } from '../api/client'
import type { Task } from '../types'

interface UseTasksFilters {
  status?: string
  project_id?: string
  tag_ids?: number[]
  q?: string
  limit?: number
  offset?: number
}

interface UseTasksResult {
  tasks: Task[]
  total: number
  loading: boolean
  error: string | null
  refetch: () => Promise<void>
}

export function useTasks(filters: UseTasksFilters = {}): UseTasksResult {
  const [tasks, setTasks] = useState<Task[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  // Stable string key for the tag list — useCallback dep array can't compare arrays.
  const tagKey = (filters.tag_ids ?? []).join(',')

  const refetch = useCallback(async () => {
    try {
      setError(null)
      const params: {
        status?: string
        project_id?: string
        tag_ids?: number[]
        q?: string
        limit?: number
        offset?: number
      } = {}
      if (filters.status) params.status = filters.status
      if (filters.project_id) params.project_id = filters.project_id
      if (filters.tag_ids && filters.tag_ids.length > 0) params.tag_ids = filters.tag_ids
      if (filters.q) params.q = filters.q
      if (filters.limit != null) params.limit = filters.limit
      if (filters.offset != null) params.offset = filters.offset
      const result = await fetchTasks(params)
      setTasks(result.data)
      setTotal(result.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tasks')
    } finally {
      setLoading(false)
    }
    // tagKey covers filters.tag_ids identity changes.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filters.status, filters.project_id, tagKey, filters.q, filters.limit, filters.offset])

  useEffect(() => {
    setLoading(true)
    refetch()
  }, [refetch])

  return { tasks, total, loading, error, refetch }
}
