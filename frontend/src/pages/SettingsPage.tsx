import { useState, useEffect, useCallback } from 'react'
import { clsx } from 'clsx'
import {
  Loader2,
  Plus,
  Pencil,
  Trash2,
  X,
  Check,
  Brain,
  Tag as TagIcon,
  User,
  Mic,
  SettingsIcon,
  GripVertical,
} from 'lucide-react'

import { useSettings, type Theme, type FontSize } from '../context/SettingsContext'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import {
  fetchPersonas,
  createPersona,
  updatePersona,
  deletePersona,
  fetchTags,
  createTag,
  updateTag,
  deleteTag,
  getTagThreadCount,
  fetchMemories,
  createMemory,
  updateMemory,
  deleteMemory,
  getModels,
  getTranscribeStatus,
} from '../api/client'
import type { Persona, Tag, Memory } from '../types'

// ── Constants ──

const PERSONA_ICONS = ['🤖', '💻', '✍️', '🌐', '🧠', '📊', '🎨', '🔬', '📝', '🎓', '🛠️', '💡']

const TAG_COLORS = [
  { name: 'Gray', hex: '#6B7280' },
  { name: 'Red', hex: '#EF4444' },
  { name: 'Orange', hex: '#F97316' },
  { name: 'Amber', hex: '#F59E0B' },
  { name: 'Green', hex: '#22C55E' },
  { name: 'Blue', hex: '#3B82F6' },
  { name: 'Purple', hex: '#8B5CF6' },
  { name: 'Pink', hex: '#EC4899' },
]

type TabId = 'general' | 'personas' | 'tags' | 'memories' | 'voice'

interface TabDef {
  id: TabId
  label: string
  icon: typeof SettingsIcon
}

const TABS: TabDef[] = [
  { id: 'general', label: 'General', icon: SettingsIcon },
  { id: 'personas', label: 'Personas', icon: User },
  { id: 'tags', label: 'Tags', icon: TagIcon },
  { id: 'memories', label: 'Memories', icon: Brain },
  { id: 'voice', label: 'Voice', icon: Mic },
]

// ── General Tab ──

function GeneralTab() {
  const { settings, updateSettings } = useSettings()
  const [models, setModels] = useState<string[]>([])
  const [defaultModel, setDefaultModel] = useState('')

  useEffect(() => {
    getModels().then((r) => setModels(r.models)).catch(() => {})
  }, [])

  useEffect(() => {
    // Load the stored default model from localStorage
    const stored = localStorage.getItem('botka-default-model')
    if (stored) setDefaultModel(stored)
  }, [])

  function handleModelChange(model: string) {
    setDefaultModel(model)
    localStorage.setItem('botka-default-model', model)
  }

  return (
    <div className="space-y-6">
      {/* Theme */}
      <div>
        <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">Theme</label>
        <div className="mt-2 flex flex-wrap gap-2">
          {([
            { value: 'light', label: 'Light' },
            { value: 'dark', label: 'Dark' },
            { value: 'dark-green', label: 'Dark Green' },
            { value: 'dark-blue', label: 'Dark Blue' },
            { value: 'system', label: 'System' },
          ] as { value: Theme; label: string }[]).map((t) => (
            <button
              key={t.value}
              onClick={() => updateSettings({ theme: t.value })}
              className={clsx(
                'rounded-md px-4 py-2 text-sm font-medium transition-colors',
                settings.theme === t.value
                  ? 'bg-zinc-900 text-white dark:bg-white dark:text-zinc-900'
                  : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-600',
              )}
            >
              {t.label}
            </button>
          ))}
        </div>
      </div>

      {/* Font Size */}
      <div>
        <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">Font Size</label>
        <div className="mt-2 flex gap-2">
          {(['small', 'medium', 'large'] as FontSize[]).map((f) => (
            <button
              key={f}
              onClick={() => updateSettings({ fontSize: f })}
              className={clsx(
                'rounded-md px-4 py-2 text-sm font-medium capitalize transition-colors',
                settings.fontSize === f
                  ? 'bg-zinc-900 text-white dark:bg-white dark:text-zinc-900'
                  : 'bg-zinc-100 text-zinc-600 hover:bg-zinc-200 dark:bg-zinc-700 dark:text-zinc-300 dark:hover:bg-zinc-600',
              )}
            >
              {f}
            </button>
          ))}
        </div>
      </div>

      {/* Default Model */}
      {models.length > 0 && (
        <div>
          <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">Default AI Model</label>
          <select
            value={defaultModel}
            onChange={(e) => handleModelChange(e.target.value)}
            className="mt-2 w-full max-w-xs rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
          >
            <option value="">Auto</option>
            {models.map((m) => (
              <option key={m} value={m}>
                {m}
              </option>
            ))}
          </select>
        </div>
      )}

      {/* Notification Sound */}
      <div className="flex items-center justify-between max-w-xs">
        <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">Notification Sound</label>
        <button
          onClick={() => updateSettings({ notificationSound: !settings.notificationSound })}
          className={clsx(
            'relative h-6 w-11 rounded-full transition-colors',
            settings.notificationSound ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-600',
          )}
        >
          <span
            className={clsx(
              'absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform',
              settings.notificationSound && 'translate-x-5',
            )}
          />
        </button>
      </div>

      {/* Send on Enter */}
      <div className="flex items-center justify-between max-w-xs">
        <label className="text-sm font-medium text-zinc-700 dark:text-zinc-300">Send on Enter</label>
        <button
          onClick={() => updateSettings({ sendOnEnter: !settings.sendOnEnter })}
          className={clsx(
            'relative h-6 w-11 rounded-full transition-colors',
            settings.sendOnEnter ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-600',
          )}
        >
          <span
            className={clsx(
              'absolute top-0.5 left-0.5 h-5 w-5 rounded-full bg-white shadow transition-transform',
              settings.sendOnEnter && 'translate-x-5',
            )}
          />
        </button>
      </div>
    </div>
  )
}

