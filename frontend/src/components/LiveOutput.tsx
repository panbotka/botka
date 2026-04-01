import { useSSE } from '../hooks/useSSE'
import TaskOutputView from './TaskOutputView'

function LiveIndicator({ connected, done, error }: { connected: boolean; done: boolean; error: string | null }) {
  if (error) {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-red-500">
        <span className="h-2 w-2 rounded-full bg-red-400" />
        UNAVAILABLE
      </span>
    )
  }
  if (done) {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-zinc-400">
        <span className="h-2 w-2 rounded-full bg-zinc-400" />
        DONE
      </span>
    )
  }
  if (connected) {
    return (
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-600">
        <span className="h-2 w-2 animate-pulse rounded-full bg-emerald-500" />
        LIVE
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-amber-500">
      <span className="h-2 w-2 rounded-full bg-amber-400" />
      RECONNECTING
    </span>
  )
}

export function LiveOutputInline({ taskId, taskTitle }: { taskId: string; taskTitle: string }) {
  const { events, connected, done, error } = useSSE(taskId)

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200">
      <div className="flex items-center justify-between border-b border-zinc-200 bg-zinc-50 px-4 py-2">
        <span className="truncate text-sm font-medium text-zinc-700">{taskTitle}</span>
        <LiveIndicator connected={connected} done={done} error={error} />
      </div>
      {error ? (
        <div className="px-4 py-3 text-sm text-zinc-500">{error}</div>
      ) : (
        <TaskOutputView events={events} isLive={!done} />
      )}
    </div>
  )
}
