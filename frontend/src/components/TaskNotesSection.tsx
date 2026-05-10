import { useCallback, useEffect, useRef, useState } from 'react'
import ReactMarkdown from 'react-markdown'
import { Loader2, Pencil, StickyNote, Trash2, X } from 'lucide-react'
import { clsx } from 'clsx'

import { formatDateTime } from '../utils/dateFormat'
import { createTaskNote, deleteTaskNote, fetchTaskNotes, updateTaskNote } from '../api/client'
import type { TaskNote } from '../types'

interface TaskNotesSectionProps {
  taskId: string
  onCountChange?: (count: number) => void
}

export function TaskNotesSection({ taskId, onCountChange }: TaskNotesSectionProps) {
  const [notes, setNotes] = useState<TaskNote[] | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [draft, setDraft] = useState('')
  const [saving, setSaving] = useState(false)

  const reportCount = useCallback(
    (xs: TaskNote[]) => {
      onCountChange?.(xs.length)
    },
    [onCountChange],
  )

  const load = useCallback(async () => {
    try {
      const next = await fetchTaskNotes(taskId)
      setNotes(next)
      reportCount(next)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load notes')
    } finally {
      setLoading(false)
    }
  }, [taskId, reportCount])

  useEffect(() => {
    load()
  }, [load])

  async function handleAdd() {
    const trimmed = draft.trim()
    if (!trimmed || saving) return
    setSaving(true)
    setError(null)
    try {
      const created = await createTaskNote(taskId, trimmed)
      setNotes((prev) => {
        const next = [...(prev ?? []), created]
        reportCount(next)
        return next
      })
      setDraft('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to add note')
    } finally {
      setSaving(false)
    }
  }

  async function handleSaveEdit(noteId: number, body: string) {
    const updated = await updateTaskNote(taskId, noteId, body)
    setNotes((prev) => {
      const next = (prev ?? []).map((n) => (n.id === noteId ? updated : n))
      reportCount(next)
      return next
    })
  }

  async function handleDelete(noteId: number) {
    if (!confirm('Delete this note? Soft-deleted notes can be recovered manually via SQL.')) return
    await deleteTaskNote(taskId, noteId)
    setNotes((prev) => {
      const next = (prev ?? []).filter((n) => n.id !== noteId)
      reportCount(next)
      return next
    })
  }

  function handleKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      handleAdd()
    }
  }

  return (
    <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
      <div className="mb-3 flex items-center gap-2">
        <StickyNote className="h-4 w-4 text-zinc-500" />
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">Notes</h2>
        {notes && notes.length > 0 && (
          <span className="text-xs text-zinc-400">({notes.length})</span>
        )}
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-sm text-zinc-500">
          <Loader2 className="h-4 w-4 animate-spin" />
          Loading notes…
        </div>
      ) : (
        <>
          {error && (
            <div className="mb-3 rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">{error}</div>
          )}
          {notes && notes.length === 0 && (
            <p className="mb-3 text-sm italic text-zinc-400">
              No notes yet. Add one to capture observations or follow-ups.
            </p>
          )}
          <ul className="mb-4 space-y-2">
            {(notes ?? []).map((note) => (
              <NoteItem
                key={note.id}
                note={note}
                onSave={handleSaveEdit}
                onDelete={() => handleDelete(note.id)}
              />
            ))}
          </ul>

          <div className="rounded-md border border-zinc-200 bg-white p-3">
            <textarea
              className="w-full resize-y rounded-md border border-zinc-200 bg-white p-2 text-sm text-zinc-900 focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400"
              rows={3}
              placeholder="Add a note… (Cmd+Enter to submit, Markdown supported)"
              value={draft}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={handleKey}
              disabled={saving}
            />
            <div className="mt-2 flex items-center justify-between">
              <span className="text-xs text-zinc-400">
                {saving ? 'Saving…' : 'Cmd/Ctrl+Enter to submit'}
              </span>
              <button
                type="button"
                onClick={handleAdd}
                disabled={!draft.trim() || saving}
                className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 disabled:opacity-50"
              >
                Add note
              </button>
            </div>
          </div>
        </>
      )}
    </div>
  )
}

