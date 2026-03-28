import { useState, useEffect, useCallback, useRef } from 'react'
import { Plus, Trash2, Globe, GripVertical } from 'lucide-react'
import type { ThreadSource } from '../types'
import {
  fetchThreadSources,
  createThreadSource,
  updateThreadSource,
  deleteThreadSource,
  reorderThreadSources,
} from '../api/client'

interface Props {
  threadId: number
  onClose: () => void
}

const isTempId = (id: number) => id < 0

export default function ThreadSourcesEditor({ threadId, onClose }: Props) {
  const [sources, setSources] = useState<ThreadSource[]>([])
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState<number | null>(null)
  const [dragIdx, setDragIdx] = useState<number | null>(null)
  const [error, setError] = useState<string | null>(null)
  const newRowRef = useRef<number | null>(null)
  const urlInputRefs = useRef<Map<number, HTMLInputElement>>(new Map())

  const load = useCallback(async () => {
    try {
      const data = await fetchThreadSources(threadId)
      setSources(data)
    } finally {
      setLoading(false)
    }
  }, [threadId])

  useEffect(() => { load() }, [load])

  useEffect(() => {
    if (newRowRef.current !== null) {
      const input = urlInputRefs.current.get(newRowRef.current)
      if (input) input.focus()
      newRowRef.current = null
    }
  })

  const handleAdd = () => {
    const tempId = -Date.now()
    setSources(prev => [...prev, { id: tempId, url: '', label: '', position: prev.length } as ThreadSource])
    newRowRef.current = tempId
  }

  const handleSave = async (source: ThreadSource) => {
    if (isTempId(source.id)) {
      if (!source.url.trim()) return
      setSaving(source.id)
      setError(null)
      try {
        const created = await createThreadSource(threadId, { url: source.url, label: source.label })
        setSources(prev => prev.map(s => s.id === source.id ? created : s))
      } catch {
        setSources(prev => prev.filter(s => s.id !== source.id))
        setError('Failed to create source')
        setTimeout(() => setError(null), 3000)
      } finally {
        setSaving(null)
      }
      return
    }
    if (!source.url.trim()) return
    setSaving(source.id)
    setError(null)
    try {
      await updateThreadSource(threadId, source.id, { url: source.url, label: source.label })
    } catch {
      setError('Failed to save source')
      setTimeout(() => setError(null), 3000)
    } finally {
      setSaving(null)
    }
  }

  const handleDelete = async (id: number) => {
    if (isTempId(id)) {
      setSources(prev => prev.filter(s => s.id !== id))
      return
    }
    await deleteThreadSource(threadId, id)
    setSources(prev => prev.filter(s => s.id !== id))
  }

  const handleChange = (id: number, field: 'url' | 'label', value: string) => {
    setSources(prev => prev.map(s => s.id === id ? { ...s, [field]: value } : s))
  }

  const handleDragStart = (idx: number) => setDragIdx(idx)
  const handleDragOver = (e: React.DragEvent, idx: number) => {
    e.preventDefault()
    if (dragIdx === null || dragIdx === idx) return
    const reordered = [...sources]
    const moved = reordered.splice(dragIdx, 1)[0]
    if (!moved) return
    reordered.splice(idx, 0, moved)
    setSources(reordered)
    setDragIdx(idx)
  }
  const handleDragEnd = async () => {
    setDragIdx(null)
    const persistedIds = sources.filter(s => !isTempId(s.id)).map(s => s.id)
    if (persistedIds.length > 0) await reorderThreadSources(threadId, persistedIds)
  }

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/40"
         onClick={onClose}>
      <div className="bg-white rounded-xl shadow-xl w-full max-w-lg mx-4 max-h-[80vh] flex flex-col"
           onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between px-5 py-4 border-b border-zinc-100">
          <div className="flex items-center gap-2">
            <Globe className="w-4 h-4 text-zinc-500" />
            <h2 className="text-sm font-semibold text-zinc-800">URL Sources</h2>
          </div>
          <button onClick={onClose}
                  className="text-xs text-zinc-400 hover:text-zinc-600 cursor-pointer">
            Done
          </button>
        </div>

        <div className="flex-1 overflow-y-auto px-5 py-3 space-y-2">
          {loading ? (
            <p className="text-xs text-zinc-400 py-4 text-center">Loading...</p>
          ) : sources.length === 0 ? (
            <p className="text-xs text-zinc-400 py-4 text-center">
              No sources yet. Add URLs to include their content in the chat context.
            </p>
          ) : (
            sources.map((src, idx) => (
              <div key={src.id}
                   draggable
                   onDragStart={() => handleDragStart(idx)}
                   onDragOver={e => handleDragOver(e, idx)}
                   onDragEnd={handleDragEnd}
                   className={`flex items-center gap-2 group rounded-lg p-2 -mx-2
                              ${dragIdx === idx ? 'bg-blue-50 opacity-70' : 'hover:bg-zinc-50'}`}>
                <GripVertical className="w-3.5 h-3.5 text-zinc-300 cursor-grab flex-shrink-0
                                         opacity-0 group-hover:opacity-100 transition-opacity" />
                <input
                  type="text"
                  value={src.label}
                  onChange={e => handleChange(src.id, 'label', e.target.value)}
                  onBlur={() => handleSave(src)}
                  placeholder="Label"
                  className="w-24 flex-shrink-0 text-xs bg-transparent border border-zinc-200
                             rounded px-2 py-1.5 text-zinc-700 placeholder-zinc-300
                             focus:border-zinc-400 focus:outline-none"
                />
                <input
                  type="url"
                  ref={el => { if (el) urlInputRefs.current.set(src.id, el); else urlInputRefs.current.delete(src.id) }}
                  value={src.url}
                  onChange={e => handleChange(src.id, 'url', e.target.value)}
                  onBlur={() => handleSave(src)}
                  placeholder="https://..."
                  className="flex-1 min-w-0 text-xs bg-transparent border border-zinc-200
                             rounded px-2 py-1.5 text-zinc-700 placeholder-zinc-300
                             focus:border-zinc-400 focus:outline-none"
                />
                {saving === src.id && (
                  <span className="text-[10px] text-zinc-400 flex-shrink-0">Saving...</span>
                )}
                <button onClick={() => handleDelete(src.id)}
                        className="text-zinc-300 hover:text-red-500 p-1 flex-shrink-0
                                   opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer">
                  <Trash2 className="w-3.5 h-3.5" />
                </button>
              </div>
            ))
          )}
        </div>

        <div className="px-5 py-3 border-t border-zinc-100 space-y-2">
          {error && (
            <p className="text-xs text-red-500">{error}</p>
          )}
          <button onClick={handleAdd}
                  className="flex items-center gap-1.5 text-xs text-zinc-500
                             hover:text-zinc-700 transition-colors cursor-pointer">
            <Plus className="w-3.5 h-3.5" />
            Add source
          </button>
        </div>
      </div>
    </div>
  )
}
