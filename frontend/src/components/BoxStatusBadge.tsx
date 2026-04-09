import { Server, Loader2 } from 'lucide-react'
import { useBoxStatus } from '../hooks/useBoxStatus'

/**
 * BoxStatusBadge is a compact, always-visible indicator of the remote Box host's
 * connectivity. Clicking it sends a Wake-on-LAN when Box is offline; clicking
 * while online is a no-op. Intended for sidebars/footers.
 */
export default function BoxStatusBadge() {
  const { state, waking, wake } = useBoxStatus()

  const dotColor = (() => {
    switch (state) {
      case 'online':
        return 'bg-emerald-500'
      case 'booting':
        return 'bg-amber-400 animate-pulse'
      case 'offline':
        return 'bg-zinc-400'
      default:
        return 'bg-zinc-300'
    }
  })()

  const title = (() => {
    switch (state) {
      case 'online':
        return 'Box is online'
      case 'booting':
        return 'Box is booting…'
      case 'offline':
        return 'Box is offline — click to wake'
      default:
        return 'Box status unknown'
    }
  })()

  const clickable = state === 'offline'
  const handleClick = () => {
    if (clickable) void wake()
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      disabled={!clickable || waking}
      title={title}
      className={`flex items-center gap-1 px-1.5 py-1 text-[11px] rounded-md transition-colors
                  ${clickable ? 'hover:bg-zinc-200 text-zinc-600 cursor-pointer' : 'text-zinc-500'}
                  ${!clickable && 'cursor-default'}`}
    >
      {state === 'booting' ? (
        <Loader2 className="w-3 h-3 animate-spin text-amber-500" />
      ) : (
        <Server className="w-3 h-3" />
      )}
      <span className={`w-1.5 h-1.5 rounded-full ${dotColor}`} />
      <span>Box</span>
    </button>
  )
}