interface NoteItemProps {
  note: TaskNote
  onSave: (id: number, body: string) => Promise<void>
  onDelete: () => Promise<void> | void
}

function NoteItem({ note, onSave, onDelete }: NoteItemProps) {
  const [editing, setEditing] = useState(false)
  const [draft, setDraft] = useState(note.body)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  useEffect(() => {
    if (editing) {
      textareaRef.current?.focus()
    }
  }, [editing])

  async function handleSave() {
    const trimmed = draft.trim()
    if (!trimmed || saving) return
    setSaving(true)
    setError(null)
    try {
      await onSave(note.id, trimmed)
      setEditing(false)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  function handleCancel() {
    setDraft(note.body)
    setEditing(false)
    setError(null)
  }

  function handleKey(e: React.KeyboardEvent<HTMLTextAreaElement>) {
    if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
      e.preventDefault()
      handleSave()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      handleCancel()
    }
  }

  const edited = note.updated_at && note.updated_at !== note.created_at

  return (
    <li className="rounded-md border border-zinc-200 bg-white p-3">
      <div className="mb-1.5 flex items-center justify-between gap-2 text-xs text-zinc-500">
        <div className="flex items-center gap-2">
          <span className="font-medium text-zinc-700">{note.author}</span>
          <span>{formatDateTime(note.created_at)}</span>
          {edited && <span className="italic">(edited {formatDateTime(note.updated_at)})</span>}
        </div>
        {!editing && (
          <div className="flex items-center gap-1">
            <button
              type="button"
              onClick={() => setEditing(true)}
              className="rounded p-1 text-zinc-400 hover:bg-zinc-100 hover:text-zinc-700"
              aria-label="Edit note"
              title="Edit"
            >
              <Pencil className="h-3.5 w-3.5" />
            </button>
            <button
              type="button"
              onClick={() => onDelete()}
              className="rounded p-1 text-zinc-400 hover:bg-red-50 hover:text-red-700"
              aria-label="Delete note"
              title="Delete"
            >
              <Trash2 className="h-3.5 w-3.5" />
            </button>
          </div>
        )}
      </div>

      {editing ? (
        <div>
          <textarea
            ref={textareaRef}
            className="w-full resize-y rounded-md border border-zinc-200 bg-white p-2 text-sm text-zinc-900 focus:border-blue-400 focus:outline-none focus:ring-1 focus:ring-blue-400"
            rows={Math.max(3, draft.split('\n').length)}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            onKeyDown={handleKey}
            disabled={saving}
          />
          {error && <p className="mt-1 text-xs text-red-700">{error}</p>}
          <div className="mt-2 flex items-center justify-end gap-2">
            <button
              type="button"
              onClick={handleCancel}
              className="inline-flex items-center gap-1 rounded-md border border-zinc-200 px-2 py-1 text-xs font-medium text-zinc-600 hover:bg-zinc-50"
            >
              <X className="h-3 w-3" />
              Cancel
            </button>
            <button
              type="button"
              onClick={handleSave}
              disabled={!draft.trim() || saving || draft.trim() === note.body}
              className={clsx(
                'inline-flex items-center gap-1 rounded-md bg-blue-600 px-2 py-1 text-xs font-medium text-white hover:bg-blue-700',
                'disabled:opacity-50',
              )}
            >
              {saving ? 'Saving…' : 'Save'}
            </button>
          </div>
        </div>
      ) : (
        <div className="prose prose-sm prose-zinc max-w-none min-w-0 break-words">
          <ReactMarkdown>{note.body}</ReactMarkdown>
        </div>
      )}
    </li>
  )
}
