import { useState, useEffect, useRef, useCallback, useMemo } from 'react'
import type { Persona, Tag, Thread, Project, SearchResult } from '../types'
import { formatDate as formatDateOnly } from '../utils/dateFormat'
import { api, searchMessages, ApiError } from '../api/client'
import { downloadExport } from '../utils/exportThread'
import { clearDraft } from './ChatInput'
import { useStreamingThreadIds } from '../context/SSEContext'
import ModelPicker from './ModelPicker'
import ThreadSourcesEditor from './ThreadSourcesEditor'
import CustomContextEditor from './CustomContextEditor'
import SignalBridgeEditor from './SignalBridgeEditor'
import {
  Plus, Search, Pin, Archive, MoreVertical, Pencil,
  Trash2, Download, Cpu, Tag as TagIcon, ChevronRight,
  X, ChevronDown, FolderGit2, Globe, FileText, Palette, MessageSquare,
} from 'lucide-react'
import { THREAD_COLORS } from '../utils/threadColors'

interface Props {
  threads: Thread[]
  activeThreadId: number | null
  onSelectThread: (id: number) => void
  onNewThread: (personaId?: number) => void
  onThreadsChange: () => void
  showArchived: boolean
  onToggleArchived: () => void
  tags: Tag[]
  selectedTagIds: number[]
  onToggleTagFilter: (tagId: number) => void
  onClearTagFilter: () => void
  personas: Persona[]
  projects: Project[]
  selectedProjectId: string | null
  onSelectProject: (id: string | null) => void
  activeProcessThreadIds: Set<number>
  mobile?: boolean
  readOnly?: boolean
}

