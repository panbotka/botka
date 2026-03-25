import { useState, useMemo } from 'react'
import { Link } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  ChevronDown,
  ChevronRight,
  FolderGit2,
  Loader2,
  ScanSearch,
  Check,
  Save,
} from 'lucide-react'

import { useProjects } from '../hooks/useProjects'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { updateProject } from '../api/client'
import type { Project, BranchStrategy, TaskStatus } from '../types'

interface ScanResult {
  discovered: number
  new: number
  deactivated: number
}

const statusColors: Record<string, string> = {
  queued: 'bg-blue-100 text-blue-700',
  running: 'bg-amber-100 text-amber-700',
  done: 'bg-emerald-100 text-emerald-700',
  failed: 'bg-red-100 text-red-700',
  pending: 'bg-zinc-100 text-zinc-600',
  needs_review: 'bg-purple-100 text-purple-700',
}

const statusOrder: TaskStatus[] = ['queued', 'running', 'done', 'failed', 'pending', 'needs_review']

function TaskCountBadges({ counts }: { counts: Record<string, number> | undefined }) {
  if (!counts) return null

  const parts = statusOrder
    .filter((s) => (counts[s] ?? 0) > 0)
    .map((s) => ({
      status: s,
      count: counts[s]!,
      label: s === 'needs_review' ? 'review' : s,
    }))

  if (parts.length === 0) {
    return <span className="text-xs text-zinc-400">No tasks</span>
  }

  return (
    <div className="flex flex-wrap gap-1">
      {parts.map(({ status, count, label }) => (
        <span
          key={status}
          className={clsx('rounded-full px-1.5 py-0.5 text-xs tabular-nums', statusColors[status])}
        >
          {count} {label}
        </span>
      ))}
    </div>
  )
}

function ProjectConfigEditor({
  project,
  onSaved,
}: {
  project: Project
  onSaved: () => void
}) {
  const [branchStrategy, setBranchStrategy] = useState<BranchStrategy>(project.branch_strategy)
  const [verificationCommand, setVerificationCommand] = useState(
    project.verification_command ?? '',
  )
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const hasChanges =
    branchStrategy !== project.branch_strategy ||
    (verificationCommand || null) !== (project.verification_command ?? null)

  async function handleSave() {
    try {
      setSaving(true)
      setError(null)
      await updateProject(project.id, {
        branch_strategy: branchStrategy,
        verification_command: verificationCommand || undefined,
      })
      setSaved(true)
      setTimeout(() => setSaved(false), 2000)
      onSaved()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to save')
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="border-t border-zinc-100 bg-zinc-50/50 px-5 py-4 space-y-4">
      {/* Branch Strategy */}
      <div>
        <label className="text-sm font-medium text-zinc-700">Branch Strategy</label>
        <div className="mt-1.5 flex gap-4">
          <label className="flex items-center gap-2 text-sm text-zinc-600 cursor-pointer">
            <input
              type="radio"
              name={`branch-${project.id}`}
              checked={branchStrategy === 'main'}
              onChange={() => setBranchStrategy('main')}
              className="accent-zinc-900"
            />
            Commit to main
          </label>
          <label className="flex items-center gap-2 text-sm text-zinc-600 cursor-pointer">
            <input
              type="radio"
              name={`branch-${project.id}`}
              checked={branchStrategy === 'feature_branch'}
              onChange={() => setBranchStrategy('feature_branch')}
              className="accent-zinc-900"
            />
            Feature branches + PR
          </label>
        </div>
      </div>

      {/* Verification Command */}
      <div>
        <label htmlFor={`verify-${project.id}`} className="text-sm font-medium text-zinc-700">
          Verification Command
        </label>
        <input
          id={`verify-${project.id}`}
          type="text"
          value={verificationCommand}
          onChange={(e) => setVerificationCommand(e.target.value)}
          placeholder="e.g., make test, go test ./..."
          className="mt-1 w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
        />
      </div>

      {/* Actions */}
      <div className="flex items-center gap-3">
        <button
          onClick={handleSave}
          disabled={saving || !hasChanges}
          className={clsx(
            'inline-flex items-center gap-1.5 rounded-md px-3 py-1.5 text-sm font-medium transition-colors',
            saved
              ? 'bg-emerald-600 text-white'
              : hasChanges
                ? 'bg-zinc-900 text-white hover:bg-zinc-800'
                : 'bg-zinc-200 text-zinc-400 cursor-not-allowed',
          )}
        >
          {saving ? (
            <Loader2 className="h-3.5 w-3.5 animate-spin" />
          ) : saved ? (
            <Check className="h-3.5 w-3.5" />
          ) : (
            <Save className="h-3.5 w-3.5" />
          )}
          {saved ? 'Saved' : 'Save'}
        </button>

        <Link
          to={`/tasks?project=${project.id}`}
          className="text-sm text-zinc-500 hover:text-zinc-700"
        >
          View Tasks
        </Link>
      </div>

      {error && <p className="text-sm text-red-500">{error}</p>}
    </div>
  )
}

function ProjectRow({
  project,
  onSaved,
}: {
  project: Project
  onSaved: () => void
}) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div className="rounded-lg border border-zinc-200 bg-zinc-50 overflow-hidden">
      <button
        onClick={() => setExpanded(!expanded)}
        className="flex w-full items-center gap-3 px-5 py-3.5 text-left hover:bg-zinc-50 transition-colors"
      >
        {expanded ? (
          <ChevronDown className="h-4 w-4 shrink-0 text-zinc-400" />
        ) : (
          <ChevronRight className="h-4 w-4 shrink-0 text-zinc-400" />
        )}

        <FolderGit2 className="h-4 w-4 shrink-0 text-zinc-400" />

        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2">
            <span className="font-medium text-zinc-900 truncate">{project.name}</span>
            <span
              className={clsx(
                'rounded-full px-1.5 py-0.5 text-xs',
                project.branch_strategy === 'feature_branch'
                  ? 'bg-blue-100 text-blue-700'
                  : 'bg-zinc-100 text-zinc-500',
              )}
            >
              {project.branch_strategy === 'feature_branch' ? 'feature_branch' : 'main'}
            </span>
            {!project.active && (
              <span className="rounded-full bg-zinc-100 px-1.5 py-0.5 text-xs text-zinc-400">
                Inactive
              </span>
            )}
          </div>
          <p className="mt-0.5 truncate text-xs text-zinc-400">{project.path}</p>
        </div>

        <div className="shrink-0">
          <TaskCountBadges counts={project.task_counts} />
        </div>
      </button>

      {expanded && <ProjectConfigEditor project={project} onSaved={onSaved} />}
    </div>
  )
}

