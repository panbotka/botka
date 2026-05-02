import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import type { Persona, Thread, Project, GlobalSearchResults } from '../types'
import { formatDate as formatDateOnly } from '../utils/dateFormat'
import { api, globalSearch, ApiError } from '../api/client'
import { downloadExport } from '../utils/exportThread'
import { clearDraft } from './ChatInput'
import { useStreamingThreadIds } from '../context/SSEContext'
import BoxStatusBadge from './BoxStatusBadge'
import {
  Plus, Search, Pin, Archive, MoreVertical, Pencil,
  Trash2, Download,
  X, ChevronDown, FolderGit2, MessageSquare, Server,
  ListTodo, FileText,
} from 'lucide-react'
import { THREAD_COLORS } from '../utils/threadColors'
import {
  parseSearchQuery,
  matchesPrefixFilters,
  removeFilter,
  filterChipLabel,
} from '../utils/searchQuery'

// How many results per section we show inline before collapsing into "+N more".
const SIDEBAR_SECTION_LIMIT = 10
// Backend max is 20; request enough to know when "+N more" should appear.
const SIDEBAR_SEARCH_FETCH_LIMIT = 20

function taskStatusBadgeClass(status: string): string {
  switch (status) {
    case 'pending':       return 'bg-zinc-100 text-zinc-600'
    case 'queued':        return 'bg-blue-50 text-blue-700'
    case 'running':       return 'bg-amber-50 text-amber-700'
    case 'done':          return 'bg-emerald-50 text-emerald-700'
    case 'failed':        return 'bg-red-50 text-red-700'
    case 'needs_review':  return 'bg-orange-50 text-orange-700'
    case 'cancelled':     return 'bg-zinc-50 text-zinc-400 line-through'
    case 'deleted':       return 'bg-zinc-50 text-zinc-400 line-through'
    default:              return 'bg-zinc-100 text-zinc-600'
  }
}

interface Props {
  threads: Thread[]
  activeThreadId: number | null
  onSelectThread: (id: number) => void
  onNewThread: (personaId?: number) => void
  onThreadsChange: () => void
  showArchived: boolean
  onToggleArchived: () => void
  personas: Persona[]
  projects: Project[]
  activeProcessThreadIds: Set<number>
  mobile?: boolean
  readOnly?: boolean
  /** Optional callback to navigate to a path (used for cross-category search results). */
  onNavigate?: (path: string) => void
}

