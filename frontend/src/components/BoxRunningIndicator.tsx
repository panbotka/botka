import { Server, Power, Loader2, AlertTriangle } from 'lucide-react'
import { useBoxStatus } from '../hooks/useBoxStatus'

/**
 * BoxRunningIndicator shows that the current thread targets the remote Box
 * host and reflects Box's connectivity state. When Box is offline it exposes
 * a quick "Wake" button so the user can bring the host up without leaving
 * the chat header.
 */
export default function BoxRunningIndicator() {
  const { state, waking, wake } = useBoxStatus()

  // Online or unknown (pre-poll) — show the minimal "Running on Box" pill.
  if (state === 'online' || state === 'unknown') {
    return (
      <span
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-md bg-sky-50 text-sky-600 border border-sky-200 flex-shrink-0"
        title="This thread runs on the remote Box host"
      >
        <Server className="w-3 h-3" />
        <span>Running on Box</span>
        {state === 'online' && (
          <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 ml-0.5" />
        )}
      </span>
    )
  }

  if (state === 'booting') {
    return (
      <span
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-md bg-amber-50 text-amber-600 border border-amber-200 flex-shrink-0"
        title="Box is booting — waiting for it to come online"
      >
        <Loader2 className="w-3 h-3 animate-spin" />
        <span>Booting Box…</span>
      </span>
    )
  }

  // state === 'offline' — show a wake button.
  return (
    <div className="flex items-center gap-1 flex-shrink-0">
      <span
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-md bg-amber-50 text-amber-700 border border-amber-200"
        title="Box is offline — the thread cannot run until it is awake"
      >
        <AlertTriangle className="w-3 h-3" />
        <span>Box offline</span>
      </span>
      <button
        onClick={wake}
        disabled={waking}
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-md bg-emerald-500 text-white hover:bg-emerald-400 active:bg-emerald-600 disabled:opacity-60 disabled:cursor-not-allowed cursor-pointer transition-colors"
        title="Send Wake-on-LAN to Box"
      >
        <Power className="w-3 h-3" />
        <span>Wake</span>
      </button>
    </div>
  )
}
