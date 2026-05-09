import { useEffect, useMemo, useRef, useState } from 'react'
import { clsx } from 'clsx'
import { Plus, Tag as TagIcon } from 'lucide-react'

import { assignTaskTags, createTaskTag, fetchTaskTags } from '../api/client'
import type { TaskTag } from '../types'
import { TaskTagChip } from './TaskTagChip'

// suggestedColors cycles when the user creates a new tag without choosing one.
const suggestedColors = [
  '#EF4444',
  '#F59E0B',
  '#10B981',
  '#3B82F6',
  '#A855F7',
  '#EC4899',
  '#14B8A6',
  '#6B7280',
]

interface TaskTagPickerProps {
  taskId: string
  selected: TaskTag[]
  onChange: (tags: TaskTag[]) => void
}

export function TaskTagPicker({ taskId, selected, onChange }: TaskTagPickerProps) {
  const [open, setOpen] = useState(false)
  const [allTags, setAllTags] = useState<TaskTag[]>([])
  const [query, setQuery] = useState('')
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const popoverRef = useRef<HTMLDivElement>(null)

  // Load tag catalogue on first open. Subsequent opens reuse the cached list
  // (the parent owns the per-task selection, which is always fresh).
  useEffect(() => {
    if (!open || allTags.length > 0) return
    fetchTaskTags()
      .then(setAllTags)
      .catch((err) => setError(err instanceof Error ? err.message : 'Failed to load tags'))
  }, [open, allTags.length])

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('mousedown', handler, true)
    return () => document.removeEventListener('mousedown', handler, true)
  }, [open])

  const selectedIDs = useMemo(() => new Set(selected.map((t) => t.id)), [selected])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    const list = q ? allTags.filter((t) => t.name.toLowerCase().includes(q)) : allTags
    return list.slice().sort((a, b) => a.name.localeCompare(b.name))
  }, [allTags, query])

  const exactMatch = useMemo(
    () => allTags.find((t) => t.name.toLowerCase() === query.trim().toLowerCase()),
    [allTags, query],
  )

  // toggle adds or removes a tag in the local selection set, then persists.
  const toggle = async (tag: TaskTag) => {
    const next = selectedIDs.has(tag.id)
      ? selected.filter((t) => t.id !== tag.id)
      : [...selected, tag]
    await persist(next)
  }

  // persist sends the new tag set to the server; on failure we revert.
  const persist = async (next: TaskTag[]) => {
    setSaving(true)
    setError(null)
    const previous = selected
    onChange(next)
    try {
      const fresh = await assignTaskTags(taskId, next.map((t) => t.id))
      onChange(fresh)
    } catch (err) {
      onChange(previous)
      setError(err instanceof Error ? err.message : 'Failed to save tags')
    } finally {
      setSaving(false)
    }
  }

  const handleCreate = async () => {
    const name = query.trim()
    if (!name) return
    setSaving(true)
    setError(null)
    try {
      const color = suggestedColors[allTags.length % suggestedColors.length]!
      const created = await createTaskTag({ name, color })
      setAllTags((prev) => [...prev, created])
      setQuery('')
      await persist([...selected, created])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to create tag')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="relative" ref={popoverRef}>
      <div className="flex flex-wrap items-center gap-1.5">
        {selected.map((tag) => (
          <TaskTagChip
            key={tag.id}
            tag={tag}
            onRemove={() => persist(selected.filter((t) => t.id !== tag.id))}
          />
        ))}
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          className={clsx(
            'inline-flex items-center gap-1 rounded-full border border-dashed px-2 py-0.5 text-xs',
            'border-zinc-300 text-zinc-500 hover:border-zinc-400 hover:text-zinc-700',
          )}
        >
          <TagIcon className="h-3 w-3" />
          {selected.length === 0 ? 'Add tags' : 'Edit'}
        </button>
      </div>

      {open && (
        <div className="absolute left-0 top-full z-20 mt-1 w-72 rounded-md border border-zinc-200 bg-white shadow-lg">
          <div className="border-b border-zinc-100 p-2">
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !exactMatch && query.trim()) {
                  e.preventDefault()
                  handleCreate()
                }
              }}
              placeholder="Search or create…"
              className="w-full rounded border border-zinc-200 px-2 py-1 text-sm focus:border-zinc-400 focus:outline-none"
            />
          </div>
          <div className="max-h-60 overflow-auto p-1">
            {filtered.length === 0 && !query.trim() && (
              <p className="px-2 py-1.5 text-xs text-zinc-500">No tags yet — type to create one.</p>
            )}
            {filtered.map((tag) => {
              const checked = selectedIDs.has(tag.id)
              return (
                <button
                  key={tag.id}
                  type="button"
                  onClick={() => toggle(tag)}
                  className={clsx(
                    'flex w-full items-center justify-between rounded px-2 py-1 text-left text-sm hover:bg-zinc-50',
                    checked && 'bg-zinc-50',
                  )}
                >
                  <TaskTagChip tag={tag} selected={checked} />
                  {checked && <span className="text-xs text-zinc-400">✓</span>}
                </button>
              )
            })}
            {query.trim() && !exactMatch && (
              <button
                type="button"
                onClick={handleCreate}
                disabled={saving}
                className="mt-1 flex w-full items-center gap-1.5 rounded px-2 py-1.5 text-left text-sm text-zinc-700 hover:bg-zinc-50 disabled:opacity-50"
              >
                <Plus className="h-3.5 w-3.5" />
                Create &ldquo;{query.trim()}&rdquo;
              </button>
            )}
          </div>
          {error && (
            <p className="border-t border-zinc-100 px-2 py-1.5 text-xs text-red-600">{error}</p>
          )}
        </div>
      )}
    </div>
  )
}
