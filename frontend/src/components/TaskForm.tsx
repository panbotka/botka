import { useState, useEffect } from 'react'
import ReactMarkdown from 'react-markdown'
import { Loader2, Eye, EyeOff } from 'lucide-react'
import { fetchProjects, createTask, updateTask, fetchTask } from '../api/client'
import type { Project } from '../types'

interface TaskFormProps {
  taskId?: string
  onSave: () => void
  onCancel: () => void
}

export function TaskForm({ taskId, onSave, onCancel }: TaskFormProps) {
  const isEdit = !!taskId

  const [projects, setProjects] = useState<Project[]>([])
  const [projectId, setProjectId] = useState('')
  const [title, setTitle] = useState('')
  const [spec, setSpec] = useState('')
  const [priority, setPriority] = useState(0)
  const [status, setStatus] = useState<'queued' | 'pending'>('queued')
  const [preview, setPreview] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    async function load() {
      try {
        const [projectList, task] = await Promise.all([
          fetchProjects(),
          taskId ? fetchTask(taskId) : Promise.resolve(null),
        ])
        setProjects(projectList.filter((p) => p.active))
        if (task) {
          setProjectId(task.project_id)
          setTitle(task.title)
          setSpec(task.spec)
          setPriority(task.priority)
        }
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load')
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [taskId])

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault()
    setSaving(true)
    setError(null)
    try {
      if (isEdit && taskId) {
        await updateTask(taskId, { title, spec, priority })
      } else {
        await createTask({ title, spec, project_id: projectId, priority, status })
      }
      onSave()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
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
    <form onSubmit={handleSubmit} className="space-y-5">
      {error && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {/* Project */}
      <div>
        <label className="mb-1.5 block text-sm font-medium text-zinc-700">Project</label>
        {isEdit ? (
          <p className="text-sm text-zinc-600">
            {projects.find((p) => p.id === projectId)?.name ?? projectId}
          </p>
        ) : (
          <select
            value={projectId}
            onChange={(e) => setProjectId(e.target.value)}
            required
            className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
          >
            <option value="">Select a project...</option>
            {projects.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        )}
      </div>

      {/* Title */}
      <div>
        <label className="mb-1.5 block text-sm font-medium text-zinc-700">Title</label>
        <input
          type="text"
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          required
          maxLength={500}
          placeholder="Task title"
          className="w-full rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
        />
      </div>

      {/* Spec */}
      <div>
        <div className="mb-1.5 flex items-center justify-between">
          <label className="text-sm font-medium text-zinc-700">Spec (Markdown)</label>
          <button
            type="button"
            onClick={() => setPreview(!preview)}
            className="inline-flex items-center gap-1 text-xs text-zinc-500 hover:text-zinc-700"
          >
            {preview ? (
              <EyeOff className="h-3.5 w-3.5" />
            ) : (
              <Eye className="h-3.5 w-3.5" />
            )}
            {preview ? 'Edit' : 'Preview'}
          </button>
        </div>
        {preview ? (
          <div className="min-h-[20rem] w-full rounded-md border border-zinc-300 bg-zinc-50 p-3 prose prose-sm prose-zinc max-w-none">
            <ReactMarkdown>{spec || '(empty)'}</ReactMarkdown>
          </div>
        ) : (
          <textarea
            value={spec}
            onChange={(e) => setSpec(e.target.value)}
            rows={20}
            placeholder="Task specification in markdown..."
            className="w-full rounded-md border border-zinc-300 px-3 py-2 font-mono text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
          />
        )}
      </div>

      {/* Priority */}
      <div>
        <label className="mb-1.5 block text-sm font-medium text-zinc-700">Priority</label>
        <input
          type="number"
          value={priority}
          onChange={(e) => setPriority(Number(e.target.value))}
          className="w-32 rounded-md border border-zinc-300 px-3 py-2 text-sm text-zinc-900 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
        />
        <p className="mt-1 text-xs text-zinc-400">Higher number = higher priority</p>
      </div>

      {/* Status (create only) */}
      {!isEdit && (
        <div>
          <label className="mb-1.5 block text-sm font-medium text-zinc-700">Status</label>
          <div className="flex gap-4">
            <label className="inline-flex items-center gap-2 text-sm text-zinc-700">
              <input
                type="radio"
                name="status"
                value="queued"
                checked={status === 'queued'}
                onChange={() => setStatus('queued')}
                className="text-zinc-900"
              />
              Queue immediately
            </label>
            <label className="inline-flex items-center gap-2 text-sm text-zinc-700">
              <input
                type="radio"
                name="status"
                value="pending"
                checked={status === 'pending'}
                onChange={() => setStatus('pending')}
                className="text-zinc-900"
              />
              Save as draft
            </label>
          </div>
        </div>
      )}

      {/* Buttons */}
      <div className="flex items-center gap-3 border-t border-zinc-200 pt-5">
        <button
          type="submit"
          disabled={saving}
          className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-4 py-2 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
        >
          {saving && <Loader2 className="h-4 w-4 animate-spin" />}
          {isEdit ? 'Save Changes' : 'Create Task'}
        </button>
        <button
          type="button"
          onClick={onCancel}
          className="rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium text-zinc-700 hover:bg-zinc-50"
        >
          Cancel
        </button>
      </div>
    </form>
  )
}