export default function ThreadSidebar({
  threads,
  activeThreadId,
  onSelectThread,
  onNewThread,
  onThreadsChange,
  showArchived,
  onToggleArchived,
  tags,
  selectedTagIds,
  onToggleTagFilter,
  onClearTagFilter,
  personas,
  projects,
  selectedProjectId,
  onSelectProject,
  activeProcessThreadIds,
  mobile,
  readOnly,
}: Props) {
  const streamingThreadIds = useStreamingThreadIds()
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editTitle, setEditTitle] = useState('')
  const [menuOpenId, setMenuOpenId] = useState<number | null>(null)
  const [modelPickerThread, setModelPickerThread] = useState<Thread | null>(null)
  const [tagMenuThreadId, setTagMenuThreadId] = useState<number | null>(null)
  const [colorMenuThreadId, setColorMenuThreadId] = useState<number | null>(null)
  const [sourcesThreadId, setSourcesThreadId] = useState<number | null>(null)
  const [customContextThread, setCustomContextThread] = useState<Thread | null>(null)
  const [signalBridgeThreadId, setSignalBridgeThreadId] = useState<number | null>(null)
  const [personaDropdownOpen, setPersonaDropdownOpen] = useState(false)
  const personaDropdownRef = useRef<HTMLDivElement>(null)
  const menuRef = useRef<HTMLDivElement>(null)
  const [toast, setToast] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<SearchResult[] | null>(null)
  const [searchLoading, setSearchLoading] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const debounceRef = useRef<ReturnType<typeof setTimeout>>(undefined)

  // Close menu on outside click or Escape
  useEffect(() => {
    if (menuOpenId === null) return
    const handleClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setMenuOpenId(null)
        setTagMenuThreadId(null)
        setColorMenuThreadId(null)
      }
    }
    const handleKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        setMenuOpenId(null)
        setTagMenuThreadId(null)
        setColorMenuThreadId(null)
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

  // Debounced message search
  useEffect(() => {
    if (debounceRef.current) clearTimeout(debounceRef.current)
    const q = searchQuery.trim()
    if (q.length < 2) {
      setSearchResults(null)
      setSearchLoading(false)
      return
    }
    setSearchLoading(true)
    debounceRef.current = setTimeout(async () => {
      try {
        const results = await searchMessages(q)
        setSearchResults(results)
      } catch {
        setSearchResults([])
      }
      setSearchLoading(false)
    }, 300)
    return () => {
      if (debounceRef.current) clearTimeout(debounceRef.current)
    }
  }, [searchQuery])

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

  const handleToggleThreadTag = async (thread: Thread, tagId: number) => {
    const currentTagIds = (thread.tags || []).map(t => t.id)
    const newTagIds = currentTagIds.includes(tagId)
      ? currentTagIds.filter(id => id !== tagId)
      : [...currentTagIds, tagId]
    try {
      await api.updateThreadTags(thread.id, newTagIds)
      onThreadsChange()
    } catch { /* ignore */ }
  }

  const handleChangeModel = async (threadId: number, model: string) => {
    try {
      await api.updateModel(threadId, model)
      onThreadsChange()
    } catch { /* ignore */ }
    setModelPickerThread(null)
  }

  const handleChangeColor = async (threadId: number, color: string) => {
    try {
      await api.updateThread(threadId, { color } as Partial<Thread>)
      onThreadsChange()
    } catch { /* ignore */ }
    setColorMenuThreadId(null)
    setMenuOpenId(null)
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

  const handleSelectSearchResult = (threadId: number) => {
    onSelectThread(threadId)
    clearSearch()
  }

  const isSearching = searchQuery.trim().length >= 2
  const hasTagFilter = selectedTagIds.length > 0
  const hasProjectFilter = selectedProjectId !== null

  const matchesFilters = useCallback((thread: Thread) => {
    if (hasTagFilter && !(thread.tags || []).some(t => selectedTagIds.includes(t.id))) return false
    if (hasProjectFilter && thread.project_id !== selectedProjectId) return false
    return true
  }, [hasTagFilter, selectedTagIds, hasProjectFilter, selectedProjectId])

  const projectMap = useMemo(() => new Map(projects.map(p => [p.id, p])), [projects])

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
                <span className="text-[10px] text-zinc-400 flex items-center gap-0.5">
                  <FolderGit2 className="w-2.5 h-2.5" />
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
                  <button
                    onClick={() => { setModelPickerThread(thread); setMenuOpenId(null) }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <Cpu className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    <div className="flex-1 min-w-0 text-left">
                      <div>Change model</div>
                      <div className="text-[11px] text-zinc-400 truncate">{thread.model || 'Default'}</div>
                    </div>
                  </button>
                  <button
                    onClick={() => { setSourcesThreadId(thread.id); setMenuOpenId(null) }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <Globe className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    Sources
                  </button>
                  <button
                    onClick={() => { setCustomContextThread(thread); setMenuOpenId(null) }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <FileText className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                    <div className="flex-1 min-w-0 text-left">
                      <div>Custom Context</div>
                      {thread.custom_context && (
                        <div className="text-[11px] text-zinc-400 truncate">
                          {thread.custom_context.length.toLocaleString()} chars
                        </div>
                      )}
                    </div>
                  </button>
                  <button
                    onClick={() => { setSignalBridgeThreadId(thread.id); setMenuOpenId(null) }}
                    className="w-full flex items-center gap-3 px-3 py-2
                               text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                  >
                    <MessageSquare className={`w-4 h-4 flex-shrink-0 ${thread.signal_bridge_active ? 'text-emerald-500' : 'text-zinc-400'}`} />
                    <span className="flex-1 text-left">Signal Bridge</span>
                    {thread.signal_bridge_active && (
                      <span className="text-[10px] text-emerald-600">Active</span>
                    )}
                  </button>
                  <div>
                    <button
                      onClick={() => setColorMenuThreadId(colorMenuThreadId === thread.id ? null : thread.id)}
                      className="w-full flex items-center gap-3 px-3 py-2
                                 text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                    >
                      <Palette className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                      <span className="flex-1 text-left">Color</span>
                      {thread.color && (
                        <span
                          className="w-3 h-3 rounded-full flex-shrink-0"
                          style={{ backgroundColor: THREAD_COLORS.find(c => c.key === thread.color)?.swatch }}
                        />
                      )}
                      <ChevronRight className={`w-3 h-3 text-zinc-400 transition-transform ${colorMenuThreadId === thread.id ? 'rotate-90' : ''}`} />
                    </button>
                    {colorMenuThreadId === thread.id && (
                      <div className="px-3 pb-2 pt-1">
                        <div className="grid grid-cols-4 gap-1.5">
                          {THREAD_COLORS.map(color => {
                            const isActive = (thread.color || '') === color.key
                            return (
                              <button
                                key={color.key || 'none'}
                                onClick={() => handleChangeColor(thread.id, color.key)}
                                className={`w-8 h-8 rounded-lg border-2 transition-all cursor-pointer flex items-center justify-center
                                  ${isActive ? 'border-zinc-500 scale-110' : 'border-zinc-200 hover:border-zinc-400'}`}
                                style={{ backgroundColor: color.key ? color.swatch : undefined }}
                                title={color.label}
                              >
                                {color.key === '' && (
                                  <X className="w-3.5 h-3.5 text-zinc-400" />
                                )}
                              </button>
                            )
                          })}
                        </div>
                      </div>
                    )}
                  </div>
                  {tags.length > 0 && (
                    <div>
                      <button
                        onClick={() => setTagMenuThreadId(tagMenuThreadId === thread.id ? null : thread.id)}
                        className="w-full flex items-center gap-3 px-3 py-2
                                   text-sm text-zinc-700 hover:bg-zinc-50 transition-colors cursor-pointer"
                      >
                        <TagIcon className="w-4 h-4 flex-shrink-0 text-zinc-400" />
                        <span className="flex-1 text-left">Tags</span>
                        <ChevronRight className={`w-3 h-3 text-zinc-400 transition-transform ${tagMenuThreadId === thread.id ? 'rotate-90' : ''}`} />
                      </button>
                      {tagMenuThreadId === thread.id && (
                        <div className="pl-7 pb-1">
                          {tags.map(tag => {
                            const isActive = (thread.tags || []).some(t => t.id === tag.id)
                            return (
                              <button
                                key={tag.id}
                                onClick={() => handleToggleThreadTag(thread, tag.id)}
                                className="w-full flex items-center gap-2.5 px-3 py-1.5
                                           text-sm text-zinc-700 hover:bg-zinc-50
                                           transition-colors cursor-pointer rounded-lg"
                              >
                                <span
                                  className="w-3 h-3 rounded-full flex-shrink-0 border-2"
                                  style={{
                                    backgroundColor: isActive ? tag.color : 'transparent',
                                    borderColor: tag.color,
                                  }}
                                />
                                <span className="truncate">{tag.name}</span>
                              </button>
                            )
                          })}
                        </div>
                      )}
                    </div>
                  )}
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

  // Tag + project filter bar
  const filterBar = (tags.length > 0 || projects.length > 0) && !isSearching && (
    <div className="px-3 pb-2 flex gap-1.5 overflow-x-auto scrollbar-hide">
      <button
        onClick={() => { onClearTagFilter(); onSelectProject(null) }}
        className={`flex-shrink-0 px-2.5 py-1 text-xs rounded-lg border transition-all duration-150 cursor-pointer
          ${!hasTagFilter && !hasProjectFilter
            ? 'bg-zinc-200/60 border-zinc-300 text-zinc-800'
            : 'bg-transparent border-zinc-200 text-zinc-500 hover:text-zinc-700 hover:border-zinc-300'
          }`}
      >
        All
      </button>
      {tags.map(tag => {
        const isSelected = selectedTagIds.includes(tag.id)
        return (
          <button
            key={tag.id}
            onClick={() => onToggleTagFilter(tag.id)}
            className={`flex-shrink-0 flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg border transition-all duration-150 cursor-pointer
              ${isSelected
                ? 'border-zinc-300 text-zinc-800'
                : 'bg-transparent border-zinc-200 text-zinc-500 hover:text-zinc-700 hover:border-zinc-300'
              }`}
            style={isSelected ? { backgroundColor: tag.color + '20', borderColor: tag.color + '60' } : {}}
          >
            <span
              className="w-2 h-2 rounded-full flex-shrink-0"
              style={{ backgroundColor: tag.color }}
            />
            {tag.name}
          </button>
        )
      })}
      {projects.filter(p => p.active).map(project => (
        <button
          key={project.id}
          onClick={() => onSelectProject(selectedProjectId === project.id ? null : project.id)}
          className={`flex-shrink-0 flex items-center gap-1.5 px-2.5 py-1 text-xs rounded-lg border transition-all duration-150 cursor-pointer
            ${selectedProjectId === project.id
              ? 'bg-zinc-200/60 border-zinc-300 text-zinc-800'
              : 'bg-transparent border-zinc-200 text-zinc-500 hover:text-zinc-700 hover:border-zinc-300'
            }`}
        >
          <FolderGit2 className="w-3 h-3" />
          {project.name}
        </button>
      ))}
    </div>
  )

  // Search results view
  const searchResultsView = isSearching && (
    searchLoading ? (
      <div className="px-3 py-4 text-sm text-zinc-400 text-center">Searching...</div>
    ) : searchResults && searchResults.length === 0 ? (
      <div className="px-3 py-4 text-sm text-zinc-400 text-center">No results found</div>
    ) : searchResults?.map((result) => (
      <div key={result.thread.id} className="mb-1">
        <button
          onClick={() => handleSelectSearchResult(result.thread.id)}
          className="w-full text-left px-3 py-2.5 rounded-xl hover:bg-zinc-100
                     transition-all duration-150 cursor-pointer"
        >
          <div className="text-sm text-zinc-900 truncate font-medium">
            {result.thread.title}
          </div>
          {result.matches.slice(0, 2).map((match) => (
            <div
              key={match.message_id}
              className="mt-1 text-xs text-zinc-500 line-clamp-2
                         [&_mark]:bg-amber-100 [&_mark]:text-amber-800 [&_mark]:rounded-sm [&_mark]:px-0.5"
              dangerouslySetInnerHTML={{ __html: match.snippet }}
            />
          ))}
        </button>
      </div>
    ))
  )

  // Thread list view
  const threadListView = (
    <>
      {pinnedThreads.length > 0 && (
        <>
          <div className="px-3 py-1.5 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
            Pinned
          </div>
          {pinnedThreads.map(renderThread)}
          {regularThreads.length > 0 && (
            <div className="my-1.5 mx-3 border-t border-zinc-100" />
          )}
        </>
      )}
      {regularThreads.map(renderThread)}
      {showArchived && archivedThreads.length > 0 && (
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

  // Model picker modal
  const modelPickerModal = modelPickerThread && (
    <>
      <div className="fixed inset-0 bg-black/20 backdrop-blur-sm z-[60]"
           onClick={() => setModelPickerThread(null)} />
      <div className="fixed left-1/2 top-1/2 -translate-x-1/2 -translate-y-1/2 z-[60]
                      w-full max-w-sm bg-zinc-100 border border-zinc-200
                      rounded-2xl shadow-2xl p-5">
        <h3 className="text-sm font-medium text-zinc-900 mb-1">Change model</h3>
        <p className="text-xs text-zinc-500 mb-3 truncate">
          {modelPickerThread.title}
        </p>
        <ModelPicker
          value={modelPickerThread.model}
          onChange={(model) => handleChangeModel(modelPickerThread.id, model)}
        />
        <button
          onClick={() => setModelPickerThread(null)}
          className="mt-3 text-sm text-zinc-500 hover:text-zinc-700 transition-colors cursor-pointer"
        >
          Cancel
        </button>
      </div>
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
          {filterBar}
          <div className="flex-1 overflow-y-auto px-2 pb-4">
            {isSearching ? searchResultsView : threadListView}
          </div>
        </div>
        {modelPickerModal}
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
            <div className="flex items-center gap-0.5">
              <button
                onClick={() => { onNewThread(); clearSearch(); setPersonaDropdownOpen(false) }}
                className="w-8 h-8 flex items-center justify-center rounded-lg
                           text-zinc-500 hover:text-zinc-800 hover:bg-zinc-200/60
                           transition-all cursor-pointer"
                title="New chat"
              >
                <Plus className="w-5 h-5" />
              </button>
              {personas.length > 0 && (
                <button
                  onClick={() => setPersonaDropdownOpen(!personaDropdownOpen)}
                  className={`w-8 h-8 flex items-center justify-center rounded-lg
                             transition-all cursor-pointer
                             ${personaDropdownOpen
                               ? 'bg-zinc-200/60 text-zinc-800'
                               : 'text-zinc-500 hover:text-zinc-800 hover:bg-zinc-200/60'}`}
                  title="Start with persona"
                >
                  <ChevronDown className="w-4 h-4" />
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
        {filterBar}

        {/* Thread list */}
        <div className="flex-1 overflow-y-auto px-2 pb-2">
          {isSearching ? searchResultsView : threadListView}
        </div>

        {/* Footer */}
        <div className="p-3 border-t border-zinc-200 flex items-center">
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
        </div>
      </aside>
      {modelPickerModal}
      {sourcesThreadId && (
        <ThreadSourcesEditor
          threadId={sourcesThreadId}
          onClose={() => setSourcesThreadId(null)}
        />
      )}
      {customContextThread && (
        <CustomContextEditor
          threadId={customContextThread.id}
          initialContent={customContextThread.custom_context || ''}
          onClose={() => setCustomContextThread(null)}
          onSaved={onThreadsChange}
        />
      )}
      {signalBridgeThreadId !== null && (
        <SignalBridgeEditor
          threadId={signalBridgeThreadId}
          onClose={() => setSignalBridgeThreadId(null)}
          onChange={onThreadsChange}
        />
      )}
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