// ── Personas Tab ──

const EMPTY_PERSONA_FORM = {
  name: '',
  system_prompt: '',
  default_model: '',
  icon: '🤖',
  starter_message: '',
  sort_order: 0,
}

function PersonasTab() {
  const [personas, setPersonas] = useState<Persona[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [editingId, setEditingId] = useState<number | null>(null)
  const [adding, setAdding] = useState(false)
  const [form, setForm] = useState(EMPTY_PERSONA_FORM)
  const [saving, setSaving] = useState(false)
  const [deleteConfirm, setDeleteConfirm] = useState<number | null>(null)
  const [models, setModels] = useState<string[]>([])
  const [dragIndex, setDragIndex] = useState<number | null>(null)
  const [dragOverIndex, setDragOverIndex] = useState<number | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    fetchPersonas()
      .then(setPersonas)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
    getModels().then((r) => setModels(r.models)).catch(() => {})
  }, [load])

  function startEdit(p: Persona) {
    setEditingId(p.id)
    setAdding(false)
    setForm({
      name: p.name,
      system_prompt: p.system_prompt,
      default_model: p.default_model || '',
      icon: p.icon || '🤖',
      starter_message: p.starter_message || '',
      sort_order: p.sort_order,
    })
  }

  function startAdd() {
    setAdding(true)
    setEditingId(null)
    setForm(EMPTY_PERSONA_FORM)
  }

  function cancel() {
    setAdding(false)
    setEditingId(null)
    setForm(EMPTY_PERSONA_FORM)
    setError('')
  }

  async function save() {
    if (!form.name.trim()) {
      setError('Name is required')
      return
    }
    setSaving(true)
    setError('')
    try {
      const data = {
        name: form.name.trim(),
        system_prompt: form.system_prompt,
        default_model: form.default_model || undefined,
        icon: form.icon,
        starter_message: form.starter_message || undefined,
        sort_order: form.sort_order,
      }
      if (editingId !== null) {
        await updatePersona(editingId, data)
      } else {
        await createPersona(data)
      }
      cancel()
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  async function handleDelete(id: number) {
    try {
      await deletePersona(id)
      setDeleteConfirm(null)
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  function handleDragStart(idx: number) {
    setDragIndex(idx)
  }

  function handleDragOver(e: React.DragEvent, idx: number) {
    e.preventDefault()
    setDragOverIndex(idx)
  }

  async function handleDrop(idx: number) {
    if (dragIndex === null || dragIndex === idx) {
      setDragIndex(null)
      setDragOverIndex(null)
      return
    }
    const reordered = [...personas]
    const [moved] = reordered.splice(dragIndex, 1)
    reordered.splice(idx, 0, moved!)
    setPersonas(reordered)
    setDragIndex(null)
    setDragOverIndex(null)

    // Save new sort orders
    for (let i = 0; i < reordered.length; i++) {
      const p = reordered[i]!
      if (p.sort_order !== i) {
        await updatePersona(p.id, { sort_order: i })
      }
    }
    load()
  }

  const isEditing = adding || editingId !== null

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {!isEditing && (
        <button
          onClick={startAdd}
          className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
        >
          <Plus className="h-4 w-4" />
          Add Persona
        </button>
      )}

      {isEditing && (
        <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-4 space-y-4 dark:border-zinc-700 dark:bg-zinc-800">
          <h3 className="text-sm font-medium text-zinc-900 dark:text-zinc-100">
            {adding ? 'New Persona' : 'Edit Persona'}
          </h3>

          {/* Icon picker */}
          <div>
            <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">Icon</label>
            <div className="mt-1 flex flex-wrap gap-1">
              {PERSONA_ICONS.map((icon) => (
                <button
                  key={icon}
                  onClick={() => setForm((f) => ({ ...f, icon }))}
                  className={clsx(
                    'h-8 w-8 rounded-md text-base transition-all',
                    form.icon === icon
                      ? 'bg-zinc-200 ring-2 ring-zinc-400 dark:bg-zinc-600 dark:ring-zinc-500'
                      : 'hover:bg-zinc-100 dark:hover:bg-zinc-700',
                  )}
                >
                  {icon}
                </button>
              ))}
            </div>
          </div>

          {/* Name */}
          <div>
            <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">Name</label>
            <input
              type="text"
              value={form.name}
              onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
              className="mt-1 w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
              placeholder="Persona name"
            />
          </div>

          {/* System Prompt */}
          <div>
            <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">System Prompt</label>
            <textarea
              value={form.system_prompt}
              onChange={(e) => setForm((f) => ({ ...f, system_prompt: e.target.value }))}
              rows={4}
              className="mt-1 w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
              placeholder="You are a helpful assistant that..."
            />
          </div>

          {/* Default Model */}
          <div>
            <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">Default Model</label>
            <select
              value={form.default_model}
              onChange={(e) => setForm((f) => ({ ...f, default_model: e.target.value }))}
              className="mt-1 w-full max-w-xs rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
            >
              <option value="">Default</option>
              {models.map((m) => (
                <option key={m} value={m}>
                  {m}
                </option>
              ))}
            </select>
          </div>

          {/* Starter Message */}
          <div>
            <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">Starter Message</label>
            <textarea
              value={form.starter_message}
              onChange={(e) => setForm((f) => ({ ...f, starter_message: e.target.value }))}
              rows={2}
              className="mt-1 w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
              placeholder="Optional message to start conversation"
            />
          </div>

          {error && <p className="text-sm text-red-500">{error}</p>}

          <div className="flex items-center gap-2">
            <button
              onClick={save}
              disabled={saving}
              className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
            >
              {saving ? <Loader2 className="h-3.5 w-3.5 animate-spin" /> : <Check className="h-3.5 w-3.5" />}
              Save
            </button>
            <button
              onClick={cancel}
              className="rounded-md px-3 py-1.5 text-sm font-medium text-zinc-600 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-700"
            >
              Cancel
            </button>
          </div>
        </div>
      )}

      {/* Persona list */}
      {personas.length === 0 && !isEditing ? (
        <div className="flex h-32 flex-col items-center justify-center gap-2">
          <User className="h-8 w-8 text-zinc-300" />
          <p className="text-sm text-zinc-400">No personas yet</p>
        </div>
      ) : (
        <div className="space-y-2">
          {personas.map((p, idx) => (
            <div
              key={p.id}
              draggable
              onDragStart={() => handleDragStart(idx)}
              onDragOver={(e) => handleDragOver(e, idx)}
              onDrop={() => handleDrop(idx)}
              onDragEnd={() => { setDragIndex(null); setDragOverIndex(null) }}
              className={clsx(
                'flex items-center gap-3 rounded-lg border border-zinc-200 bg-zinc-50 px-4 py-3 transition-colors dark:border-zinc-700 dark:bg-zinc-800',
                dragOverIndex === idx && 'border-zinc-400 bg-zinc-50',
              )}
            >
              <GripVertical className="h-4 w-4 shrink-0 cursor-grab text-zinc-300" />
              <span className="text-xl shrink-0">{p.icon || '🤖'}</span>
              <div className="flex-1 min-w-0">
                <div className="font-medium text-zinc-900 dark:text-zinc-100">{p.name}</div>
                {p.default_model && (
                  <p className="text-xs text-zinc-400 truncate">{p.default_model}</p>
                )}
                {p.system_prompt && (
                  <p className="text-xs text-zinc-400 truncate mt-0.5">
                    {p.system_prompt.slice(0, 100)}
                    {p.system_prompt.length > 100 ? '...' : ''}
                  </p>
                )}
              </div>
              <div className="flex items-center gap-1 shrink-0">
                <button
                  onClick={() => startEdit(p)}
                  className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-700 dark:hover:text-zinc-300"
                  title="Edit"
                >
                  <Pencil className="h-3.5 w-3.5" />
                </button>
                {deleteConfirm === p.id ? (
                  <div className="flex items-center gap-1">
                    <button
                      onClick={() => handleDelete(p.id)}
                      className="rounded p-1.5 text-red-500 hover:bg-red-50"
                      title="Confirm delete"
                    >
                      <Check className="h-3.5 w-3.5" />
                    </button>
                    <button
                      onClick={() => setDeleteConfirm(null)}
                      className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-700"
                      title="Cancel"
                    >
                      <X className="h-3.5 w-3.5" />
                    </button>
                  </div>
                ) : (
                  <button
                    onClick={() => setDeleteConfirm(p.id)}
                    className="rounded p-1.5 text-zinc-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950"
                    title="Delete"
                  >
                    <Trash2 className="h-3.5 w-3.5" />
                  </button>
                )}
              </div>
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Tags Tab ──

function TagsTab() {
  const [tags, setTags] = useState<Tag[]>([])
  const [threadCounts, setThreadCounts] = useState<Record<number, number>>({})
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // New tag form
  const [newName, setNewName] = useState('')
  const [newColor, setNewColor] = useState('#3B82F6')

  // Edit mode
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editName, setEditName] = useState('')
  const [editColor, setEditColor] = useState('')

  const [deleteConfirm, setDeleteConfirm] = useState<number | null>(null)
  const [deleteCount, setDeleteCount] = useState<number | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    fetchTags()
      .then((tags) => {
        setTags(tags)
        // Load thread counts for all tags
        Promise.all(tags.map((t) => getTagThreadCount(t.id).then((count) => [t.id, count] as const)))
          .then((pairs) => {
            const map: Record<number, number> = {}
            for (const [id, count] of pairs) map[id] = count
            setThreadCounts(map)
          })
          .catch(() => {})
      })
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  async function handleCreate() {
    if (!newName.trim()) return
    setError('')
    try {
      await createTag({ name: newName.trim(), color: newColor })
      setNewName('')
      setNewColor('#3B82F6')
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create')
    }
  }

  function startEdit(t: Tag) {
    setEditingId(t.id)
    setEditName(t.name)
    setEditColor(t.color)
  }

  async function saveEdit() {
    if (!editName.trim() || editingId === null) return
    setError('')
    try {
      await updateTag(editingId, { name: editName.trim(), color: editColor })
      setEditingId(null)
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update')
    }
  }

  async function handleDeleteClick(id: number) {
    if (deleteConfirm === id) {
      try {
        await deleteTag(id)
        setDeleteConfirm(null)
        setDeleteCount(null)
        load()
      } catch (e) {
        setError(e instanceof Error ? e.message : 'Failed to delete')
      }
    } else {
      setDeleteConfirm(id)
      try {
        const count = await getTagThreadCount(id)
        setDeleteCount(count)
      } catch {
        setDeleteCount(null)
      }
    }
  }

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      {/* Create form */}
      <div className="flex items-end gap-3">
        <div className="flex-1 max-w-xs">
          <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">Name</label>
          <input
            type="text"
            value={newName}
            onChange={(e) => setNewName(e.target.value)}
            onKeyDown={(e) => e.key === 'Enter' && handleCreate()}
            className="mt-1 w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
            placeholder="Tag name"
          />
        </div>
        <div>
          <label className="text-xs font-medium text-zinc-500 dark:text-zinc-400">Color</label>
          <div className="mt-1 flex gap-1">
            {TAG_COLORS.map((c) => (
              <button
                key={c.hex}
                onClick={() => setNewColor(c.hex)}
                title={c.name}
                className={clsx(
                  'h-7 w-7 rounded-full transition-transform',
                  newColor === c.hex && 'ring-2 ring-zinc-400 ring-offset-2 scale-110 dark:ring-offset-zinc-900',
                )}
                style={{ backgroundColor: c.hex }}
              />
            ))}
          </div>
        </div>
        <button
          onClick={handleCreate}
          disabled={!newName.trim()}
          className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
        >
          <Plus className="h-4 w-4" />
          Add
        </button>
      </div>

      {error && <p className="text-sm text-red-500">{error}</p>}

      {/* Tag list */}
      {tags.length === 0 ? (
        <div className="flex h-32 flex-col items-center justify-center gap-2">
          <TagIcon className="h-8 w-8 text-zinc-300" />
          <p className="text-sm text-zinc-400">No tags yet</p>
        </div>
      ) : (
        <div className="space-y-2">
          {tags.map((t) => (
            <div
              key={t.id}
              className="flex items-center gap-3 rounded-lg border border-zinc-200 bg-zinc-50 px-4 py-3 dark:border-zinc-700 dark:bg-zinc-800"
            >
              {editingId === t.id ? (
                <>
                  <div className="flex gap-1">
                    {TAG_COLORS.map((c) => (
                      <button
                        key={c.hex}
                        onClick={() => setEditColor(c.hex)}
                        title={c.name}
                        className={clsx(
                          'h-5 w-5 rounded-full transition-transform',
                          editColor === c.hex && 'ring-2 ring-zinc-400 ring-offset-1 scale-110 dark:ring-offset-zinc-800',
                        )}
                        style={{ backgroundColor: c.hex }}
                      />
                    ))}
                  </div>
                  <input
                    type="text"
                    value={editName}
                    onChange={(e) => setEditName(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter') saveEdit()
                      if (e.key === 'Escape') setEditingId(null)
                    }}
                    className="flex-1 rounded-md border border-zinc-300 bg-zinc-50 px-2 py-1 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
                    autoFocus
                  />
                  <button
                    onClick={saveEdit}
                    className="rounded p-1.5 text-emerald-600 hover:bg-emerald-50"
                  >
                    <Check className="h-3.5 w-3.5" />
                  </button>
                  <button
                    onClick={() => setEditingId(null)}
                    className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-700"
                  >
                    <X className="h-3.5 w-3.5" />
                  </button>
                </>
              ) : (
                <>
                  <span
                    className="h-4 w-4 rounded-full shrink-0"
                    style={{ backgroundColor: t.color }}
                  />
                  <span className="flex-1 text-sm font-medium text-zinc-900 dark:text-zinc-100">{t.name}</span>
                  <span className="text-xs text-zinc-400 tabular-nums">
                    {threadCounts[t.id] ?? 0} threads
                  </span>
                  <button
                    onClick={() => startEdit(t)}
                    className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-700 dark:hover:text-zinc-300"
                    title="Edit"
                  >
                    <Pencil className="h-3.5 w-3.5" />
                  </button>
                  {deleteConfirm === t.id ? (
                    <div className="flex items-center gap-1">
                      <span className="text-xs text-red-500">
                        {deleteCount !== null ? `${deleteCount} threads` : 'Delete?'}
                      </span>
                      <button
                        onClick={() => handleDeleteClick(t.id)}
                        className="rounded p-1.5 text-red-500 hover:bg-red-50"
                        title="Confirm delete"
                      >
                        <Check className="h-3.5 w-3.5" />
                      </button>
                      <button
                        onClick={() => { setDeleteConfirm(null); setDeleteCount(null) }}
                        className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-700"
                        title="Cancel"
                      >
                        <X className="h-3.5 w-3.5" />
                      </button>
                    </div>
                  ) : (
                    <button
                      onClick={() => handleDeleteClick(t.id)}
                      className="rounded p-1.5 text-zinc-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950"
                      title="Delete"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                    </button>
                  )}
                </>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ── Memories Tab ──

function MemoriesTab() {
  const [memories, setMemories] = useState<Memory[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [newContent, setNewContent] = useState('')
  const [editingId, setEditingId] = useState<string | null>(null)
  const [editContent, setEditContent] = useState('')
  const [deleteConfirm, setDeleteConfirm] = useState<string | null>(null)

  const load = useCallback(() => {
    setLoading(true)
    fetchMemories()
      .then(setMemories)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [])

  useEffect(() => {
    load()
  }, [load])

  async function handleCreate() {
    if (!newContent.trim()) return
    setError('')
    try {
      await createMemory(newContent.trim())
      setNewContent('')
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to create')
    }
  }

  function startEdit(m: Memory) {
    setEditingId(m.id)
    setEditContent(m.content)
  }

  async function saveEdit() {
    if (!editContent.trim() || editingId === null) return
    setError('')
    try {
      await updateMemory(editingId, editContent.trim())
      setEditingId(null)
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to update')
    }
  }

  async function handleDelete(id: string) {
    try {
      await deleteMemory(id)
      setDeleteConfirm(null)
      load()
    } catch (e) {
      setError(e instanceof Error ? e.message : 'Failed to delete')
    }
  }

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <p className="text-sm text-zinc-500">
        Memories are included in every chat session's system prompt to provide persistent context.
      </p>

      {/* Create form */}
      <div className="flex gap-2">
        <textarea
          value={newContent}
          onChange={(e) => setNewContent(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) handleCreate()
          }}
          rows={2}
          className="flex-1 rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-800 dark:text-zinc-100"
          placeholder="Add a memory..."
        />
        <button
          onClick={handleCreate}
          disabled={!newContent.trim()}
          className="self-end inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
        >
          <Plus className="h-4 w-4" />
          Add
        </button>
      </div>

      {error && <p className="text-sm text-red-500">{error}</p>}

      {/* Memory list */}
      {memories.length === 0 ? (
        <div className="flex h-32 flex-col items-center justify-center gap-2">
          <Brain className="h-8 w-8 text-zinc-300" />
          <p className="text-sm text-zinc-400">No memories yet</p>
        </div>
      ) : (
        <div className="space-y-2">
          {memories.map((m) => (
            <div
              key={m.id}
              className="rounded-lg border border-zinc-200 bg-zinc-50 px-4 py-3 dark:border-zinc-700 dark:bg-zinc-800"
            >
              {editingId === m.id ? (
                <div className="space-y-2">
                  <textarea
                    value={editContent}
                    onChange={(e) => setEditContent(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) saveEdit()
                      if (e.key === 'Escape') setEditingId(null)
                    }}
                    rows={3}
                    className="w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500 dark:border-zinc-600 dark:bg-zinc-700 dark:text-zinc-100"
                    autoFocus
                  />
                  <div className="flex items-center gap-2">
                    <button
                      onClick={saveEdit}
                      className="inline-flex items-center gap-1 rounded-md bg-zinc-900 px-2.5 py-1 text-xs font-medium text-white hover:bg-zinc-800 dark:bg-white dark:text-zinc-900 dark:hover:bg-zinc-200"
                    >
                      <Check className="h-3 w-3" />
                      Save
                    </button>
                    <button
                      onClick={() => setEditingId(null)}
                      className="rounded-md px-2.5 py-1 text-xs font-medium text-zinc-500 hover:bg-zinc-100 dark:text-zinc-400 dark:hover:bg-zinc-700"
                    >
                      Cancel
                    </button>
                  </div>
                </div>
              ) : (
                <div className="flex items-start gap-3">
                  <p className="flex-1 text-sm text-zinc-700 whitespace-pre-wrap dark:text-zinc-300">
                    {m.content.length > 200 ? m.content.slice(0, 200) + '...' : m.content}
                  </p>
                  <div className="flex items-center gap-1 shrink-0">
                    <button
                      onClick={() => startEdit(m)}
                      className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-600 dark:hover:bg-zinc-700 dark:hover:text-zinc-300"
                      title="Edit"
                    >
                      <Pencil className="h-3.5 w-3.5" />
                    </button>
                    {deleteConfirm === m.id ? (
                      <div className="flex items-center gap-1">
                        <button
                          onClick={() => handleDelete(m.id)}
                          className="rounded p-1.5 text-red-500 hover:bg-red-50"
                          title="Confirm delete"
                        >
                          <Check className="h-3.5 w-3.5" />
                        </button>
                        <button
                          onClick={() => setDeleteConfirm(null)}
                          className="rounded p-1.5 text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-700"
                          title="Cancel"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      </div>
                    ) : (
                      <button
                        onClick={() => setDeleteConfirm(m.id)}
                        className="rounded p-1.5 text-zinc-400 hover:bg-red-50 hover:text-red-500 dark:hover:bg-red-950"
                        title="Delete"
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </button>
                    )}
                  </div>
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      <p className="text-xs text-zinc-400">
        {memories.length}/100 memories used
      </p>
    </div>
  )
}

// ── Voice Tab ──

function VoiceTab() {
  const [enabled, setEnabled] = useState<boolean | null>(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    getTranscribeStatus()
      .then((r) => setEnabled(r.enabled))
      .catch(() => setEnabled(false))
      .finally(() => setLoading(false))
  }, [])

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center gap-3">
        <div
          className={clsx(
            'h-3 w-3 rounded-full',
            enabled ? 'bg-emerald-500' : 'bg-zinc-300 dark:bg-zinc-600',
          )}
        />
        <span className="text-sm text-zinc-700 dark:text-zinc-300">
          Transcription is {enabled ? 'enabled' : 'disabled'}
        </span>
      </div>

      {!enabled && (
        <p className="text-sm text-zinc-400">
          Voice transcription requires a Whisper-compatible server to be configured on the backend.
        </p>
      )}

      {enabled && (
        <p className="text-sm text-zinc-500">
          Voice input is available in chat. Click the microphone icon in the chat input to start recording.
        </p>
      )}
    </div>
  )
}

// ── Main Settings Page ──

export default function SettingsPage() {
  useDocumentTitle('Settings')
  const [activeTab, setActiveTab] = useState<TabId>('general')

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100">Settings</h1>

      {/* Tab navigation */}
      <div className="border-b border-zinc-200 dark:border-zinc-700">
        <nav className="flex gap-6">
          {TABS.map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              onClick={() => setActiveTab(id)}
              className={clsx(
                'flex items-center gap-1.5 border-b-2 pb-2.5 pt-1 text-sm font-medium transition-colors',
                activeTab === id
                  ? 'border-zinc-900 text-zinc-900 dark:border-white dark:text-white'
                  : 'border-transparent text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-300',
              )}
            >
              <Icon className="h-4 w-4" />
              {label}
            </button>
          ))}
        </nav>
      </div>

      {/* Tab content */}
      <div>
        {activeTab === 'general' && <GeneralTab />}
        {activeTab === 'personas' && <PersonasTab />}
        {activeTab === 'tags' && <TagsTab />}
        {activeTab === 'memories' && <MemoriesTab />}
        {activeTab === 'voice' && <VoiceTab />}
      </div>
    </div>
  )
}
