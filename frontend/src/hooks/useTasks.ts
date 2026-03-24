import { useEffect, useState, useCallback } from 'react'
import { fetchTasks } from '../api/client'
import type { Task } from '../types'

interface UseTasksFilters {
  status?: string
  project_id?: string
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

  const refetch = useCallback(async () => {
    try {
      setError(null)
      const params: { status?: string; project_id?: string; limit: number } = {
        limit: 200,
      }
      if (filters.status) params.status = filters.status
      if (filters.project_id) params.project_id = filters.project_id
      const result = await fetchTasks(params)
      setTasks(result.data)
      setTotal(result.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch tasks')
    } finally {
      setLoading(false)
    }
  }, [filters.status, filters.project_id])

  useEffect(() => {
    setLoading(true)
    refetch()
  }, [refetch])

  return { tasks, total, loading, error, refetch }
}
