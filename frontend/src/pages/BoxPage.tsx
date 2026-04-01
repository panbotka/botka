import { useEffect, useState, useCallback, useRef } from 'react'
import {
  Server,
  Power,
  PowerOff,
  Play,
  Square,
  Cpu,
  ExternalLink,
  RefreshCw,
  Loader2,
} from 'lucide-react'
import { clsx } from 'clsx'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import {
  fetchBoxStatus,
  wakeBox,
  shutdownBox,
  startBoxService,
  stopBoxService,
} from '../api/client'
import type { BoxStatus, BoxServiceStatus } from '../types'

const NORMAL_POLL_MS = 10_000
const FAST_POLL_MS = 3_000
const FAST_POLL_DURATION_MS = 60_000

export default function BoxPage() {
  const [status, setStatus] = useState<BoxStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [showShutdownConfirm, setShowShutdownConfirm] = useState(false)
  const fastPollUntil = useRef(0)

  useDocumentTitle('Box')

  const refresh = useCallback(async () => {
    try {
      const data = await fetchBoxStatus()
      setStatus(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fetch status')
    } finally {
      setLoading(false)
    }
  }, [])

  // Polling with fast mode after actions
  useEffect(() => {
    refresh()
    const tick = () => {
      const now = Date.now()
      const interval = now < fastPollUntil.current ? FAST_POLL_MS : NORMAL_POLL_MS
      refresh()
      timer = window.setTimeout(tick, interval)
    }
    let timer = window.setTimeout(tick, NORMAL_POLL_MS)
    return () => clearTimeout(timer)
  }, [refresh])

  const startFastPolling = () => {
    fastPollUntil.current = Date.now() + FAST_POLL_DURATION_MS
  }

  const handleWake = async () => {
    setActionLoading('wake')
    try {
      await wakeBox()
      startFastPolling()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Wake failed')
    } finally {
      setActionLoading(null)
    }
  }

  const handleShutdown = async () => {
    setShowShutdownConfirm(false)
    setActionLoading('shutdown')
    try {
      await shutdownBox()
      startFastPolling()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Shutdown failed')
    } finally {
      setActionLoading(null)
    }
  }

  const handleServiceAction = async (name: string, action: 'start' | 'stop') => {
    setActionLoading(`${action}-${name}`)
    try {
      if (action === 'start') {
        await startBoxService(name)
      } else {
        await stopBoxService(name)
      }
      startFastPolling()
      // Immediate refresh
      await refresh()
    } catch (err) {
      setError(err instanceof Error ? err.message : `${action} ${name} failed`)
    } finally {
      setActionLoading(null)
    }
  }

  if (loading) {
    return (
      <div className="flex h-64 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  const online = status?.online ?? false

  return (
    <div className="mx-auto max-w-3xl space-y-6">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Server className="h-6 w-6 text-zinc-600" />
          <div>
            <h1 className="text-xl font-semibold text-zinc-900">Box Server</h1>
            <p className="text-sm text-zinc-500">{status?.host ?? '—'}</p>
          </div>
        </div>
        <button
          onClick={refresh}
          className="rounded-md p-2 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 transition-colors"
          title="Refresh"
        >
          <RefreshCw className="h-4 w-4" />
        </button>
      </div>

      {/* Error banner */}
      {error && (
        <div className="rounded-lg border border-red-200 bg-red-50 px-4 py-3 text-sm text-red-700">
          {error}
          <button onClick={() => setError(null)} className="ml-2 font-medium underline">
            Dismiss
          </button>
        </div>
      )}

      {/* Status + Power controls */}
      <div className="rounded-lg border border-zinc-200 bg-white dark:bg-zinc-100 p-5">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div
              className={clsx(
                'h-4 w-4 rounded-full',
                online ? 'bg-emerald-500 shadow-[0_0_8px_rgba(16,185,129,0.5)]' : 'bg-zinc-300',
              )}
            />
            <span className="text-lg font-medium text-zinc-900">
              {online ? 'Online' : 'Offline'}
            </span>
          </div>
          <div className="flex gap-2">
            <button
              onClick={handleWake}
              disabled={online || actionLoading !== null}
              className={clsx(
                'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                online || actionLoading !== null
                  ? 'cursor-not-allowed bg-zinc-100 text-zinc-400'
                  : 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100',
              )}
            >
              {actionLoading === 'wake' ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Power className="h-4 w-4" />
              )}
              Wake
            </button>
            <button
              onClick={() => setShowShutdownConfirm(true)}
              disabled={!online || actionLoading !== null}
              className={clsx(
                'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
                !online || actionLoading !== null
                  ? 'cursor-not-allowed bg-zinc-100 text-zinc-400'
                  : 'bg-red-50 text-red-700 hover:bg-red-100',
              )}
            >
              {actionLoading === 'shutdown' ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <PowerOff className="h-4 w-4" />
              )}
              Shutdown
            </button>
          </div>
        </div>
      </div>

      {/* Shutdown confirmation dialog */}
      {showShutdownConfirm && (
        <div className="rounded-lg border border-red-200 bg-red-50 p-4">
          <p className="text-sm font-medium text-red-800">
            Are you sure you want to shut down the box?
          </p>
          <div className="mt-3 flex gap-2">
            <button
              onClick={handleShutdown}
              className="rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-red-700 transition-colors"
            >
              Yes, shut down
            </button>
            <button
              onClick={() => setShowShutdownConfirm(false)}
              className="rounded-md bg-white dark:bg-zinc-200 px-3 py-1.5 text-sm font-medium text-zinc-700 border border-zinc-300 hover:bg-zinc-50 transition-colors"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Services */}
      <div>
        <h2 className="mb-3 text-sm font-semibold uppercase tracking-wider text-zinc-500">
          Services
        </h2>
        <div className="space-y-3">
          {(status?.services ?? []).map((svc) => (
            <ServiceCard
              key={svc.name}
              service={svc}
              boxOnline={online}
              actionLoading={actionLoading}
              onStart={() => handleServiceAction(svc.name, 'start')}
              onStop={() => handleServiceAction(svc.name, 'stop')}
            />
          ))}
        </div>
      </div>
    </div>
  )
}

function ServiceCard({
  service,
  boxOnline,
  actionLoading,
  onStart,
  onStop,
}: {
  service: BoxServiceStatus
  boxOnline: boolean
  actionLoading: string | null
  onStart: () => void
  onStop: () => void
}) {
  const isRunning = service.status === 'running'
  const isSystemd = service.type === 'systemd'
  const isStarting = actionLoading === `start-${service.name}`
  const isStopping = actionLoading === `stop-${service.name}`

  return (
    <div className="rounded-lg border border-zinc-200 bg-white dark:bg-zinc-100 p-4">
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <Cpu className="h-5 w-5 text-zinc-400" />
          <div>
            <div className="flex items-center gap-2">
              <span className="font-medium text-zinc-900">{service.name}</span>
              <span
                className={clsx(
                  'inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium',
                  isRunning
                    ? 'bg-emerald-100 text-emerald-700'
                    : 'bg-zinc-100 text-zinc-500',
                )}
              >
                {isRunning ? 'running' : 'stopped'}
              </span>
              <span className="text-xs text-zinc-400">:{service.port}</span>
            </div>
            <p className="text-sm text-zinc-500">{service.description}</p>
            {service.vram_usage_mb > 0 && (
              <p className="text-xs text-zinc-400">
                ~{service.vram_usage_mb >= 1000
                  ? `${(service.vram_usage_mb / 1000).toFixed(1)} GB`
                  : `${service.vram_usage_mb} MB`} VRAM
              </p>
            )}
          </div>
        </div>
        <div className="flex items-center gap-2">
          {isRunning && (
            <a
              href={service.url}
              target="_blank"
              rel="noopener noreferrer"
              className="rounded-md p-1.5 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 transition-colors"
              title="Open service"
            >
              <ExternalLink className="h-4 w-4" />
            </a>
          )}
          {isSystemd && boxOnline && (
            <>
              {isRunning ? (
                <button
                  onClick={onStop}
                  disabled={actionLoading !== null}
                  className={clsx(
                    'inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                    actionLoading !== null
                      ? 'cursor-not-allowed bg-zinc-100 text-zinc-400'
                      : 'bg-red-50 text-red-700 hover:bg-red-100',
                  )}
                >
                  {isStopping ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Square className="h-3.5 w-3.5" />
                  )}
                  Stop
                </button>
              ) : (
                <button
                  onClick={onStart}
                  disabled={actionLoading !== null}
                  className={clsx(
                    'inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-medium transition-colors',
                    actionLoading !== null
                      ? 'cursor-not-allowed bg-zinc-100 text-zinc-400'
                      : 'bg-emerald-50 text-emerald-700 hover:bg-emerald-100',
                  )}
                >
                  {isStarting ? (
                    <Loader2 className="h-3.5 w-3.5 animate-spin" />
                  ) : (
                    <Play className="h-3.5 w-3.5" />
                  )}
                  Start
                </button>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  )
}