export default function ProjectsPage() {
  const { projects, loading, error, refetch, scan, scanning } = useProjects()
  useRefreshOnFocus(refetch)
  const [scanResult, setScanResult] = useState<ScanResult | null>(null)

  const sorted = useMemo(() => {
    const active = projects.filter((p) => p.active)
    const inactive = projects.filter((p) => !p.active)
    active.sort((a, b) => a.name.localeCompare(b.name))
    inactive.sort((a, b) => a.name.localeCompare(b.name))
    return [...active, ...inactive]
  }, [projects])

  async function handleScan() {
    setScanResult(null)
    const result = await scan()
    if (result) {
      setScanResult(result)
      setTimeout(() => setScanResult(null), 5000)
    }
  }

  return (
    <div className="mx-auto max-w-5xl space-y-5">
      {/* Header */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-3">
          <h1 className="text-2xl font-bold text-zinc-900">Projects</h1>
          {!loading && (
            <span className="rounded-full bg-zinc-100 px-2 py-0.5 text-xs tabular-nums text-zinc-500">
              {projects.length}
            </span>
          )}
        </div>
        <button
          onClick={handleScan}
          disabled={scanning}
          className="inline-flex items-center gap-1.5 rounded-md bg-zinc-900 px-3 py-1.5 text-sm font-medium text-white hover:bg-zinc-800 disabled:opacity-50"
        >
          {scanning ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <ScanSearch className="h-4 w-4" />
          )}
          Scan for Projects
        </button>
      </div>

      {/* Scan result flash */}
      {scanResult && (
        <div className="rounded-md bg-emerald-50 border border-emerald-200 px-4 py-2.5 text-sm text-emerald-700">
          Found {scanResult.discovered} projects
          {scanResult.new > 0 && <>, <strong>{scanResult.new} new</strong></>}
          {scanResult.deactivated > 0 && <>, {scanResult.deactivated} deactivated</>}
        </div>
      )}

      {/* Content */}
      {loading ? (
        <div className="flex h-48 items-center justify-center">
          <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
        </div>
      ) : error ? (
        <div className="flex h-48 items-center justify-center">
          <p className="text-sm text-red-500">{error}</p>
        </div>
      ) : sorted.length === 0 ? (
        <div className="flex h-48 flex-col items-center justify-center gap-2">
          <FolderGit2 className="h-8 w-8 text-zinc-300" />
          <p className="text-sm text-zinc-400">No projects found. Run a scan to discover them.</p>
        </div>
      ) : (
        <div className="space-y-2">
          {sorted.map((project) => (
            <ProjectRow key={project.id} project={project} onSaved={refetch} />
          ))}
        </div>
      )}
    </div>
  )
}
