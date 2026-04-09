import { useState, useEffect, useCallback } from 'react'
import { MessageSquare, Check, X } from 'lucide-react'
import type { SignalBridge, SignalGroup } from '../types'
import {
  getSignalGroups,
  getSignalBridge,
  setSignalBridge,
  removeSignalBridge,
  ApiError,
} from '../api/client'

interface Props {
  threadId: number
  onClose: () => void
  onChange?: () => void
}

type LoadState =
  | { kind: 'loading' }
  | { kind: 'ready'; groups: SignalGroup[]; bridge: SignalBridge | null }
  | { kind: 'unavailable'; message: string }
  | { kind: 'error'; message: string }

export default function SignalBridgeEditor({ threadId, onClose, onChange }: Props) {
  const [state, setState] = useState<LoadState>({ kind: 'loading' })
  const [selectedGroupId, setSelectedGroupId] = useState<string>('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const showError = (msg: string) => {
    setError(msg)
    setTimeout(() => setError(null), 3000)
  }

  const load = useCallback(async () => {
    setState({ kind: 'loading' })
    // Always try to fetch the bridge first so we can show connection status
    // even when the signal-cli daemon is down.
    let bridge: SignalBridge | null = null
    try {
      bridge = await getSignalBridge(threadId)
    } catch (err) {
      const msg = err instanceof ApiError ? err.message : 'Failed to load bridge'
      setState({ kind: 'error', message: msg })
      return
    }

    try {
      const groups = await getSignalGroups()
      setState({ kind: 'ready', groups, bridge })
      if (bridge) setSelectedGroupId(bridge.group_id)
      else if (groups.length > 0) setSelectedGroupId(groups[0]!.id)
    } catch (err) {
      if (err instanceof ApiError && err.status === 503) {
        setState({ kind: 'unavailable', message: 'Signal service unavailable' })
      } else {
        const msg = err instanceof ApiError ? err.message : 'Failed to load groups'
        setState({ kind: 'error', message: msg })
      }
    }
  }, [threadId])

  useEffect(() => { load() }, [load])

  const handleConnect = async () => {
    if (state.kind !== 'ready' || !selectedGroupId) return
    const group = state.groups.find(g => g.id === selectedGroupId)
    if (!group) return
    setSaving(true)
    try {
      const bridge = await setSignalBridge(threadId, group.id, group.name)
      setState({ kind: 'ready', groups: state.groups, bridge })
      onChange?.()
    } catch (err) {
      showError(err instanceof ApiError ? err.message : 'Failed to connect bridge')
    } finally {
      setSaving(false)
    }
  }

  const handleDisconnect = async () => {
    if (state.kind !== 'ready' || !state.bridge) return
    setSaving(true)
    try {
      await removeSignalBridge(threadId)
      setState({ kind: 'ready', groups: state.groups, bridge: null })
      onChange?.()
    } catch (err) {
      showError(err instanceof ApiError ? err.message : 'Failed to disconnect bridge')
    } finally {
      setSaving(false)
    }
  }

  const isConnected = state.kind === 'ready' && state.bridge !== null
  const currentGroupName = state.kind === 'ready' && state.bridge ? state.bridge.group_name : null

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
         onClick={onClose}>
      <div className="bg-white dark:bg-zinc-100 rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col"
           onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
          <div className="flex items-center gap-2">
            <MessageSquare className={`w-4 h-4 ${isConnected ? 'text-emerald-500' : 'text-zinc-500'}`} />
            <h2 className="text-sm font-semibold text-zinc-800">Signal Bridge</h2>
          </div>
          <button onClick={onClose}
                  className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer">
            Done
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-4 space-y-4">
          {/* Current status */}
          <div className="rounded-lg border border-zinc-200 px-3 py-2.5 bg-zinc-50">
            <div className="text-[11px] uppercase tracking-wide text-zinc-400 mb-1">Status</div>
            {state.kind === 'loading' ? (
              <div className="text-sm text-zinc-500">Loading...</div>
            ) : isConnected ? (
              <div className="flex items-center gap-2">
                <Check className="w-3.5 h-3.5 text-emerald-500 flex-shrink-0" />
                <span className="text-sm text-zinc-800 font-medium truncate">
                  {currentGroupName || 'Connected'}
                </span>
              </div>
            ) : (
              <div className="flex items-center gap-2">
                <X className="w-3.5 h-3.5 text-zinc-400 flex-shrink-0" />
                <span className="text-sm text-zinc-500">Not connected</span>
              </div>
            )}
          </div>

          {/* Group selector */}
          {state.kind === 'loading' && (
            <p className="text-xs text-zinc-400 py-2 text-center">Loading groups...</p>
          )}

          {state.kind === 'unavailable' && (
            <p className="text-xs text-zinc-500 py-4 text-center">{state.message}</p>
          )}

          {state.kind === 'error' && (
            <p className="text-xs text-red-500 py-4 text-center">{state.message}</p>
          )}

          {state.kind === 'ready' && state.groups.length === 0 && (
            <p className="text-xs text-zinc-400 py-4 text-center">No Signal groups found</p>
          )}

          {state.kind === 'ready' && state.groups.length > 0 && (
            <div>
              <label htmlFor="signal-group-select"
                     className="block text-[11px] uppercase tracking-wide text-zinc-400 mb-1.5">
                {isConnected ? 'Change group' : 'Select a group'}
              </label>
              <select
                id="signal-group-select"
                value={selectedGroupId}
                onChange={e => setSelectedGroupId(e.target.value)}
                disabled={saving}
                className="w-full text-sm bg-white border border-zinc-200
                           rounded-lg px-3 py-2 text-zinc-700
                           focus:border-zinc-400 focus:outline-none
                           disabled:opacity-50"
              >
                {state.groups.map(g => (
                  <option key={g.id} value={g.id}>
                    {g.name || '(unnamed group)'} — {g.member_count} members
                  </option>
                ))}
              </select>
            </div>
          )}
        </div>

        <div className="px-5 py-3 border-t border-zinc-100 space-y-2">
          {error && (
            <p className="text-xs text-red-500">{error}</p>
          )}
          <div className="flex items-center justify-end gap-2">
            {isConnected && (
              <button
                onClick={handleDisconnect}
                disabled={saving}
                className="text-xs px-3 py-1.5 rounded-lg border border-zinc-200
                           text-zinc-600 hover:bg-zinc-50 hover:text-red-600
                           hover:border-red-200 transition-colors cursor-pointer
                           disabled:opacity-50 disabled:cursor-not-allowed"
              >
                Disconnect
              </button>
            )}
            {state.kind === 'ready' && state.groups.length > 0 && (
              <button
                onClick={handleConnect}
                disabled={
                  saving ||
                  !selectedGroupId ||
                  (isConnected && state.bridge?.group_id === selectedGroupId)
                }
                className="text-xs px-3 py-1.5 rounded-lg bg-zinc-800 text-white
                           hover:bg-zinc-700 transition-colors cursor-pointer
                           disabled:opacity-50 disabled:cursor-not-allowed"
              >
                {saving ? 'Saving...' : isConnected ? 'Update' : 'Connect'}
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  )
}
