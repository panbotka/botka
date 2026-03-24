import { useEffect, useState, useCallback } from 'react'
import { fetchProjects, scanProjects } from '../api/client'
import type { Project } from '../types'

interface UseProjectsResult {
  projects: Project[]
  loading: boolean
  error: string | null
  refetch: () => Promise<void>
  scan: () => Promise<{ discovered: number; new: number; deactivated: number } | null>
  scanning: boolean
}

export function useProjects(): UseProjectsResult {
  const [projects, setProjects] = useState<Project[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [scanning, setScanning] = useState(false)

  const refetch = useCallback(async () => {
    try {
      setError(null)
      const data = await fetchProjects()
      setProjects(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch projects')
    } finally {
      setLoading(false)
    }
  }, [])

  const scan = useCallback(async () => {
    try {
      setScanning(true)
      const result = await scanProjects()
      await refetch()
      return result
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to scan projects')
      return null
    } finally {
      setScanning(false)
    }
  }, [refetch])

  useEffect(() => {
    setLoading(true)
    refetch()
  }, [refetch])

  return { projects, loading, error, refetch, scan, scanning }
}
