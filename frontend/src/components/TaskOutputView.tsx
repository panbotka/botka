import { useEffect, useRef, useState, useCallback } from 'react'
import type { TaskOutputEvent } from '../utils/parseNDJSON'
import MarkdownContent from './MarkdownContent'
import ToolCallPanel from './ToolCallPanel'
import { AlertTriangle, ChevronDown } from 'lucide-react'

interface Props {
  events: TaskOutputEvent[]
  isLive?: boolean
}

const INITIAL_RENDER_COUNT = 200
const LOAD_MORE_COUNT = 200

export default function TaskOutputView({ events, isLive }: Props) {
  const containerRef = useRef<HTMLDivElement>(null)
  const shouldAutoScroll = useRef(true)
  const [showScrollButton, setShowScrollButton] = useState(false)
  const [visibleCount, setVisibleCount] = useState(INITIAL_RENDER_COUNT)

  // Group consecutive text events together for better rendering
  const groupedEvents = groupEvents(events)

  // For performance: only render tail events, with "Load more" at top
  const totalGroups = groupedEvents.length
  const startIndex = Math.max(0, totalGroups - visibleCount)
  const visibleGroups = groupedEvents.slice(startIndex)
  const hasMore = startIndex > 0

  function handleScroll() {
    const el = containerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
    shouldAutoScroll.current = atBottom
    setShowScrollButton(!atBottom)
  }

  useEffect(() => {
    const el = containerRef.current
    if (el && shouldAutoScroll.current) {
      el.scrollTop = el.scrollHeight
    }
  }, [events.length])

  const scrollToBottom = useCallback(() => {
    const el = containerRef.current
    if (el) {
      el.scrollTop = el.scrollHeight
      shouldAutoScroll.current = true
      setShowScrollButton(false)
    }
  }, [])

  function loadMore() {
    setVisibleCount(prev => prev + LOAD_MORE_COUNT)
  }

  if (events.length === 0) {
    return (
      <div className="flex items-center justify-center py-12 text-sm text-zinc-400">
        {isLive ? 'Waiting for output...' : 'No output captured.'}
      </div>
    )
  }

  return (
    <div className="relative">
      <div
        ref={containerRef}
        onScroll={handleScroll}
        className="overflow-y-auto space-y-1 p-4"
        style={{ maxHeight: '600px' }}
      >
        {hasMore && (
          <button
            onClick={loadMore}
            className="mb-2 w-full rounded-md border border-zinc-200 bg-zinc-50 px-3 py-1.5 text-xs text-zinc-500 hover:bg-zinc-100"
          >
            Load {Math.min(LOAD_MORE_COUNT, startIndex)} more events...
          </button>
        )}

        {visibleGroups.map((group, i) => (
          <EventGroup key={startIndex + i} group={group} />
        ))}
      </div>

      {showScrollButton && (
        <button
          onClick={scrollToBottom}
          className="absolute bottom-3 right-3 flex items-center gap-1 rounded-full bg-zinc-800 px-3 py-1.5 text-xs text-zinc-200 shadow-lg hover:bg-zinc-700"
        >
          <ChevronDown className="h-3 w-3" />
          Scroll to bottom
        </button>
      )}
    </div>
  )
}

type EventGroup =
  | { type: 'text'; text: string }
  | { type: 'tool_use'; name: string; input: Record<string, unknown> }
  | { type: 'result'; cost_usd: number; duration_ms: number; is_error: boolean }
  | { type: 'system_error'; message: string }

function groupEvents(events: TaskOutputEvent[]): EventGroup[] {
  const groups: EventGroup[] = []
  let pendingText = ''

  for (const ev of events) {
    if (ev.type === 'text') {
      if (pendingText) pendingText += '\n'
      pendingText += ev.text
    } else {
      if (pendingText) {
        groups.push({ type: 'text', text: pendingText })
        pendingText = ''
      }
      groups.push(ev)
    }
  }
  if (pendingText) {
    groups.push({ type: 'text', text: pendingText })
  }

  return groups
}

function EventGroup({ group }: { group: EventGroup }) {
  switch (group.type) {
    case 'text':
      return (
        <div className="prose prose-sm prose-zinc max-w-none min-w-0 break-words">
          <MarkdownContent content={group.text} />
        </div>
      )
    case 'tool_use':
      return <ToolCallPanel name={group.name} input={group.input} />
    case 'result':
      return <ResultEvent {...group} />
    case 'system_error':
      return <SystemErrorEvent message={group.message} />
  }
}

function ResultEvent({ cost_usd, duration_ms, is_error }: { cost_usd: number; duration_ms: number; is_error: boolean }) {
  const seconds = Math.floor(duration_ms / 1000)
  const minutes = Math.floor(seconds / 60)
  const durationStr = minutes > 0
    ? `${minutes}m ${seconds % 60}s`
    : `${seconds}s`

  return (
    <div className={`mt-2 flex items-center gap-3 rounded-md px-3 py-2 text-xs font-medium ${
      is_error
        ? 'bg-red-50 text-red-700 border border-red-200'
        : 'bg-emerald-50 text-emerald-700 border border-emerald-200'
    }`}>
      <span>{is_error ? 'Failed' : 'Completed'}</span>
      {cost_usd > 0 && <span>${cost_usd.toFixed(4)}</span>}
      {duration_ms > 0 && <span>{durationStr}</span>}
    </div>
  )
}

function SystemErrorEvent({ message }: { message: string }) {
  return (
    <div className="my-2 flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-800">
      <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{message}</span>
    </div>
  )
}
