import { useState, useEffect, useCallback } from 'react'
import type { MCPServerWithStatus } from '../types'
import {
  fetchThreadMCPServers,
  setThreadMCPServers,
  fetchProjectMCPServers,
  setProjectMCPServers,
} from '../api/client'

interface UseMCPServersResult {
  servers: MCPServerWithStatus[]
  loading: boolean
  error: string | null
  toggle: (serverId: number) => Promise<void>
}

export function useMCPServers(
  scope: { type: 'thread'; id: number } | { type: 'project'; id: string },
): UseMCPServersResult {
  const [servers, setServers] = useState<MCPServerWithStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  const load = useCallback(async () => {
    try {
      setError(null)
      const data =
        scope.type === 'thread'
          ? await fetchThreadMCPServers(scope.id)
          : await fetchProjectMCPServers(scope.id)
      setServers(data)
    } catch {
      setError('Failed to load MCP servers')
    } finally {
      setLoading(false)
    }
  }, [scope.type, scope.id])

  useEffect(() => {
    load()
  }, [load])

  const toggle = useCallback(
    async (serverId: number) => {
      const server = servers.find((s) => s.id === serverId)
      if (!server || server.is_default) return

      const newEnabled = !server.enabled
      setServers((prev) =>
        prev.map((s) => (s.id === serverId ? { ...s, enabled: newEnabled } : s)),
      )

      const enabledIds = servers
        .filter((s) => {
          if (s.id === serverId) return newEnabled
          return s.enabled && !s.is_default
        })
        .map((s) => s.id)

      try {
        const updated =
          scope.type === 'thread'
            ? await setThreadMCPServers(scope.id, enabledIds)
            : await setProjectMCPServers(scope.id, enabledIds)
        setServers(updated)
      } catch {
        setServers((prev) =>
          prev.map((s) => (s.id === serverId ? { ...s, enabled: !newEnabled } : s)),
        )
        setError('Failed to update MCP servers')
      }
    },
    [servers, scope.type, scope.id],
  )

  return { servers, loading, error, toggle }
}
