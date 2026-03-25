import { useState, useEffect, useCallback, useRef, useMemo } from 'react'
import { useLocation, useNavigate } from 'react-router-dom'
import type { Tag, Thread, Persona, Project } from '../types'
import { api } from '../api/client'
import { useIsMobile } from '../hooks/useIsMobile'
import { useProcesses } from '../hooks/useProcesses'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import ChatView from '../components/ChatView'
import ThreadSidebar from '../components/ThreadSidebar'
import ProcessBar from '../components/ProcessBar'
import ProjectPicker from '../components/ProjectPicker'
import { MessageSquare, ArrowLeft } from 'lucide-react'

function parseThreadIdFromPath(pathname: string): number | null {
  const match = pathname.match(/^\/chat\/(\d+)$/)
  return match ? Number(match[1]) : null
}

export default function ChatPage() {
  const isMobile = useIsMobile()
  const location = useLocation()
  const navigate = useNavigate()
  const [threads, setThreads] = useState<Thread[]>([])
  const [activeThreadId, setActiveThreadId] = useState<number | null>(
    () => parseThreadIdFromPath(window.location.pathname),
  )
  const [showArchived, setShowArchived] = useState(false)
  const [tags, setTags] = useState<Tag[]>([])
  const [selectedTagIds, setSelectedTagIds] = useState<number[]>([])
  const [personas, setPersonas] = useState<Persona[]>([])
  const [projects, setProjects] = useState<Project[]>([])
  const [selectedProjectId, setSelectedProjectId] = useState<string | null>(null)
  const [pendingStarterMessage, setPendingStarterMessage] = useState<string | null>(null)
  const [streamingThreadId, setStreamingThreadId] = useState<number | null>(null)
  const [threadNotFound, setThreadNotFound] = useState(false)

  const showArchivedRef = useRef(showArchived)
  useEffect(() => { showArchivedRef.current = showArchived }, [showArchived])

  const { processes, killProcess } = useProcesses()
  const activeProcessThreadIds = useMemo(
    () => new Set(processes.map(p => p.thread_id)),
    [processes],
  )

  // Sync activeThreadId from React Router location changes (back/forward)
  useEffect(() => {
    const id = parseThreadIdFromPath(location.pathname)
    setActiveThreadId(id)
    setThreadNotFound(false)
  }, [location.pathname])

  // Navigate to a thread
  const selectThread = useCallback((id: number | null, replace = false) => {
    setActiveThreadId(id)
    setThreadNotFound(false)
    const path = id ? `/chat/${id}` : '/chat'
    navigate(path, { replace })
  }, [navigate])

  // Validate initial thread ID from URL
  useEffect(() => {
    const urlThreadId = parseThreadIdFromPath(window.location.pathname)
    if (urlThreadId) {
      api.getThread(urlThreadId).catch(() => {
        setThreadNotFound(true)
        selectThread(null, true)
      })
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Auto-dismiss "not found" toast
  useEffect(() => {
    if (!threadNotFound) return
    const timer = setTimeout(() => setThreadNotFound(false), 3000)
    return () => clearTimeout(timer)
  }, [threadNotFound])

  // Update document title
  const activeThread = threads.find(t => t.id === activeThreadId)
  useDocumentTitle(activeThread?.title || 'Chat')

  // Data loading
  const threadsLoaded = useRef(false)

  const loadThreads = useCallback(async () => {
    try {
      const list = await api.fetchThreads(showArchivedRef.current)
      setThreads(list)
      threadsLoaded.current = true
    } catch { /* ignore */ }
  }, [])

  const loadTags = useCallback(async () => {
    try { setTags(await api.fetchTags()) } catch { /* ignore */ }
  }, [])

  const loadPersonas = useCallback(async () => {
    try { setPersonas(await api.fetchPersonas()) } catch { /* ignore */ }
  }, [])

  const loadProjects = useCallback(async () => {
    try { setProjects(await api.fetchProjects()) } catch { /* ignore */ }
  }, [])

  useEffect(() => {
    loadThreads()
    loadTags()
    loadPersonas()
    loadProjects()
  }, [loadThreads, loadTags, loadPersonas, loadProjects, showArchived])

  useRefreshOnFocus(loadThreads)

  // On desktop, auto-select first thread if none selected
  useEffect(() => {
    if (!threadsLoaded.current) return
    if (isMobile) return
    const urlThreadId = parseThreadIdFromPath(window.location.pathname)
    if (!urlThreadId && threads.length > 0) {
      selectThread(threads[0]!.id, true)
    }
  }, [threads, selectThread, isMobile])

  // Thread operations
  const handleNewThread = async (personaId?: number) => {
    try {
      const thread = await api.createThread(personaId ? { persona_id: personaId } : {})
      setThreads(prev => [thread, ...prev])
      selectThread(thread.id)

      if (personaId) {
        const persona = personas.find(p => p.id === personaId)
        if (persona?.starter_message) {
          setPendingStarterMessage(persona.starter_message)
        }
      }
    } catch { /* ignore */ }
  }

  const handleTitleUpdate = useCallback((threadId: number, title: string) => {
    setThreads(prev => prev.map(t => t.id === threadId ? { ...t, title } : t))
  }, [])

  const handleThreadsChange = useCallback(() => {
    loadThreads()
  }, [loadThreads])

  const handleProjectChange = useCallback(async (threadId: number, projectId: string | null) => {
    try {
      await api.updateThreadProject(threadId, projectId)
      setThreads(prev => prev.map(t =>
        t.id === threadId ? { ...t, project_id: projectId ?? undefined } : t,
      ))
    } catch { /* ignore */ }
  }, [])

  const handleToggleArchived = useCallback(() => {
    setShowArchived(prev => !prev)
  }, [])

  // Mobile navigation
  const handleMobileBack = useCallback(() => {
    selectThread(null)
  }, [selectThread])

  // Shared sidebar props
  const sidebarProps = {
    threads,
    activeThreadId,
    onSelectThread: (id: number) => selectThread(id),
    onNewThread: handleNewThread,
    onThreadsChange: handleThreadsChange,
    showArchived,
    onToggleArchived: handleToggleArchived,
    tags,
    selectedTagIds,
    onToggleTagFilter: (tagId: number) => {
      setSelectedTagIds(prev =>
        prev.includes(tagId) ? prev.filter(id => id !== tagId) : [...prev, tagId],
      )
    },
    onClearTagFilter: () => setSelectedTagIds([]),
    personas,
    projects,
    selectedProjectId,
    onSelectProject: setSelectedProjectId,
    streamingThreadId,
    activeProcessThreadIds,
  }

  const showMobileChat = isMobile && activeThreadId !== null

  // Mobile layout
  if (isMobile) {
    return (
      <div className="h-full flex flex-col">
        {showMobileChat ? (
          <div className="flex-1 flex flex-col min-w-0">
            {/* Mobile chat header */}
            <header className="flex items-center gap-3 px-4 h-12 bg-zinc-50 border-b border-zinc-200 sticky top-0 z-10 flex-shrink-0">
              <button
                onClick={handleMobileBack}
                className="text-zinc-500 hover:text-zinc-800 cursor-pointer transition-colors -ml-1 min-w-[44px] min-h-[44px] flex items-center justify-center"
              >
                <ArrowLeft className="w-5 h-5" />
              </button>
              <div className="flex-1 min-w-0">
                <span className="text-sm text-zinc-900 font-medium truncate block">
                  {activeThread?.persona_icon && <span className="mr-1">{activeThread.persona_icon}</span>}
                  {activeThread?.title || 'New conversation'}
                </span>
                {activeThread?.persona_name && (
                  <span className="text-[11px] text-zinc-400 truncate block">
                    {activeThread.persona_name}
                  </span>
                )}
              </div>
              {activeThread && (
                <>
                  <ProjectPicker
                    projects={projects}
                    currentProjectId={activeThread.project_id}
                    onSelect={(projectId) => handleProjectChange(activeThread.id, projectId)}
                  />
                  <span className="text-[11px] text-zinc-500 bg-zinc-100 px-2 py-0.5 rounded-md flex-shrink-0">
                    {activeThread.model || 'Default'}
                  </span>
                </>
              )}
            </header>
            <ProcessBar processes={processes} onKill={killProcess} />
            <ChatView
              threadId={activeThreadId}
              thread={activeThread}
              onTitleUpdate={handleTitleUpdate}
              onNewThread={() => handleNewThread()}
              onOpenSearch={handleMobileBack}
              pendingStarterMessage={pendingStarterMessage}
              onStarterMessageConsumed={() => setPendingStarterMessage(null)}
              onStreamingChange={setStreamingThreadId}
            />
          </div>
        ) : (
          <>
            <ThreadSidebar
              {...sidebarProps}
              mobile
            />
            {/* FAB: New Chat */}
            <button
              onClick={() => handleNewThread()}
              className="fixed right-4 bottom-20 z-30 w-14 h-14 rounded-full
                         bg-amber-500 hover:bg-amber-400 active:bg-amber-600
                         text-white shadow-lg shadow-amber-500/20
                         flex items-center justify-center cursor-pointer transition-colors"
              style={{ marginBottom: 'env(safe-area-inset-bottom, 0px)' }}
            >
              <MessageSquare className="w-6 h-6" />
            </button>
          </>
        )}

        {threadNotFound && (
          <div className="fixed bottom-20 left-1/2 -translate-x-1/2 z-50 animate-message-in">
            <div className="bg-zinc-800 text-white text-sm px-4 py-2.5 rounded-xl shadow-lg">
              Conversation not found
            </div>
          </div>
        )}
      </div>
    )
  }

  // Desktop layout
  return (
    <div className="h-full flex">
      <ThreadSidebar {...sidebarProps} />

      <div className="flex-1 flex flex-col min-w-0">
        {activeThreadId ? (
          <>
            {/* Desktop chat header */}
            <header className="flex items-center gap-3 px-4 h-12 bg-zinc-50 border-b border-zinc-200 sticky top-0 z-10 flex-shrink-0">
              <div className="flex-1 min-w-0">
                <span className="text-sm text-zinc-900 font-medium truncate block">
                  {activeThread?.persona_icon && <span className="mr-1">{activeThread.persona_icon}</span>}
                  {activeThread?.title || 'New conversation'}
                </span>
                {activeThread?.persona_name && (
                  <span className="text-[11px] text-zinc-400 truncate block">
                    {activeThread.persona_name}
                  </span>
                )}
              </div>
              {activeThread && (
                <>
                  <ProjectPicker
                    projects={projects}
                    currentProjectId={activeThread.project_id}
                    onSelect={(projectId) => handleProjectChange(activeThread.id, projectId)}
                  />
                  <span className="text-[11px] text-zinc-500 bg-zinc-100 px-2 py-0.5 rounded-md flex-shrink-0">
                    {activeThread.model || 'Default'}
                  </span>
                </>
              )}
            </header>
            <ProcessBar processes={processes} onKill={killProcess} />
            <ChatView
              threadId={activeThreadId}
              thread={activeThread}
              onTitleUpdate={handleTitleUpdate}
              onNewThread={() => handleNewThread()}
              onOpenSearch={() => {}}
              pendingStarterMessage={pendingStarterMessage}
              onStarterMessageConsumed={() => setPendingStarterMessage(null)}
              onStreamingChange={setStreamingThreadId}
            />
          </>
        ) : (
          /* Empty state — no thread selected */
          <div className="flex-1 flex items-center justify-center">
            <div className="text-center">
              <div className="w-16 h-16 mx-auto mb-4 rounded-2xl bg-zinc-100 flex items-center justify-center">
                <MessageSquare className="w-8 h-8 text-zinc-400" />
              </div>
              <h2 className="text-lg font-medium text-zinc-700 mb-1">Select a conversation</h2>
              <p className="text-sm text-zinc-400 mb-4">
                Pick a thread from the sidebar or start a new chat
              </p>
              <button
                onClick={() => handleNewThread()}
                className="px-4 py-2 bg-zinc-900 text-white text-sm font-medium rounded-lg
                           hover:bg-zinc-800 transition-colors cursor-pointer"
              >
                New Chat
              </button>
            </div>
          </div>
        )}
      </div>

      {threadNotFound && (
        <div className="fixed bottom-6 left-1/2 -translate-x-1/2 z-50 animate-message-in">
          <div className="bg-zinc-800 text-white text-sm px-4 py-2.5 rounded-xl shadow-lg">
            Conversation not found
          </div>
        </div>
      )}
    </div>
  )
}
