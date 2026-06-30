import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, Loader2, User, Bot, Settings as SettingsIcon, MessageSquare, ListTodo } from 'lucide-react'
import { clsx } from 'clsx'
import { searchAll } from '../api/client'
import type { ThreadSearchHit, TaskSearchHit } from '../types'

interface Props {
  open: boolean
  onClose: () => void
  initialQuery?: string
}

const DEBOUNCE_MS = 250

const roleIcon = (role?: string) => {
  if (role === 'user') return User
  if (role === 'assistant') return Bot
  return SettingsIcon
}

// Flat, keyboard-navigable item over both result sections.
type Item =
  | { kind: 'thread'; hit: ThreadSearchHit }
  | { kind: 'task'; hit: TaskSearchHit }

const statusColor: Record<string, string> = {
  done: 'text-emerald-600 bg-emerald-50',
  running: 'text-sky-600 bg-sky-50',
  failed: 'text-red-600 bg-red-50',
  needs_review: 'text-amber-600 bg-amber-50',
  queued: 'text-zinc-500 bg-zinc-100',
  pending: 'text-zinc-500 bg-zinc-100',
  cancelled: 'text-zinc-400 bg-zinc-100',
}

export default function SearchOverlay({ open, onClose, initialQuery }: Props) {
  const [query, setQuery] = useState('')
  const [threads, setThreads] = useState<ThreadSearchHit[]>([])
  const [tasks, setTasks] = useState<TaskSearchHit[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)
  // Track the in-flight search query so a fast typist's stale responses
  // can't overwrite newer ones.
  const requestSeqRef = useRef(0)

  // Reset state and focus on open
  useEffect(() => {
    if (open) {
      setQuery(initialQuery ?? '')
      setThreads([])
      setTasks([])
      setSelectedIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open, initialQuery])

  // Debounced unified search
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (!open) return
    const trimmed = query.trim()
    if (trimmed.length < 2) {
      requestSeqRef.current += 1
      setThreads([])
      setTasks([])
      setLoading(false)
      return
    }
    setLoading(true)
    debounceRef.current = setTimeout(() => {
      const seq = ++requestSeqRef.current
      searchAll(trimmed)
        .then((res) => {
          if (seq !== requestSeqRef.current) return
          setThreads(res.threads)
          setTasks(res.tasks)
        })
        .catch(() => {
          if (seq !== requestSeqRef.current) return
          setThreads([])
          setTasks([])
        })
        .finally(() => {
          if (seq === requestSeqRef.current) setLoading(false)
        })
    }, DEBOUNCE_MS)

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, open])

  const items = useMemo<Item[]>(
    () => [
      ...threads.map((hit): Item => ({ kind: 'thread', hit })),
      ...tasks.map((hit): Item => ({ kind: 'task', hit })),
    ],
    [threads, tasks],
  )

  // Reset selection when results change
  useEffect(() => {
    setSelectedIndex(0)
  }, [items.length])

  // Scroll selected item into view
  useEffect(() => {
    const el = listRef.current?.querySelector('[data-selected="true"]') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  const handleNavigate = useCallback(
    (item: Item) => {
      onClose()
      if (item.kind === 'thread') {
        // Land at the end of the conversation with the composer focused — the
        // common case is "continue this chat" rather than re-read the old hit.
        navigate(`/chat/${item.hit.thread_id}?focus=1`)
      } else {
        navigate(`/tasks/${item.hit.task_id}`)
      }
    },
    [onClose, navigate],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev + 1) % Math.max(items.length, 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev - 1 + items.length) % Math.max(items.length, 1))
      } else if (e.key === 'Enter' && items.length > 0) {
        e.preventDefault()
        const item = items[selectedIndex]
        if (item) handleNavigate(item)
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    },
    [items, selectedIndex, onClose, handleNavigate],
  )

  if (!open) return null

  const trimmed = query.trim()
  const hasQuery = trimmed.length >= 2
  const total = items.length

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative bg-white dark:bg-zinc-100 border border-zinc-200 rounded-2xl shadow-2xl shadow-black/20 w-full max-w-2xl mx-4 overflow-hidden animate-palette-in"
        onKeyDown={handleKeyDown}
      >
        {/* Search input */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-zinc-200">
          <Search className="w-4 h-4 text-zinc-400 flex-shrink-0" />
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Hledat v konverzacích a úkolech..."
            className="flex-1 bg-transparent text-sm text-zinc-900 placeholder-zinc-400 outline-none"
          />
          {loading && <Loader2 className="w-4 h-4 text-zinc-400 animate-spin flex-shrink-0" />}
          {query && !loading && (
            <button
              onClick={() => setQuery('')}
              className="text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer text-sm"
            >
              Clear
            </button>
          )}
          <kbd className="text-[10px] text-zinc-400 bg-zinc-200 px-1.5 py-0.5 rounded border border-zinc-300 font-mono flex-shrink-0">
            Esc
          </kbd>
        </div>

        {/* Status header */}
        {hasQuery && !loading && total > 0 && (
          <div className="px-4 py-1.5 text-[11px] text-zinc-500 border-b border-zinc-100 bg-zinc-50/50 flex items-center justify-between">
            <span>
              <span className="font-medium text-zinc-700">{threads.length}</span> konverzací ·{' '}
              <span className="font-medium text-zinc-700">{tasks.length}</span> úkolů
            </span>
            <span className="flex items-center gap-1 text-zinc-400">
              <kbd className="text-[10px] bg-white px-1 py-0.5 rounded border border-zinc-200 font-mono">↑↓</kbd>
              <span>pohyb</span>
              <kbd className="text-[10px] bg-white px-1 py-0.5 rounded border border-zinc-200 font-mono ml-1">↵</kbd>
              <span>otevřít</span>
            </span>
          </div>
        )}

        {/* Results */}
        <div ref={listRef} className="max-h-[60vh] overflow-y-auto py-1">
          {!hasQuery && (
            <div className="px-4 py-10 text-center text-sm text-zinc-400">
              <MessageSquare className="w-6 h-6 mx-auto mb-2 text-zinc-300" />
              <p>Napiš aspoň 2 znaky pro hledání v konverzacích a úkolech.</p>
            </div>
          )}

          {hasQuery && !loading && total === 0 && (
            <div className="px-4 py-10 text-center text-sm text-zinc-400">
              Nic neodpovídá &ldquo;{trimmed}&rdquo;.
            </div>
          )}

          {threads.length > 0 && (
            <div className="px-4 pt-2 pb-1 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
              Konverzace
            </div>
          )}
          {threads.map((hit, i) => {
            const Icon = roleIcon(hit.role)
            return (
              <button
                type="button"
                key={`thread-${hit.thread_id}`}
                data-selected={i === selectedIndex}
                className={clsx(
                  'w-full text-left px-4 py-2.5 cursor-pointer transition-colors',
                  i === selectedIndex
                    ? 'bg-zinc-200/80 text-zinc-900'
                    : 'text-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-200',
                )}
                onClick={() => handleNavigate({ kind: 'thread', hit })}
                onMouseEnter={() => setSelectedIndex(i)}
              >
                <div className="flex items-center gap-2 mb-0.5">
                  <MessageSquare className="w-3 h-3 text-zinc-400 flex-shrink-0" />
                  <span className="text-sm font-medium text-zinc-800 truncate">
                    {hit.thread_title || 'Bez názvu'}
                  </span>
                  {hit.matched_title && (
                    <span className="text-[9px] font-semibold uppercase tracking-wide text-amber-600 bg-amber-50 rounded px-1 py-0.5 flex-shrink-0">
                      název
                    </span>
                  )}
                </div>
                {hit.content_snippet ? (
                  <div className="flex items-center gap-1.5">
                    <Icon className="w-3 h-3 text-zinc-400 flex-shrink-0" aria-label={hit.role} />
                    <div
                      className="text-xs text-zinc-500 leading-snug truncate flex-1 [&_mark]:bg-amber-200 [&_mark]:text-zinc-900 [&_mark]:rounded-sm [&_mark]:px-0.5"
                      // The server escapes everything except <mark>, so this is safe.
                      dangerouslySetInnerHTML={{ __html: hit.content_snippet }}
                    />
                  </div>
                ) : (
                  <div className="text-xs text-zinc-400 italic">Shoda v názvu konverzace</div>
                )}
              </button>
            )
          })}

          {tasks.length > 0 && (
            <div className="px-4 pt-2 pb-1 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
              Úkoly
            </div>
          )}
          {tasks.map((hit, j) => {
            const i = threads.length + j
            return (
              <button
                type="button"
                key={`task-${hit.task_id}`}
                data-selected={i === selectedIndex}
                className={clsx(
                  'w-full text-left px-4 py-2.5 cursor-pointer transition-colors',
                  i === selectedIndex
                    ? 'bg-zinc-200/80 text-zinc-900'
                    : 'text-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-200',
                )}
                onClick={() => handleNavigate({ kind: 'task', hit })}
                onMouseEnter={() => setSelectedIndex(i)}
              >
                <div className="flex items-center gap-2 mb-0.5">
                  <ListTodo className="w-3 h-3 text-zinc-400 flex-shrink-0" />
                  <span className="text-sm font-medium text-zinc-800 truncate flex-1">
                    {hit.title || 'Bez názvu'}
                  </span>
                  <span
                    className={clsx(
                      'text-[9px] font-semibold uppercase tracking-wide rounded px-1 py-0.5 flex-shrink-0',
                      statusColor[hit.status] ?? 'text-zinc-500 bg-zinc-100',
                    )}
                  >
                    {hit.status}
                  </span>
                </div>
                {hit.content_snippet && (
                  <div
                    className="text-xs text-zinc-500 leading-snug truncate [&_mark]:bg-amber-200 [&_mark]:text-zinc-900 [&_mark]:rounded-sm [&_mark]:px-0.5"
                    // The server escapes everything except <mark>, so this is safe.
                    dangerouslySetInnerHTML={{ __html: hit.content_snippet }}
                  />
                )}
              </button>
            )
          })}
        </div>
      </div>
    </div>
  )
}
