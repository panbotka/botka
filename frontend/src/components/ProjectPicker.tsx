import { useState, useEffect, useRef } from 'react'
import { FolderOpen, X } from 'lucide-react'
import type { Project } from '../types'

interface Props {
  projects: Project[]
  currentProjectId?: string
  onSelect: (projectId: string | null) => void
}

export default function ProjectPicker({ projects, currentProjectId, onSelect }: Props) {
  const [open, setOpen] = useState(false)
  const containerRef = useRef<HTMLDivElement>(null)

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

  const currentProject = projects.find(p => p.id === currentProjectId)
  const activeProjects = projects.filter(p => p.active)

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
                        rounded-xl shadow-xl shadow-zinc-200/50 py-1 overflow-hidden">
          <div className="max-h-60 overflow-y-auto">
            <button
              onClick={() => { onSelect(null); setOpen(false) }}
              className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer flex items-center gap-2
                         ${!currentProjectId ? 'text-amber-600 bg-amber-50' : 'text-zinc-500 hover:bg-zinc-50 hover:text-zinc-900'}`}
            >
              <X className="w-3.5 h-3.5" />
              No project
            </button>
            {activeProjects.map(p => (
              <button
                key={p.id}
                onClick={() => { onSelect(p.id); setOpen(false) }}
                className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer flex items-center gap-2
                           ${currentProjectId === p.id ? 'text-amber-600 bg-amber-50' : 'text-zinc-700 hover:bg-zinc-50'}`}
              >
                <FolderOpen className="w-3.5 h-3.5 flex-shrink-0" />
                <span className="truncate">{p.name}</span>
              </button>
            ))}
            {activeProjects.length === 0 && (
              <div className="px-3 py-2 text-sm text-zinc-400">No projects available</div>
            )}
          </div>
        </div>
      )}
    </div>
  )
}
