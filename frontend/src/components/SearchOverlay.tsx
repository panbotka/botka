import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { Search, ListTodo, FolderGit2, MessageSquare, FileText, Loader2 } from 'lucide-react'
import { clsx } from 'clsx'
import { globalSearch } from '../api/client'
import type { GlobalSearchResults } from '../types'

interface Props {
  open: boolean
  onClose: () => void
}

type ResultItem = {
  id: string
  type: 'task' | 'project' | 'thread' | 'message'
  title: string
  subtitle?: string
  path: string
}

function toItems(data: GlobalSearchResults): ResultItem[] {
  const items: ResultItem[] = []

  for (const th of data.threads) {
    items.push({
      id: `thread-${th.id}`,
      type: 'thread',
      title: th.title || 'Untitled thread',
      path: `/chat/${th.id}`,
    })
  }

  for (const t of data.tasks) {
    items.push({
      id: `task-${t.id}`,
      type: 'task',
      title: t.title,
      subtitle: [t.status, t.project_name].filter(Boolean).join(' \u00b7 '),
      path: `/tasks/${t.id}`,
    })
  }

  for (const p of data.projects) {
    items.push({
      id: `project-${p.id}`,
      type: 'project',
      title: p.name,
      subtitle: p.path,
      path: `/projects/${p.id}`,
    })
  }

  for (const m of data.messages) {
    items.push({
      id: `message-${m.id}`,
      type: 'message',
      title: m.thread_title || 'Untitled thread',
      subtitle: stripHtml(m.snippet),
      path: `/chat/${m.thread_id}`,
    })
  }

  return items
}

function stripHtml(s: string): string {
  return s.replace(/<[^>]*>/g, '')
}

const sectionLabels: Record<ResultItem['type'], string> = {
  task: 'Tasks',
  project: 'Projects',
  thread: 'Threads',
  message: 'Messages',
}

const sectionIcons: Record<ResultItem['type'], typeof ListTodo> = {
  task: ListTodo,
  project: FolderGit2,
  thread: MessageSquare,
  message: FileText,
}

export default function SearchOverlay({ open, onClose }: Props) {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState<ResultItem[]>([])
  const [loading, setLoading] = useState(false)
  const [selectedIndex, setSelectedIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const navigate = useNavigate()
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Focus input on open
  useEffect(() => {
    if (open) {
      setQuery('')
      setResults([])
      setSelectedIndex(0)
      setTimeout(() => inputRef.current?.focus(), 0)
    }
  }, [open])

  // Debounced search
  const doSearch = useCallback((q: string) => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (q.length < 2) {
      setResults([])
      setLoading(false)
      return
    }
    setLoading(true)
    debounceRef.current = setTimeout(async () => {
      try {
        const data = await globalSearch(q)
        setResults(toItems(data))
      } catch {
        setResults([])
      } finally {
        setLoading(false)
      }
    }, 300)
  }, [])

  useEffect(() => {
    doSearch(query)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [query, doSearch])

  // Reset selection when results change
  useEffect(() => {
    setSelectedIndex(0)
  }, [results.length])

  // Scroll selected item into view
  useEffect(() => {
    const el = listRef.current?.querySelector('[data-selected="true"]') as HTMLElement | null
    el?.scrollIntoView({ block: 'nearest' })
  }, [selectedIndex])

  const handleNavigate = useCallback(
    (item: ResultItem) => {
      onClose()
      navigate(item.path)
    },
    [onClose, navigate],
  )

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev + 1) % Math.max(results.length, 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelectedIndex((prev) => (prev - 1 + results.length) % Math.max(results.length, 1))
      } else if (e.key === 'Enter' && results.length > 0) {
        e.preventDefault()
        const item = results[selectedIndex]
        if (item) handleNavigate(item)
      } else if (e.key === 'Escape') {
        e.preventDefault()
        onClose()
      }
    },
    [results, selectedIndex, onClose, handleNavigate],
  )

  if (!open) return null

  // Group items by type for section headers
  let lastType = ''

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative bg-white dark:bg-zinc-100 border border-zinc-200 rounded-2xl shadow-2xl shadow-black/20 w-full max-w-lg mx-4 overflow-hidden animate-palette-in"
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
            placeholder="Search threads, tasks, projects, messages..."
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

        {/* Results */}
        <div ref={listRef} className="max-h-80 overflow-y-auto py-1">
          {query.length >= 2 && !loading && results.length === 0 && (
            <div className="px-4 py-8 text-center text-sm text-zinc-400">
              No results for &ldquo;{query}&rdquo;
            </div>
          )}

          {query.length < 2 && (
            <div className="px-4 py-8 text-center text-sm text-zinc-400">
              Type at least 2 characters to search
            </div>
          )}

          {results.map((item, i) => {
            const showHeader = item.type !== lastType
            lastType = item.type
            const Icon = sectionIcons[item.type]

            return (
              <div key={item.id}>
                {showHeader && (
                  <div className="px-4 pt-2 pb-1 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
                    {sectionLabels[item.type]}
                  </div>
                )}
                <button
                  type="button"
                  data-selected={i === selectedIndex}
                  className={clsx(
                    'w-full text-left px-4 py-2 flex items-center gap-3 cursor-pointer transition-colors text-sm',
                    i === selectedIndex
                      ? 'bg-zinc-200 text-zinc-900'
                      : 'text-zinc-600 hover:bg-zinc-50 dark:hover:bg-zinc-200',
                  )}
                  onClick={() => handleNavigate(item)}
                  onMouseEnter={() => setSelectedIndex(i)}
                >
                  <span className="flex-shrink-0 w-5 h-5 flex items-center justify-center text-zinc-400">
                    <Icon className="w-4 h-4" />
                  </span>
                  <div className="flex-1 min-w-0">
                    <span className="truncate block">{item.title}</span>
                    {item.subtitle && (
                      <span className="text-zinc-400 text-xs truncate block">{item.subtitle}</span>
                    )}
                  </div>
                </button>
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
