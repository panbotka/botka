import { useState, useEffect, useCallback, useRef } from 'react'
import { fetchProjectCommands, runProjectCommand, killProjectCommand } from '../api/client'
import type { Project, RunningCommandStatus } from '../types'

interface UseProjectCommandsResult {
  hasDevCommand: boolean
  hasDeployCommand: boolean
  runningDev: RunningCommandStatus | undefined
  runningDeploy: RunningCommandStatus | undefined
  toast: string | null
  confirmDeploy: boolean
  setConfirmDeploy: (v: boolean) => void
  handleRun: (type: 'dev' | 'deploy') => void
  handleKill: (pid: number) => void
  confirmAndDeploy: () => void
}

export function useProjectCommands(project: Project | undefined): UseProjectCommandsResult {
  const [commands, setCommands] = useState<RunningCommandStatus[]>([])
  const [toast, setToast] = useState<string | null>(null)
  const [confirmDeploy, setConfirmDeploy] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const hasDevCommand = !!project?.dev_command
  const hasDeployCommand = !!project?.deploy_command
  const projectId = project?.id

  const loadCommands = useCallback(async () => {
    if (!projectId) return
    try {
      const cmds = await fetchProjectCommands(projectId)
      setCommands(cmds)
    } catch {
      // ignore polling errors
    }
  }, [projectId])

  useEffect(() => {
    if (!hasDevCommand && !hasDeployCommand) {
      setCommands([])
      return
    }
    loadCommands()
    intervalRef.current = setInterval(loadCommands, 5000)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [loadCommands, hasDevCommand, hasDeployCommand])

  const showToast = useCallback((msg: string, duration = 3000) => {
    setToast(msg)
    setTimeout(() => setToast(null), duration)
  }, [])

  const handleRun = useCallback(async (type: 'dev' | 'deploy') => {
    if (!projectId) return
    try {
      const result = await runProjectCommand(projectId, type)
      showToast(`${type === 'dev' ? 'Dev' : 'Deploy'} started (PID ${result.pid})`)
      loadCommands()
    } catch (err) {
      showToast(err instanceof Error ? err.message : 'Failed to start command', 4000)
    }
  }, [projectId, showToast, loadCommands])

  const handleKill = useCallback(async (pid: number) => {
    if (!projectId) return
    try {
      await killProjectCommand(projectId, pid)
      showToast('Process stopped')
      loadCommands()
    } catch {
      showToast('Failed to stop process', 4000)
    }
  }, [projectId, showToast, loadCommands])

  const confirmAndDeploy = useCallback(() => {
    setConfirmDeploy(false)
    handleRun('deploy')
  }, [handleRun])

  const runningDev = commands.find((c) => c.command_type === 'dev')
  const runningDeploy = commands.find((c) => c.command_type === 'deploy')

  return {
    hasDevCommand,
    hasDeployCommand,
    runningDev,
    runningDeploy,
    toast,
    confirmDeploy,
    setConfirmDeploy,
    handleRun,
    handleKill,
    confirmAndDeploy,
  }
}
