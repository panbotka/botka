import { useState, useEffect, useRef, useCallback } from 'react'
import { Search, Loader2, X, ChevronDown, ChevronUp } from 'lucide-react'
import { searchMessagesFTS } from '../api/client'
import type { MessageSearchHit } from '../types'

interface Props {
  threadId: number
  open: boolean
  onClose: () => void
  // Called when the user picks a hit. Receives the message id so the parent
  // can scroll to / flash the message in place without a route navigation.
  onSelect: (messageId: number) => void
}

const PAGE_SIZE = 20
const DEBOUNCE_MS = 250

// InThreadSearch is the inline "search this thread" affordance shown above the
// chat. It reuses the same /search/messages endpoint as the global palette,
// just scoped to a single thread. Hits cycle in place via ↑/↓ + Enter.
export default function InThreadSearch({ threadId, open, onClose, onSelect }: Props) {
  const [query, setQuery] = useState('')
  const [hits, setHits] = useState<MessageSearchHit[]>([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(false)
  const [activeIndex, setActiveIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)
  const requestSeqRef = useRef(0)

  useEffect(() => {
    if (open) {
      setQuery('')
      setHits([])
      setTotal(0)
      setActiveIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open])

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
      searchMessagesFTS({ query: trimmed, threadId, limit: PAGE_SIZE })
        .then((resp) => {
          if (seq !== requestSeqRef.current) return
          setHits(resp.data)
          setTotal(resp.total)
          setActiveIndex(0)
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
  }, [query, open, threadId])

  // Auto-jump to the active hit so the chat scrolls as the user navigates.
  useEffect(() => {
    if (hits.length === 0) return
    const hit = hits[activeIndex]
    if (hit) onSelect(hit.message_id)
  }, [activeIndex, hits, onSelect])

  const goNext = useCallback(() => {
    setActiveIndex((i) => (hits.length ? (i + 1) % hits.length : 0))
  }, [hits.length])
  const goPrev = useCallback(() => {
    setActiveIndex((i) => (hits.length ? (i - 1 + hits.length) % hits.length : 0))
  }, [hits.length])

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'Enter') {
        e.preventDefault()
        if (e.shiftKey) goPrev()
        else goNext()
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        goNext()
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        goPrev()
      }
    },
    [goNext, goPrev, onClose],
  )

  if (!open) return null
  const trimmed = query.trim()
  const hasQuery = trimmed.length >= 2

  return (
    <div className="absolute top-2 right-4 z-20 bg-white dark:bg-zinc-100 border border-zinc-200 rounded-xl shadow-lg shadow-black/10 w-80 max-w-[calc(100%-2rem)] animate-palette-in">
      <div className="flex items-center gap-2 px-3 py-2">
        <Search className="w-4 h-4 text-zinc-400 flex-shrink-0" />
        <input
          ref={inputRef}
          type="text"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onKeyDown={handleKeyDown}
          placeholder="Search this thread..."
          className="flex-1 bg-transparent text-sm text-zinc-900 placeholder-zinc-400 outline-none min-w-0"
        />
        {loading && <Loader2 className="w-4 h-4 text-zinc-400 animate-spin flex-shrink-0" />}
        {hasQuery && !loading && (
          <span className="text-[11px] text-zinc-400 flex-shrink-0 tabular-nums">
            {total === 0 ? '0 / 0' : `${activeIndex + 1} / ${total}`}
          </span>
        )}
        <div className="flex items-center gap-0.5 flex-shrink-0">
          <button
            type="button"
            onClick={goPrev}
            disabled={hits.length === 0}
            aria-label="Previous match"
            className="p-1 text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
          >
            <ChevronUp className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={goNext}
            disabled={hits.length === 0}
            aria-label="Next match"
            className="p-1 text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100 rounded transition-colors disabled:opacity-40 disabled:cursor-not-allowed cursor-pointer"
          >
            <ChevronDown className="w-3.5 h-3.5" />
          </button>
          <button
            type="button"
            onClick={onClose}
            aria-label="Close search"
            className="p-1 text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100 rounded transition-colors cursor-pointer"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        </div>
      </div>
      {hasQuery && hits[activeIndex] && (
        <div
          className="px-3 pb-2 text-xs text-zinc-500 leading-snug border-t border-zinc-100 pt-2 [&_mark]:bg-amber-200 [&_mark]:text-zinc-900 [&_mark]:rounded-sm [&_mark]:px-0.5"
          // Server-sanitized: only <mark> survives.
          dangerouslySetInnerHTML={{ __html: hits[activeIndex].content_snippet }}
        />
      )}
    </div>
  )
}
