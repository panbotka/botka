import { useState, useEffect, useRef } from 'react'
import { FolderOpen, X, Search, Server, AlertTriangle, RefreshCw } from 'lucide-react'
import type { Project } from '../types'
import { api } from '../api/client'

interface Props {
  projects: Project[]
  currentProjectId?: string
  onSelect: (projectId: string | null) => void
  /** Triggered after a successful box project refresh so parents can reload their project list. */
  onBoxProjectsChanged?: () => void
}

/** Returns a score for how well `query` fuzzy-matches `target` (case-insensitive).
 *  Returns -1 if no match. Higher score = better match.
 *  Scoring: exact prefix > substring > fuzzy (non-contiguous). */
function fuzzyScore(query: string, target: string): number {
  const q = query.toLowerCase()
  const t = target.toLowerCase()
  if (q.length === 0) return 0

  // Exact prefix match — best score
  if (t.startsWith(q)) return 3

  // Substring match
  if (t.includes(q)) return 2

  // Fuzzy: all query chars appear in order
  let qi = 0
  for (let ti = 0; ti < t.length && qi < q.length; ti++) {
    if (t[ti] === q[qi]) qi++
  }
  if (qi === q.length) return 1

  return -1
}

/** True if the project path identifies a directory on the remote Box host. */
export function isBoxProject(p: Project): boolean {
  return p.path.startsWith('box:')
}

export default function ProjectPicker({ projects, currentProjectId, onSelect, onBoxProjectsChanged }: Props) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
  const [boxOnline, setBoxOnline] = useState<boolean | null>(null)
  const [boxNote, setBoxNote] = useState<string | null>(null)
  const [refreshing, setRefreshing] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (!open) return
    const handle = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [open])

  // Auto-focus the search input and clear search when dropdown opens/closes
  useEffect(() => {
    if (open) {
      // Small delay to ensure the input is rendered
      requestAnimationFrame(() => inputRef.current?.focus())
    } else {
      setSearch('')
    }
  }, [open])

  // Refresh box projects on open so the remote list reflects reality.
  const refreshBox = async () => {
    setRefreshing(true)
    try {
      const resp = await api.fetchBoxProjects()
      setBoxOnline(resp.online)
      setBoxNote(resp.online ? null : (resp.note || 'Box is offline'))
      if (resp.online) onBoxProjectsChanged?.()
    } catch {
      setBoxOnline(false)
      setBoxNote('Failed to reach Box')
    } finally {
      setRefreshing(false)
    }
  }

  useEffect(() => {
    if (!open) return
    // Kick off a single refresh per open. Don't re-trigger on re-renders.
    refreshBox()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [open])

  const currentProject = projects.find(p => p.id === currentProjectId)
  const currentIsBox = currentProject ? isBoxProject(currentProject) : false
  const activeProjects = projects.filter(p => p.active)

  // Filter and sort by fuzzy match score, preserving local vs remote grouping.
  const query = search.trim()
  const rank = (p: Project): number => query ? fuzzyScore(query, p.name) : 0
  const filteredProjects = activeProjects
    .map(p => ({ project: p, score: rank(p) }))
    .filter(r => r.score >= 0)
    .sort((a, b) => b.score - a.score)
    .map(r => r.project)

  const localProjects = filteredProjects.filter(p => !isBoxProject(p))
  const remoteProjects = filteredProjects.filter(p => isBoxProject(p))

  return (
    <div className="relative" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-md transition-colors cursor-pointer flex-shrink-0
                   text-zinc-500 bg-zinc-100 hover:bg-zinc-200"
      >
        {currentIsBox ? <Server className="w-3 h-3" /> : <FolderOpen className="w-3 h-3" />}
        <span className="truncate max-w-[120px]">
          {currentProject?.name || 'No project'}
        </span>
      </button>

      {open && (
        <div className="absolute right-0 z-50 mt-1 w-64 bg-zinc-100 border border-zinc-200
                        rounded-xl shadow-xl shadow-black/10 py-1 overflow-hidden">
          <div className="px-2 py-1.5">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 -translate-y-1/2 w-3.5 h-3.5 text-zinc-400" />
              <input
                ref={inputRef}
                type="text"
                value={search}
                onChange={e => setSearch(e.target.value)}
                placeholder="Search..."
                className="w-full pl-7 pr-2 py-1 text-sm bg-white dark:bg-zinc-200 border border-zinc-200 rounded-lg
                           text-zinc-700 placeholder:text-zinc-400 outline-none focus:border-amber-300
                           focus:ring-1 focus:ring-amber-200"
              />
            </div>
          </div>
          <div className="max-h-72 overflow-y-auto">
            <button
              onClick={() => { onSelect(null); setOpen(false) }}
              className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer flex items-center gap-2
                         ${!currentProjectId ? 'text-amber-600 bg-amber-50' : 'text-zinc-500 hover:bg-zinc-50 dark:hover:bg-zinc-200 hover:text-zinc-900'}`}
            >
              <X className="w-3.5 h-3.5" />
              No project
            </button>

            {/* Local (RPi) section */}
            <SectionHeader label="Local (RPi)" icon={<FolderOpen className="w-3 h-3" />} />
            {localProjects.map(p => (
              <button
                key={p.id}
                onClick={() => { onSelect(p.id); setOpen(false) }}
                className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer flex items-center gap-2
                           ${currentProjectId === p.id ? 'text-amber-600 bg-amber-50' : 'text-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-200'}`}
              >
                <FolderOpen className="w-3.5 h-3.5 flex-shrink-0" />
                <span className="truncate">{p.name}</span>
              </button>
            ))}
            {localProjects.length === 0 && (
              <div className="px-3 py-1.5 text-xs text-zinc-400">
                {query ? 'No local matches' : 'No local projects'}
              </div>
            )}

            {/* Remote (Box) section */}
            <SectionHeader
              label="Remote (Box)"
              icon={<Server className="w-3 h-3" />}
              action={
                <button
                  onClick={(e) => { e.stopPropagation(); refreshBox() }}
                  disabled={refreshing}
                  title="Refresh Box projects"
                  className="p-0.5 text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer"
                >
                  <RefreshCw className={`w-3 h-3 ${refreshing ? 'animate-spin' : ''}`} />
                </button>
              }
            />
            {boxOnline === false && (
              <div className="px-3 py-1.5 text-xs text-amber-600 flex items-start gap-1.5">
                <AlertTriangle className="w-3 h-3 flex-shrink-0 mt-0.5" />
                <span>{boxNote || 'Box is offline'}</span>
              </div>
            )}
            {remoteProjects.map(p => (
              <button
                key={p.id}
                onClick={() => { onSelect(p.id); setOpen(false) }}
                className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer flex items-center gap-2
                           ${currentProjectId === p.id ? 'text-amber-600 bg-amber-50' : 'text-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-200'}`}
              >
                <Server className="w-3.5 h-3.5 flex-shrink-0 text-sky-500" />
                <span className="truncate">{p.name}</span>
              </button>
            ))}
            {remoteProjects.length === 0 && boxOnline !== false && (
              <div className="px-3 py-1.5 text-xs text-zinc-400">
                {query ? 'No remote matches' : (refreshing ? 'Loading…' : 'No remote projects')}
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

function SectionHeader({ label, icon, action }: { label: string; icon: React.ReactNode; action?: React.ReactNode }) {
  return (
    <div className="px-3 py-1 mt-1 flex items-center justify-between text-[10px] uppercase tracking-wide text-zinc-400 border-t border-zinc-200 first:border-t-0">
      <span className="flex items-center gap-1">
        {icon}
        {label}
      </span>
      {action}
    </div>
  )
}
