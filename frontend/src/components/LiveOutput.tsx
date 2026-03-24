import { useEffect, useRef } from 'react'
import { useSSE } from '../hooks/useSSE'

function LiveIndicator({ connected, done }: { connected: boolean; done: boolean }) {
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
      <span className="inline-flex items-center gap-1.5 text-xs font-medium text-emerald-400">
        <span className="h-2 w-2 animate-pulse rounded-full bg-emerald-400" />
        LIVE
      </span>
    )
  }
  return (
    <span className="inline-flex items-center gap-1.5 text-xs font-medium text-amber-400">
      <span className="h-2 w-2 rounded-full bg-amber-400" />
      RECONNECTING
    </span>
  )
}

function OutputBody({
  output,
  done,
  maxHeight,
}: {
  output: string
  done: boolean
  maxHeight: string
}) {
  const containerRef = useRef<HTMLDivElement>(null)
  const shouldAutoScroll = useRef(true)

  function handleScroll() {
    const el = containerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    shouldAutoScroll.current = atBottom
  }

  useEffect(() => {
    const el = containerRef.current
    if (el && shouldAutoScroll.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [output])

  return (
    <div
      ref={containerRef}
      onScroll={handleScroll}
      className="overflow-y-auto p-4 font-mono text-sm leading-relaxed text-zinc-300"
      style={{ maxHeight }}
    >
      {output ? (
        <pre className="whitespace-pre-wrap break-words">{output}</pre>
      ) : (
        <span className="text-zinc-500">
          {done ? 'No output captured.' : 'Waiting for output...'}
        </span>
      )}
    </div>
  )
}

export function LiveOutputInline({ taskId, taskTitle }: { taskId: string; taskTitle: string }) {
  const { output, connected, done } = useSSE(taskId)

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-700 bg-[#1a1b26]">
      <div className="flex items-center justify-between border-b border-zinc-700 px-4 py-2">
        <span className="truncate text-sm font-medium text-zinc-200">{taskTitle}</span>
        <LiveIndicator connected={connected} done={done} />
      </div>
      <OutputBody output={output} done={done} maxHeight="500px" />
    </div>
  )
}
