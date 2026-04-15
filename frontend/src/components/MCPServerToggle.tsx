import { Link } from 'react-router-dom'
import { Plug, Settings } from 'lucide-react'
import { useMCPServers } from '../hooks/useMCPServers'

interface Props {
  scope: { type: 'thread'; id: number } | { type: 'project'; id: string }
  onClose?: () => void
}

function TypeBadge({ type }: { type: string }) {
  const isSSE = type === 'sse'
  return (
    <span
      className={`inline-flex items-center rounded-full px-1.5 py-0.5 text-[10px] font-medium ${
        isSSE
          ? 'bg-purple-100 text-purple-700'
          : 'bg-blue-100 text-blue-700'
      }`}
    >
      {type}
    </span>
  )
}

export default function MCPServerToggle({ scope, onClose }: Props) {
  const { servers, loading, error, toggle } = useMCPServers(scope)

  const hasNonDefault = servers.some((s) => !s.is_default)

  const content = (
    <div className="space-y-1">
      {loading ? (
        <p className="text-xs text-zinc-400 py-4 text-center">Loading...</p>
      ) : error ? (
        <p className="text-xs text-red-500 py-2 text-center">{error}</p>
      ) : servers.length === 0 ? (
        <div className="py-4 text-center">
          <p className="text-xs text-zinc-400">No MCP servers configured</p>
          <Link
            to="/settings?tab=mcp-servers"
            className="inline-flex items-center gap-1 mt-2 text-xs text-zinc-500 hover:text-zinc-700 transition-colors"
          >
            <Settings className="w-3 h-3" />
            Configure in Settings
          </Link>
        </div>
      ) : (
        <>
          {servers.map((server) => (
            <div
              key={server.id}
              className="flex items-center gap-2.5 py-1.5 px-1 rounded-lg"
            >
              <button
                onClick={() => toggle(server.id)}
                disabled={server.is_default}
                className={`relative w-8 h-[18px] rounded-full transition-colors flex-shrink-0 ${
                  server.is_default
                    ? 'bg-emerald-400 cursor-not-allowed opacity-60'
                    : server.enabled
                      ? 'bg-emerald-500 cursor-pointer'
                      : 'bg-zinc-300 cursor-pointer'
                }`}
              >
                <span
                  className={`absolute top-0.5 left-0.5 w-3.5 h-3.5 rounded-full bg-white shadow transition-transform ${
                    server.enabled ? 'translate-x-3.5' : 'translate-x-0'
                  }`}
                />
              </button>
              <span className="text-sm text-zinc-700 truncate flex-1 min-w-0">
                {server.name}
              </span>
              <TypeBadge type={server.server_type} />
              {server.is_default && (
                <span className="text-[10px] text-amber-600 font-medium flex-shrink-0">
                  (default)
                </span>
              )}
            </div>
          ))}
          {hasNonDefault && (
            <p className="text-[10px] text-zinc-400 mt-2 px-1">
              Changes take effect on the next message
            </p>
          )}
        </>
      )}
    </div>
  )

  if (onClose) {
    return (
      <div
        className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
        onClick={onClose}
      >
        <div
          className="bg-white dark:bg-zinc-100 rounded-xl shadow-xl w-full max-w-sm mx-4 max-h-[80vh] flex flex-col"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
            <div className="flex items-center gap-2">
              <Plug className="w-4 h-4 text-zinc-500" />
              <h2 className="text-sm font-semibold text-zinc-800">
                MCP Servers
              </h2>
            </div>
            <button
              onClick={onClose}
              className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer"
            >
              Done
            </button>
          </div>
          <div className="flex-1 overflow-y-auto px-5 py-3">{content}</div>
        </div>
      </div>
    )
  }

  return content
}
