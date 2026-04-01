import { useState, useEffect, useRef } from 'react'
import { FolderOpen, X, Search } from 'lucide-react'
import type { Project } from '../types'

interface Props {
  projects: Project[]
  currentProjectId?: string
  onSelect: (projectId: string | null) => void
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

export default function ProjectPicker({ projects, currentProjectId, onSelect }: Props) {
  const [open, setOpen] = useState(false)
  const [search, setSearch] = useState('')
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

  const currentProject = projects.find(p => p.id === currentProjectId)
  const activeProjects = projects.filter(p => p.active)

  // Filter and sort by fuzzy match score
  const query = search.trim()
  const filteredProjects = query
    ? activeProjects
        .map(p => ({ project: p, score: fuzzyScore(query, p.name) }))
        .filter(r => r.score >= 0)
        .sort((a, b) => b.score - a.score)
        .map(r => r.project)
    : activeProjects

  return (
    <div className="relative" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        className="flex items-center gap-1 text-[11px] px-2 py-0.5 rounded-md transition-colors cursor-pointer flex-shrink-0
                   text-zinc-500 bg-zinc-100 hover:bg-zinc-200"
      >
        <FolderOpen className="w-3 h-3" />
        <span className="truncate max-w-[120px]">
          {currentProject?.name || 'No project'}
        </span>
      </button>

      {open && (
        <div className="absolute right-0 z-50 mt-1 w-56 bg-zinc-100 border border-zinc-200
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
          <div className="max-h-60 overflow-y-auto">
            <button
              onClick={() => { onSelect(null); setOpen(false) }}
              className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer flex items-center gap-2
                         ${!currentProjectId ? 'text-amber-600 bg-amber-50' : 'text-zinc-500 hover:bg-zinc-50 dark:hover:bg-zinc-200 hover:text-zinc-900'}`}
            >
              <X className="w-3.5 h-3.5" />
              No project
            </button>
            {filteredProjects.map(p => (
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
            {query && filteredProjects.length === 0 && (
              <div className="px-3 py-2 text-sm text-zinc-400">No matches</div>
            )}
            {!query && activeProjects.length === 0 && (
              <div className="px-3 py-2 text-sm text-zinc-400">No projects available</div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
