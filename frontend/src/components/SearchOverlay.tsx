import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, Loader2, User, Bot, Settings as SettingsIcon, MessageSquare } from 'lucide-react'
import { clsx } from 'clsx'
import { searchMessagesFTS } from '../api/client'
import type { MessageSearchHit } from '../types'

interface Props {
  open: boolean
  onClose: () => void
  initialQuery?: string
}

const PAGE_SIZE = 30
const DEBOUNCE_MS = 250

const roleIcon = (role: string) => {
  if (role === 'user') return User
  if (role === 'assistant') return Bot
  return SettingsIcon
}

export default function SearchOverlay({ open, onClose, initialQuery }: Props) {
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<MessageSearchHit[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [loadingMore, setLoadingMore] = useState(false)
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
      setHits([])
      setTotal(0)
      setSelectedIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open, initialQuery])

  // Debounced first-page search
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (!open) return
    const trimmed = query.trim()
    if (trimmed.length < 2) {
      requestSeqRef.current += 1
      setHits([])
      setTotal(0)
      setLoading(false)
      return
    }
    setLoading(true)
    debounceRef.current = setTimeout(() => {
      const seq = ++requestSeqRef.current
      searchMessagesFTS({ query: trimmed, limit: PAGE_SIZE, offset: 0 })
        .then((resp) => {
          if (seq !== requestSeqRef.current) return
          setHits(resp.data)
          setTotal(resp.total)
        })
        .catch(() => {
          if (seq !== requestSeqRef.current) return
          setHits([])
          setTotal(0)
        })
        .finally(() => {
          if (seq === requestSeqRef.current) setLoading(false)
        })
    }, DEBOUNCE_MS)

    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, open])

  // Reset selection when results change
  useEffect(() => {
    setSelectedIndex(0)
  }, [hits.length])

  // Scroll selected item into view
  useEffect(() => {
    const el = listRef.current?.querySelector('[data-selected="true"]') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  const handleNavigate = useCallback(
    (hit: MessageSearchHit) => {
      onClose()
      navigate(`/chat/${hit.thread_id}?msg=${hit.message_id}`)
    },
    [onClose, navigate],
  )

  const handleLoadMore = useCallback(() => {
    const trimmed = query.trim()
    if (trimmed.length < 2 || loadingMore || hits.length >= total) return
    setLoadingMore(true)
    const seq = requestSeqRef.current
    searchMessagesFTS({ query: trimmed, limit: PAGE_SIZE, offset: hits.length })
      .then((resp) => {
        if (seq !== requestSeqRef.current) return
        setHits((prev) => [...prev, ...resp.data])
        setTotal(resp.total)
      })
      .catch(() => {})
      .finally(() => {
        if (seq === requestSeqRef.current) setLoadingMore(false)
      })
  }, [query, hits.length, total, loadingMore])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev + 1) % Math.max(hits.length, 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev - 1 + hits.length) % Math.max(hits.length, 1))
      } else if (e.key === 'Enter' && hits.length > 0) {
        e.preventDefault()
        const hit = hits[selectedIndex]
        if (hit) handleNavigate(hit)
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    },
    [hits, selectedIndex, onClose, handleNavigate],
  )

  if (!open) return null

  const trimmed = query.trim()
  const hasQuery = trimmed.length >= 2
  const hasMore = hits.length < total

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
            placeholder="Search messages across all threads..."
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

        {/* Total / status header */}
        {hasQuery && !loading && (
          <div className="px-4 py-1.5 text-[11px] text-zinc-500 border-b border-zinc-100 bg-zinc-50/50 flex items-center justify-between">
            <span>
              Total: <span className="font-medium text-zinc-700">{total}</span> {total === 1 ? 'match' : 'matches'}
            </span>
            {total > 0 && (
              <span className="flex items-center gap-1 text-zinc-400">
                <kbd className="text-[10px] bg-white px-1 py-0.5 rounded border border-zinc-200 font-mono">↑↓</kbd>
                <span>navigate</span>
                <kbd className="text-[10px] bg-white px-1 py-0.5 rounded border border-zinc-200 font-mono ml-1">↵</kbd>
                <span>open</span>
              </span>
            )}
          </div>
        )}

        {/* Results */}
        <div ref={listRef} className="max-h-[60vh] overflow-y-auto py-1">
          {!hasQuery && (
            <div className="px-4 py-10 text-center text-sm text-zinc-400">
              <MessageSquare className="w-6 h-6 mx-auto mb-2 text-zinc-300" />
              <p>Type at least 2 characters to search across messages.</p>
            </div>
          )}

          {hasQuery && !loading && hits.length === 0 && (
            <div className="px-4 py-10 text-center text-sm text-zinc-400">
              No messages match &ldquo;{trimmed}&rdquo;.
            </div>
          )}

          {hits.map((hit, i) => {
            const Icon = roleIcon(hit.role)
            return (
              <button
                type="button"
                key={`${hit.message_id}`}
                data-selected={i === selectedIndex}
                className={clsx(
                  'w-full text-left px-4 py-2.5 cursor-pointer transition-colors',
                  i === selectedIndex
                    ? 'bg-zinc-200/80 text-zinc-900'
                    : 'text-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-200',
                )}
                onClick={() => handleNavigate(hit)}
                onMouseEnter={() => setSelectedIndex(i)}
              >
                <div className="flex items-center gap-2 mb-0.5">
                  <span
                    className="text-[10px] font-semibold tracking-wider text-zinc-500 truncate"
                    style={{ fontVariant: 'small-caps', textTransform: 'lowercase' }}
                  >
                    {hit.thread_title || 'Untitled thread'}
                  </span>
                  <span className="text-zinc-300">·</span>
                  <Icon className="w-3 h-3 text-zinc-400 flex-shrink-0" aria-label={hit.role} />
                  <span className="text-[10px] text-zinc-400 capitalize">{hit.role}</span>
                </div>
                <div
                  className="text-sm text-zinc-700 leading-snug [&_mark]:bg-amber-200 [&_mark]:text-zinc-900 [&_mark]:rounded-sm [&_mark]:px-0.5"
                  // The server escapes everything except <mark>, so this is safe.
                  dangerouslySetInnerHTML={{ __html: hit.content_snippet }}
                />
              </button>
            )
          })}

          {hasMore && hasQuery && (
            <div className="px-4 py-2 flex justify-center">
              <button
                type="button"
                onClick={handleLoadMore}
                disabled={loadingMore}
                className="text-xs text-zinc-500 hover:text-zinc-800 transition-colors px-3 py-1.5 rounded-md hover:bg-zinc-100 cursor-pointer disabled:opacity-50 disabled:cursor-wait flex items-center gap-1.5"
              >
                {loadingMore && <Loader2 className="w-3 h-3 animate-spin" />}
                Load more ({total - hits.length} remaining)
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