export default function ThreadSidebar({
  threads,
  activeThreadId,
  onSelectThread,
  onNewThread,
  onThreadsChange,
  showArchived,
  onToggleArchived,
  personas,
  projects,
  activeProcessThreadIds,
  mobile,
  readOnly,
  onNavigate,
}: Props) {
  const streamingThreadIds = useStreamingThreadIds()
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [menuOpenId, setMenuOpenId] = useState<number | null>(null)
  const [personaDropdownOpen, setPersonaDropdownOpen] = useState(false)
  const personaDropdownRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<GlobalSearchResults | null>(null)
  const [searchLoading, setSearchLoading] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Close menu on outside click or Escape
  useEffect(() => {
    if (menuOpenId === null) return
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpenId(null)
      }
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setMenuOpenId(null)
      }
    }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
    }
  }, [menuOpenId])

  // Close persona dropdown on outside click
  useEffect(() => {
    if (!personaDropdownOpen) return
    const handleClick = (e: MouseEvent) => {
      if (personaDropdownRef.current && !personaDropdownRef.current.contains(e.target as Node)) {
        setPersonaDropdownOpen(false)
      }
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setPersonaDropdownOpen(false)
    }
    document.addEventListener('mousedown', handleClick)
    document.addEventListener('keydown', handleKey)
    return () => {
      document.removeEventListener('mousedown', handleClick)
      document.removeEventListener('keydown', handleKey)
    }
  }, [personaDropdownOpen])

  const clearSearch = useCallback(() => {
    setSearchQuery('')
    setSearchResults(null)
    setSearchLoading(false)
    if (debounceRef.current) clearTimeout(debounceRef.current)
  }, [])

  const parsedQuery = useMemo(() => parseSearchQuery(searchQuery), [searchQuery])
  const hasPrefix = parsedQuery.filters.length > 0
  const freeText = parsedQuery.freeText

  // Debounced global search across threads, messages, projects, and tasks.
  // Only the free-text portion of the input is sent to the API; prefix
  // filters are applied client-side either to the local thread list (when
  // the input is prefix-only) or to the API thread results (when mixed).
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    if (freeText.length < 2) {
      setSearchResults(null)
      setSearchLoading(false)
      return
    }
    setSearchLoading(true)
    debounceRef.current = setTimeout(async () => {
      try {
        const results = await globalSearch(freeText, SIDEBAR_SEARCH_FETCH_LIMIT)
        setSearchResults(results)
      } catch {
        setSearchResults({ threads: [], messages: [], projects: [], tasks: [] })
      }
      setSearchLoading(false)
    }, 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [freeText])

  const handleRename = async (id: number) => {
    if (!editTitle.trim()) {
      setEditingId(null)
      return
    }
    try {
      await api.renameThread(id, editTitle.trim())
      onThreadsChange()
    } catch { /* ignore */ }
    setEditingId(null)
  }

  const handleDelete = async (id: number) => {
    try {
      await api.deleteThread(id)
      clearDraft(id)
      onThreadsChange()
    } catch { /* ignore */ }
  }

  const showToast = useCallback((msg: string) => {
    setToast(msg)
    setTimeout(() => setToast(null), 3000)
  }, [])

  const handlePin = async (id: number, pinned: boolean) => {
    try {
      if (pinned) await api.unpinThread(id)
      else await api.pinThread(id)
      onThreadsChange()
    } catch (err) {
      if (err instanceof ApiError) showToast(err.message)
    }
  }

  const handleArchive = async (id: number, archived: boolean) => {
    try {
      if (archived) await api.unarchiveThread(id)
      else await api.archiveThread(id)
      onThreadsChange()
    } catch { /* ignore */ }
  }

  const handleExport = async (thread: Thread) => {
    try {
      const detail = await api.getThread(thread.id)
      if (detail.messages.length === 0) return
      downloadExport(detail.messages, 'md', thread)
    } catch { /* ignore */ }
  }

  const formatDate = (dateStr: string) => {
    const d = new Date(dateStr)
    const now = new Date()
    const diff = now.getTime() - d.getTime()
    const mins = Math.floor(diff / 60000)
    const hours = Math.floor(diff / 3600000)
    const days = Math.floor(diff / 86400000)
    if (mins < 1) return 'now'
    if (mins < 60) return `${mins}m`
    if (hours < 24) return `${hours}h`
    if (days === 1) return 'Yesterday'
    if (days < 7) return d.toLocaleDateString('en-US', { weekday: 'short' })
    return formatDateOnly(d)
  }

  const handleSelectSearchThread = (threadId: number) => {
    onSelectThread(threadId)
    clearSearch()
  }

  const handleNavigateSearchResult = (path: string) => {
    if (onNavigate) {
      onNavigate(path)
    }
    clearSearch()
  }

  const handleOpenGlobalSearch = (query: string) => {
    window.dispatchEvent(new CustomEvent('botka:open-search', { detail: { query } }))
    clearSearch()
  }

  const isSearching = freeText.length >= 2
  // Prefix-only mode: prefix filters are active but no API search is happening.
  // We filter the local thread list rather than render search results.
  const isLocalFiltering = hasPrefix && !isSearching
  const archivedPrefixForced = parsedQuery.filters.some(
    f => f.key === 'archived' && f.value === 'true',
  )
  const archivedSectionVisible = showArchived || (isLocalFiltering && archivedPrefixForced)

  const projectMap = useMemo(() => new Map(projects.map(p => [p.id, p])), [projects])

  const matchesFilters = useCallback((thread: Thread) => {
    if (isLocalFiltering && !matchesPrefixFilters(thread, parsedQuery, projectMap)) return false
    return true
  }, [isLocalFiltering, parsedQuery, projectMap])

  const pinnedThreads = useMemo(() => threads.filter(t => t.pinned && !t.archived && matchesFilters(t)), [threads, matchesFilters])
  const regularThreads = useMemo(() => threads.filter(t => !t.pinned && !t.archived && matchesFilters(t)), [threads, matchesFilters])
  const archivedThreads = useMemo(() => threads.filter(t => t.archived && matchesFilters(t)), [threads, matchesFilters])

  const renderThread = (thread: Thread) => {
    const isSelected = activeThreadId === thread.id
    const isStreaming = streamingThreadIds.has(thread.id)
    const hasProcess = activeProcessThreadIds.has(thread.id)

    const threadColorEntry = thread.color ? THREAD_COLORS.find(c => c.key === thread.color) : null

    return (
    <div
      key={thread.id}
      className={`group flex items-stretch gap-2 px-3 py-2.5 mb-0.5
                 rounded-xl cursor-pointer transition-all duration-150
                 ${thread.archived ? 'opacity-50' : ''}
                 ${isSelected
                   ? 'bg-zinc-200/70 text-zinc-900' + (isStreaming ? ' ring-1 ring-emerald-400/50' : hasProcess ? ' ring-1 ring-emerald-400/30' : '')
                   : (isStreaming
                     ? 'bg-emerald-50 hover:bg-emerald-100/70'
                     : hasProcess
                       ? 'bg-emerald-50/50 hover:bg-emerald-50'
                       : 'hover:bg-zinc-100') + ' text-zinc-700 hover:text-zinc-900'}`}
      style={threadColorEntry ? { borderLeft: `3px solid ${threadColorEntry.swatch}40` } : undefined}
      onClick={() => onSelectThread(thread.id)}
    >
      {editingId === thread.id ? (
        <input
          value={editTitle}
          onChange={(e) => setEditTitle(e.target.value)}
          onBlur={() => handleRename(thread.id)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') handleRename(thread.id)
            if (e.key === 'Escape') setEditingId(null)
          }}
          autoFocus
          className="flex-1 bg-transparent border-b border-amber-500/50 outline-none text-zinc-900 text-sm"
          onClick={(e) => e.stopPropagation()}
        />
      ) : (
        <>
          <div className="flex-1 min-w-0">
            <div className="flex items-center justify-between gap-1.5">
              <span className="font-medium text-sm truncate flex items-center gap-1">
                {isStreaming && (
                  <span className="w-1.5 h-1.5 rounded-full flex-shrink-0 animate-pulse bg-emerald-500" />
                )}
                {thread.pinned && <Pin className="w-3 h-3 text-amber-500 flex-shrink-0" />}
                {thread.persona_icon && <span className="flex-shrink-0">{thread.persona_icon}</span>}
                {thread.signal_bridge_active && (
                  <MessageSquare className="w-3 h-3 text-emerald-500 flex-shrink-0" aria-label="Signal bridge active" />
                )}
                {thread.title || 'New conversation'}
              </span>
              <span className="text-[11px] text-zinc-400 flex-shrink-0">
                {formatDate(thread.last_message_at || thread.updated_at)}
              </span>
            </div>
            {thread.last_message_preview && (
              <div className="text-xs text-zinc-400 truncate mt-0.5">
                {thread.last_message_preview}
              </div>
            )}
            <div className="flex items-center gap-1.5 mt-0.5">
              {thread.tags && thread.tags.length > 0 && (
                <span className="flex items-center gap-0.5 flex-shrink-0">
                  {thread.tags.slice(0, 3).map(tag => (
                    <span
                      key={tag.id}
                      className="w-2 h-2 rounded-full flex-shrink-0"
                      style={{ backgroundColor: tag.color }}
                      title={tag.name}
                    />
                  ))}
                </span>
              )}
              {thread.project_id && projectMap.get(thread.project_id) && (
                <span
                  className={`text-[10px] flex items-center gap-0.5 ${
                    projectMap.get(thread.project_id)!.path.startsWith('box:')
                      ? 'text-sky-500'
                      : 'text-zinc-400'
                  }`}
                  title={projectMap.get(thread.project_id)!.path.startsWith('box:') ? 'Running on Box' : undefined}
                >
                  {projectMap.get(thread.project_id)!.path.startsWith('box:') ? (
                    <Server className="w-2.5 h-2.5" />
                  ) : (
                    <FolderGit2 className="w-2.5 h-2.5" />
                  )}
                  <span className="truncate max-w-[80px]">{projectMap.get(thread.project_id)!.name}</span>
                </span>
              )}
              {thread.total_cost_usd != null && thread.total_cost_usd > 0 && (
                <span className="text-[10px] text-zinc-400 ml-auto">${thread.total_cost_usd.toFixed(2)}</span>
              )}
            </div>
          </div>

          {!readOnly && <div
              className="relative flex items-stretch gap-0.5 flex-shrink-0"
              ref={menuOpenId === thread.id ? menuRef : undefined}
            >
              <button
                onClick={(e) => {
                  e.stopPropagation()
                  setMenuOpenId(menuOpenId === thread.id ? null : thread.id)
                }}
                className={`w-8 flex items-center justify-center rounded-lg transition-colors cursor-pointer ${
                  menuOpenId === thread.id
                    ? 'text-zinc-700 bg-zinc-200'
                    : 'text-zinc-400 hover:text-zinc-700 hover:bg-zinc-100'
                }`}
                title="Actions"
              >
                <MoreVertical className="w-4 h-4" />
              </button>

              {menuOpenId === thread.id && (
                <div
                  className="absolute right-0 top-full mt-1 z-50 w-44
                             bg-zinc-100 border border-zinc-200 rounded-xl
                             shadow-lg shadow-zinc-200/50 py-1 overflow-hidden"
                  onClick={(e) => e.stopPropagation()}
                >
                  {!thread.archived && (
                    <button
                      onClick={() => { handlePin(thread.id, thread.pinned); setMenuOpenId(null) }}
                      className="w-full flex items-center gap-3 px-3 py-2
                                 text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                    >
                      <Pin className={`w-4 h-4 flex-shrink-0 ${thread.pinned ? 'text-amber-500' : 'text-zinc-400'}`} />
                      {thread.pinned ? 'Unpin' : 'Pin'}
                    </button>
                  )}
                  <button
                    onClick={() => {
                      setEditingId(thread.id)
                      setEditTitle(thread.title)
                      setMenuOpenId(null)
                    }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <Pencil className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    Rename
                  </button>
                  <button
                    onClick={() => {
                      const msg = thread.archived
                        ? 'Are you sure you want to unarchive this thread?'
                        : 'Are you sure you want to archive this thread?'
                      if (window.confirm(msg)) handleArchive(thread.id, thread.archived)
                      setMenuOpenId(null)
                    }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <Archive className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    {thread.archived ? 'Unarchive' : 'Archive'}
                  </button>
                  <button
                    onClick={() => { handleExport(thread); setMenuOpenId(null) }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <Download className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    Export
                  </button>
                  <div className="my-1 mx-2 border-t border-zinc-100" />
                  <button
                    onClick={() => {
                      if (window.confirm('Are you sure you want to delete this thread?')) handleDelete(thread.id)
                      setMenuOpenId(null)
                    }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-red-600 hover:bg-red-50 transition-colors cursor-pointer"
                  >
                    <Trash2 className="w-4 h-4 flex-shrink-0" />
                    Delete
                  </button>
                </div>
              )}
            </div>}
        </>
      )}
    </div>
  )
  }

  // Search input component (reused in both layouts)
  const searchInput = (
    <div className="px-3 pb-2">
      <div className="relative">
        <Search className="w-4 h-4 absolute left-3 top-1/2 -translate-y-1/2 text-zinc-400 pointer-events-none" />
        <input
          ref={searchInputRef}
          type="text"
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              clearSearch()
              searchInputRef.current?.blur()
            }
          }}
          placeholder="Search messages..."
          className="w-full bg-zinc-100 border border-zinc-200 rounded-xl
                     pl-9 pr-8 py-2 text-sm text-zinc-900 placeholder-zinc-400
                     outline-none focus:border-zinc-300 focus:bg-zinc-50 transition-all duration-150"
        />
        {searchQuery && (
          <button
            onClick={clearSearch}
            className="absolute right-2 top-1/2 -translate-y-1/2 p-0.5
                       text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer"
          >
            <X className="w-3.5 h-3.5" />
          </button>
        )}
      </div>
    </div>
  )

  // Removable chip row for prefix filters parsed out of the search input.
  const activeFiltersRow = hasPrefix && (
    <div className="px-3 pb-2 flex flex-wrap gap-1.5">
      {parsedQuery.filters.map((f, i) => (
        <button
          key={`${f.key}-${i}-${f.rawText}`}
          type="button"
          onClick={() => setSearchQuery(removeFilter(searchQuery, f))}
          className="flex items-center gap-1 px-2.5 py-1 text-xs rounded-lg border
                     bg-zinc-200/60 border-zinc-300 text-zinc-800 hover:bg-zinc-300/60
                     transition-all duration-150 cursor-pointer"
          title={`Remove filter ${filterChipLabel(f)}`}
          aria-label={`Remove filter ${filterChipLabel(f)}`}
        >
          <span>{filterChipLabel(f)}</span>
          <X className="w-3 h-3" />
        </button>
      ))}
    </div>
  )

  // Lookup map for enriching thread search results with local metadata
  // (tags, project, persona_icon) — search response only has id/title/updated_at.
  const threadById = useMemo(() => new Map(threads.map(t => [t.id, t])), [threads])

  const trimmedQuery = searchQuery.trim()

  const renderSectionHeader = (label: string) => (
    <div className="px-3 py-1.5 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
      {label}
    </div>
  )

  const renderMoreLink = (sectionLabel: string, total: number) => {
    if (total <= SIDEBAR_SECTION_LIMIT) return null
    const extra = total - SIDEBAR_SECTION_LIMIT
    return (
      <button
        type="button"
        onClick={() => handleOpenGlobalSearch(trimmedQuery)}
        className="w-full text-left px-3 py-1.5 text-xs text-zinc-500 hover:text-zinc-800
                   hover:bg-zinc-100 rounded-lg transition-colors cursor-pointer"
        aria-label={`Open global search for more ${sectionLabel.toLowerCase()}`}
      >
        +{extra} more {sectionLabel.toLowerCase()}…
      </button>
    )
  }

  const renderThreadResult = (
    res: { id: number; title: string; updated_at: string },
  ) => {
    const localThread = threadById.get(res.id)
    return (
      <button
        key={`thread-${res.id}`}
        type="button"
        onClick={() => handleSelectSearchThread(res.id)}
        className="w-full text-left px-3 py-2 mb-0.5 rounded-xl hover:bg-zinc-100
                   transition-all duration-150 cursor-pointer"
      >
        <div className="flex items-center justify-between gap-1.5">
          <span className="text-sm font-medium text-zinc-900 truncate flex items-center gap-1">
            {localThread?.persona_icon && <span className="flex-shrink-0">{localThread.persona_icon}</span>}
            {res.title || 'New conversation'}
          </span>
          <span className="text-[11px] text-zinc-400 flex-shrink-0">
            {formatDate(localThread?.last_message_at || res.updated_at)}
          </span>
        </div>
        {(localThread?.tags && localThread.tags.length > 0) || (localThread?.project_id && projectMap.get(localThread.project_id)) ? (
          <div className="flex items-center gap-1.5 mt-0.5">
            {localThread?.tags && localThread.tags.length > 0 && (
              <span className="flex items-center gap-0.5 flex-shrink-0">
                {localThread.tags.slice(0, 3).map(tag => (
                  <span
                    key={tag.id}
                    className="w-2 h-2 rounded-full flex-shrink-0"
                    style={{ backgroundColor: tag.color }}
                    title={tag.name}
                  />
                ))}
              </span>
            )}
            {localThread?.project_id && projectMap.get(localThread.project_id) && (
              <span
                className={`text-[10px] flex items-center gap-0.5 ${
                  projectMap.get(localThread.project_id)!.path.startsWith('box:')
                    ? 'text-sky-500'
                    : 'text-zinc-400'
                }`}
              >
                {projectMap.get(localThread.project_id)!.path.startsWith('box:') ? (
                  <Server className="w-2.5 h-2.5" />
                ) : (
                  <FolderGit2 className="w-2.5 h-2.5" />
                )}
                <span className="truncate max-w-[80px]">{projectMap.get(localThread.project_id)!.name}</span>
              </span>
            )}
          </div>
        ) : null}
      </button>
    )
  }

  const renderMessageResult = (
    res: { id: number; thread_id: number; thread_title: string; snippet: string },
  ) => (
    <button
      key={`message-${res.id}`}
      type="button"
      onClick={() => handleSelectSearchThread(res.thread_id)}
      className="w-full text-left px-3 py-2 mb-0.5 rounded-xl hover:bg-zinc-100
                 transition-all duration-150 cursor-pointer"
    >
      <div className="flex items-center gap-1.5">
        <FileText className="w-3 h-3 flex-shrink-0 text-zinc-400" />
        <span className="text-sm text-zinc-900 truncate">
          {res.thread_title || 'Untitled thread'}
        </span>
      </div>
      <div
        className="mt-0.5 ml-[18px] text-xs text-zinc-500 line-clamp-2
                   [&_mark]:bg-amber-100 [&_mark]:text-amber-800 [&_mark]:rounded-sm [&_mark]:px-0.5"
        dangerouslySetInnerHTML={{ __html: res.snippet }}
      />
    </button>
  )

  const renderProjectResult = (
    res: { id: string; name: string; path: string },
  ) => (
    <button
      key={`project-${res.id}`}
      type="button"
      onClick={() => handleNavigateSearchResult(`/projects/${res.id}`)}
      className="w-full text-left px-3 py-2 mb-0.5 rounded-xl hover:bg-zinc-100
                 transition-all duration-150 cursor-pointer"
    >
      <div className="flex items-center gap-1.5">
        {res.path.startsWith('box:') ? (
          <Server className="w-3 h-3 flex-shrink-0 text-sky-500" />
        ) : (
          <FolderGit2 className="w-3 h-3 flex-shrink-0 text-zinc-400" />
        )}
        <span className="text-sm text-zinc-900 truncate font-medium">{res.name}</span>
      </div>
      <div className="text-xs text-zinc-400 truncate ml-[18px]">{res.path}</div>
    </button>
  )

  const renderTaskResult = (
    res: { id: string; title: string; status: string; project_name: string },
  ) => (
    <button
      key={`task-${res.id}`}
      type="button"
      onClick={() => handleNavigateSearchResult(`/tasks/${res.id}`)}
      className="w-full text-left px-3 py-2 mb-0.5 rounded-xl hover:bg-zinc-100
                 transition-all duration-150 cursor-pointer"
    >
      <div className="flex items-center gap-1.5">
        <ListTodo className="w-3 h-3 flex-shrink-0 text-zinc-400" />
        <span className="text-sm text-zinc-900 truncate font-medium flex-1">{res.title}</span>
        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium flex-shrink-0 ${taskStatusBadgeClass(res.status)}`}>
          {res.status}
        </span>
      </div>
      {res.project_name && (
        <div className="text-xs text-zinc-400 truncate ml-[18px]">{res.project_name}</div>
      )}
    </button>
  )

  // In mixed mode (free-text + prefix), the API returns threads matching the
  // free-text part; we then narrow to those that also match prefix filters.
  // Threads not in the local list are dropped because we need their tags /
  // project_id / persona_name to evaluate prefix filters.
  const visibleSearchThreads = useMemo(() => {
    if (!searchResults) return []
    if (!hasPrefix) return searchResults.threads
    return searchResults.threads.filter(res => {
      const localThread = threadById.get(res.id)
      if (!localThread) return false
      return matchesPrefixFilters(localThread, parsedQuery, projectMap)
    })
  }, [searchResults, hasPrefix, threadById, parsedQuery, projectMap])

  // Search results view
  const searchResultsView = isSearching && (() => {
    if (searchLoading) {
      return <div className="px-3 py-4 text-sm text-zinc-400 text-center">Searching...</div>
    }
    if (!searchResults) return null

    const threadCount = visibleSearchThreads.length
    const messageCount = searchResults.messages.length
    const projectCount = searchResults.projects.length
    const taskCount = searchResults.tasks.length
    const totalCount = threadCount + messageCount + projectCount + taskCount

    if (totalCount === 0) {
      return <div className="px-3 py-4 text-sm text-zinc-400 text-center">No results found</div>
    }

    return (
      <>
        {threadCount > 0 && (
          <>
            {renderSectionHeader('Threads')}
            {visibleSearchThreads.slice(0, SIDEBAR_SECTION_LIMIT).map(renderThreadResult)}
            {renderMoreLink('Threads', threadCount)}
          </>
        )}
        {messageCount > 0 && (
          <>
            {renderSectionHeader('Messages')}
            {searchResults.messages.slice(0, SIDEBAR_SECTION_LIMIT).map(renderMessageResult)}
            {renderMoreLink('Messages', messageCount)}
          </>
        )}
        {projectCount > 0 && (
          <>
            {renderSectionHeader('Projects')}
            {searchResults.projects.slice(0, SIDEBAR_SECTION_LIMIT).map(renderProjectResult)}
            {renderMoreLink('Projects', projectCount)}
          </>
        )}
        {taskCount > 0 && (
          <>
            {renderSectionHeader('Tasks')}
            {searchResults.tasks.slice(0, SIDEBAR_SECTION_LIMIT).map(renderTaskResult)}
            {renderMoreLink('Tasks', taskCount)}
          </>
        )}
      </>
    )
  })()

  // Thread list view
  const threadListView = (
    <>
      {pinnedThreads.length > 0 && (
        <>
          <div className="px-3 py-1.5 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
            Pinned
          </div>
          {pinnedThreads.map(renderThread)}
        </>
      )}
      {pinnedThreads.length > 0 && regularThreads.length > 0 && (
        <div className="px-3 py-1.5 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
          Recent
        </div>
      )}
      {regularThreads.map(renderThread)}
      {archivedSectionVisible && archivedThreads.length > 0 && (
        <>
          <div className="my-1.5 mx-3 border-t border-zinc-100" />
          <div className="px-3 py-1.5 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
            Archived
          </div>
          {archivedThreads.map(renderThread)}
        </>
      )}
      {threads.length === 0 && (
        <div className="px-3 py-8 text-center text-zinc-400 text-sm">
          No conversations yet
        </div>
      )}
    </>
  )

  // Mobile full-screen mode
  if (mobile) {
    return (
      <>
        <div className="flex-1 flex flex-col w-full bg-zinc-50 overflow-hidden">
          <div className="px-4 pt-4 pb-2">
            <h1 className="text-xl font-bold text-zinc-900">Chats</h1>
          </div>
          {searchInput}
          {activeFiltersRow}
          <div className="flex-1 overflow-y-auto px-2 pb-4">
            {isSearching ? searchResultsView : threadListView}
          </div>
        </div>
        {toast && (
          <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-[100]
                          bg-red-600 text-white text-sm px-4 py-2.5 rounded-xl
                          shadow-lg shadow-red-600/20">
            {toast}
          </div>
        )}
      </>
    )
  }

  // Desktop sidebar mode
  return (
    <>
      <aside className="w-80 bg-zinc-50 border-r border-zinc-200 flex flex-col h-full flex-shrink-0">
        {/* Header */}
        <div className="p-3 pb-2 flex items-center justify-between relative" ref={personaDropdownRef}>
          <h1 className="text-base font-semibold text-zinc-900">Chats</h1>
          {!readOnly && (
            <div className="flex items-stretch rounded-lg overflow-hidden bg-amber-500 text-white
                            shadow-sm shadow-amber-500/20">
              <button
                onClick={() => { onNewThread(); clearSearch(); setPersonaDropdownOpen(false) }}
                className="flex items-center gap-1.5 px-2.5 py-1.5 text-sm font-medium
                           hover:bg-amber-400 transition-colors cursor-pointer"
                title="New chat"
              >
                <Plus className="w-4 h-4" />
                <span>New</span>
              </button>
              {personas.length > 0 && (
                <button
                  onClick={() => setPersonaDropdownOpen(!personaDropdownOpen)}
                  className={`flex items-center justify-center px-1.5 border-l border-amber-400/60
                             transition-colors cursor-pointer
                             ${personaDropdownOpen ? 'bg-amber-600' : 'hover:bg-amber-600'}`}
                  title="Start with persona"
                  aria-label="Start with persona"
                  aria-expanded={personaDropdownOpen}
                  aria-haspopup="menu"
                >
                  <ChevronDown className="w-3.5 h-3.5" />
                </button>
              )}
            </div>
          )}
          {personaDropdownOpen && (
            <div className="absolute left-3 right-3 top-full mt-1 z-50
                           bg-zinc-100 border border-zinc-200 rounded-xl
                           shadow-lg shadow-zinc-200/50 py-1 overflow-hidden">
              <button
                onClick={() => { onNewThread(); clearSearch(); setPersonaDropdownOpen(false) }}
                className="w-full flex items-center gap-2.5 px-3 py-2.5
                           text-sm text-zinc-700 hover:bg-zinc-50
                           transition-colors cursor-pointer"
              >
                <span className="text-base">💬</span>
                <span>Empty chat</span>
              </button>
              <div className="my-1 mx-2 border-t border-zinc-100" />
              {personas.map(persona => (
                <button
                  key={persona.id}
                  onClick={() => { onNewThread(persona.id); clearSearch(); setPersonaDropdownOpen(false) }}
                  className="w-full flex items-center gap-2.5 px-3 py-2.5
                             text-sm text-zinc-700 hover:bg-zinc-50
                             transition-colors cursor-pointer"
                >
                  <span className="text-base">{persona.icon}</span>
                  <div className="min-w-0 text-left">
                    <div className="truncate">{persona.name}</div>
                    {persona.default_model && (
                      <div className="text-[10px] text-zinc-400 truncate">{persona.default_model}</div>
                    )}
                  </div>
                </button>
              ))}
            </div>
          )}
        </div>

        {searchInput}
        {activeFiltersRow}

        {/* Thread list */}
        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {isSearching ? searchResultsView : threadListView}
        </div>

        {/* Footer */}
        <div className="p-3 border-t border-zinc-200 flex items-center justify-between gap-2">
          <button
            onClick={onToggleArchived}
            className={`flex items-center gap-1.5 px-2 py-1.5 text-xs rounded-lg
                       transition-all duration-150 cursor-pointer
                       ${showArchived
                         ? 'text-zinc-700 bg-zinc-200/60'
                         : 'text-zinc-400 hover:text-zinc-600'}`}
            title={showArchived ? 'Hide archived' : 'Show archived'}
          >
            <Archive className="w-3.5 h-3.5" />
            {showArchived ? 'Hide archived' : 'Archived'}
          </button>
          <BoxStatusBadge />
        </div>
      </aside>
      {toast && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-[100]
                        bg-red-600 text-white text-sm px-4 py-2.5 rounded-xl
                        shadow-lg shadow-red-600/20 animate-in fade-in slide-in-from-bottom-4">
          {toast}
        </div>
      )}
    </>
  )
}
