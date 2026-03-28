import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, Link } from 'react-router-dom'
import { clsx } from 'clsx'
import {
  ArrowLeft,
  BarChart3,
  GitBranch,
  GitCommitHorizontal,
  ListTodo,
  Settings,
  Loader2,
  FolderGit2,
  CheckCircle2,
  XCircle,
  Clock,
  AlertTriangle,
  Save,
  Check,
  DollarSign,
  Timer,
  TrendingUp,
  Play,
  Rocket,
  Square,
} from 'lucide-react'
import {
  fetchProject,
  fetchProjectGitLog,
  fetchProjectGitStatus,
  fetchProjectStats,
  fetchProjectCommands,
  runProjectCommand,
  killProjectCommand,
  fetchTasks,
  updateProject,
} from '../api/client'
import { useRefreshOnFocus } from '../hooks/useRefreshOnFocus'
import { useDocumentTitle } from '../hooks/useDocumentTitle'
import type { Project, Task, BranchStrategy, GitCommit, GitStatus, ProjectStats, RunningCommandStatus } from '../types'

type TabId = 'stats' | 'git-status' | 'git-history' | 'tasks' | 'settings'

const tabs: { id: TabId; label: string; icon: typeof BarChart3 }[] = [
  { id: 'stats', label: 'Stats', icon: BarChart3 },
  { id: 'git-status', label: 'Git Status', icon: GitBranch },
  { id: 'git-history', label: 'Git History', icon: GitCommitHorizontal },
  { id: 'tasks', label: 'Tasks', icon: ListTodo },
  { id: 'settings', label: 'Settings', icon: Settings },
]

const statusColors: Record<string, string> = {
  queued: 'bg-blue-100 text-blue-700',
  running: 'bg-amber-100 text-amber-700',
  done: 'bg-emerald-100 text-emerald-700',
  failed: 'bg-red-100 text-red-700',
  pending: 'bg-zinc-100 text-zinc-600',
  needs_review: 'bg-purple-100 text-purple-700',
  cancelled: 'bg-zinc-100 text-zinc-400',
}

const statusLabels: Record<string, string> = {
  pending: 'Pending',
  queued: 'Queued',
  running: 'Running',
  done: 'Done',
  failed: 'Failed',
  needs_review: 'Needs Review',
  cancelled: 'Cancelled',
}

const statusIcons: Record<string, typeof CheckCircle2> = {
  done: CheckCircle2,
  failed: XCircle,
  running: Loader2,
  queued: Clock,
  pending: Clock,
  needs_review: AlertTriangle,
  cancelled: XCircle,
}

function formatDuration(ms: number): string {
  const seconds = Math.floor(ms / 1000)
  if (seconds < 60) return `${seconds}s`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m`
  const hours = Math.floor(minutes / 60)
  const remainMinutes = minutes % 60
  return `${hours}h ${remainMinutes}m`
}

function formatRelative(iso: string): string {
  const date = new Date(iso)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMin = Math.floor(diffMs / 60000)
  if (diffMin < 1) return 'just now'
  if (diffMin < 60) return `${diffMin}m ago`
  const diffH = Math.floor(diffMin / 60)
  if (diffH < 24) return `${diffH}h ago`
  const diffD = Math.floor(diffH / 24)
  return `${diffD}d ago`
}

// ── Stats Tab ──

function StatsTab({ projectId }: { projectId: string }) {
  const [stats, setStats] = useState<ProjectStats | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    fetchProjectStats(projectId)
      .then(setStats)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [projectId])

  if (loading) return <TabLoader />
  if (error) return <TabError message={error} />
  if (!stats) return null

  const statuses = ['pending', 'queued', 'running', 'done', 'failed', 'needs_review', 'cancelled']

  return (
    <div className="space-y-6">
      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        <StatCard
          icon={ListTodo}
          label="Total Tasks"
          value={String(stats.total)}
        />
        <StatCard
          icon={TrendingUp}
          label="Success Rate"
          value={stats.success_rate != null ? `${(stats.success_rate * 100).toFixed(0)}%` : '--'}
          color={stats.success_rate != null && stats.success_rate >= 0.8 ? 'text-emerald-600' : stats.success_rate != null ? 'text-amber-600' : undefined}
        />
        <StatCard
          icon={Timer}
          label="Avg Duration"
          value={stats.avg_duration_ms != null ? formatDuration(stats.avg_duration_ms) : '--'}
        />
        <StatCard
          icon={DollarSign}
          label="Total Cost"
          value={stats.total_cost_usd != null ? `$${stats.total_cost_usd.toFixed(2)}` : '--'}
        />
      </div>

      {/* Status breakdown */}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <h3 className="mb-4 text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Tasks by Status
        </h3>
        {stats.total === 0 ? (
          <p className="text-sm text-zinc-400">No tasks yet</p>
        ) : (
          <div className="space-y-2">
            {statuses.map((status) => {
              const count = stats.by_status[status] ?? 0
              if (count === 0) return null
              const pct = (count / stats.total) * 100
              return (
                <div key={status} className="flex items-center gap-3">
                  <span className="w-24 text-sm text-zinc-600">{statusLabels[status]}</span>
                  <div className="flex-1 h-5 bg-zinc-100 rounded-full overflow-hidden">
                    <div
                      className={clsx('h-full rounded-full', statusColors[status]?.split(' ')[0] || 'bg-zinc-300')}
                      style={{ width: `${pct}%` }}
                    />
                  </div>
                  <span className="w-10 text-right text-sm tabular-nums text-zinc-700">{count}</span>
                </div>
              )
            })}
          </div>
        )}
      </div>
    </div>
  )
}

function StatCard({
  icon: Icon,
  label,
  value,
  color,
}: {
  icon: typeof ListTodo
  label: string
  value: string
  color?: string
}) {
  return (
    <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-4">
      <div className="flex items-center gap-2 text-zinc-500">
        <Icon className="h-4 w-4" />
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
      </div>
      <p className={clsx('mt-2 text-2xl font-bold', color || 'text-zinc-900')}>{value}</p>
    </div>
  )
}

// ── Git Status Tab ──

function GitStatusTab({ projectId }: { projectId: string }) {
  const [status, setStatus] = useState<GitStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    fetchProjectGitStatus(projectId)
      .then(setStatus)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [projectId])

  if (loading) return <TabLoader />
  if (error) return <TabError message={error} />
  if (!status) return null

  return (
    <div className="space-y-4">
      {/* Branch + clean/dirty badge */}
      <div className="flex items-center gap-3">
        <div className="flex items-center gap-2">
          <GitBranch className="h-4 w-4 text-zinc-400" />
          <span className="font-mono text-sm font-medium text-zinc-900">{status.branch}</span>
        </div>
        <span
          className={clsx(
            'rounded-full px-2 py-0.5 text-xs font-medium',
            status.clean ? 'bg-emerald-100 text-emerald-700' : 'bg-amber-100 text-amber-700',
          )}
        >
          {status.clean ? 'Clean' : 'Dirty'}
        </span>
      </div>

      {/* Changed files */}
      {status.changed_files.length > 0 && (
        <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Changed Files ({status.changed_files.length})
          </h3>
          <div className="space-y-1">
            {status.changed_files.map((f, i) => (
              <div key={i} className="flex items-center gap-2 font-mono text-sm">
                <span
                  className={clsx(
                    'w-6 text-center text-xs font-bold',
                    f.status === 'M' && 'text-amber-600',
                    f.status === '??' && 'text-zinc-400',
                    f.status === 'A' && 'text-emerald-600',
                    f.status === 'D' && 'text-red-600',
                    f.status === 'R' && 'text-blue-600',
                  )}
                >
                  {f.status}
                </span>
                <span className="text-zinc-700">{f.path}</span>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Diff stat */}
      {status.diff_stat && (
        <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
          <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
            Diff Stat
          </h3>
          <pre className="whitespace-pre-wrap text-sm text-zinc-700 font-mono">{status.diff_stat}</pre>
        </div>
      )}

      {status.clean && status.changed_files.length === 0 && (
        <div className="flex flex-col items-center justify-center py-8 gap-2">
          <CheckCircle2 className="h-8 w-8 text-emerald-400" />
          <p className="text-sm text-zinc-500">Working tree is clean</p>
        </div>
      )}
    </div>
  )
}

// ── Git History Tab ──

function GitHistoryTab({ projectId }: { projectId: string }) {
  const [commits, setCommits] = useState<GitCommit[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    fetchProjectGitLog(projectId)
      .then(setCommits)
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [projectId])

  if (loading) return <TabLoader />
  if (error) return <TabError message={error} />

  if (commits.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 gap-2">
        <GitCommitHorizontal className="h-8 w-8 text-zinc-300" />
        <p className="text-sm text-zinc-400">No commit history</p>
      </div>
    )
  }

  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-zinc-200 text-left text-xs font-medium uppercase tracking-wide text-zinc-500">
            <th className="pb-2 pr-4">Hash</th>
            <th className="pb-2 pr-4">Message</th>
            <th className="pb-2 pr-4">Author</th>
            <th className="pb-2">Date</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-zinc-100">
          {commits.map((c) => (
            <tr key={c.hash} className="text-zinc-700">
              <td className="whitespace-nowrap py-2 pr-4 font-mono text-xs text-zinc-500">{c.hash}</td>
              <td className="py-2 pr-4 max-w-md truncate">{c.message}</td>
              <td className="whitespace-nowrap py-2 pr-4 text-zinc-500">{c.author}</td>
              <td className="whitespace-nowrap py-2 text-zinc-400">{formatRelative(c.date)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}

// ── Tasks Tab ──

function TasksTab({ projectId }: { projectId: string }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    setLoading(true)
    fetchTasks({ project_id: projectId })
      .then((resp) => setTasks(resp.data))
      .catch((e) => setError(e.message))
      .finally(() => setLoading(false))
  }, [projectId])

  if (loading) return <TabLoader />
  if (error) return <TabError message={error} />

  if (tasks.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center py-8 gap-2">
        <ListTodo className="h-8 w-8 text-zinc-300" />
        <p className="text-sm text-zinc-400">No tasks for this project</p>
      </div>
    )
  }

  // Sort: running first, then queued, pending, needs_review, failed, done, cancelled
  const order: Record<string, number> = { running: 0, queued: 1, pending: 2, needs_review: 3, failed: 4, done: 5, cancelled: 6 }
  const sorted = [...tasks].sort((a, b) => (order[a.status] ?? 9) - (order[b.status] ?? 9))

  return (
    <div className="space-y-2">
      {sorted.map((task) => {
        const Icon = statusIcons[task.status] || Clock
        return (
          <Link
            key={task.id}
            to={`/tasks/${task.id}`}
            className="flex items-center gap-3 rounded-lg border border-zinc-200 bg-zinc-50 px-4 py-3 hover:bg-zinc-100 transition-colors"
          >
            <Icon
              className={clsx(
                'h-4 w-4 shrink-0',
                task.status === 'done' && 'text-emerald-500',
                task.status === 'failed' && 'text-red-500',
                task.status === 'running' && 'text-amber-500 animate-spin',
                task.status === 'queued' && 'text-blue-500',
                task.status === 'pending' && 'text-zinc-400',
                task.status === 'needs_review' && 'text-purple-500',
                task.status === 'cancelled' && 'text-zinc-400',
              )}
            />
            <div className="flex-1 min-w-0">
              <p className="text-sm font-medium text-zinc-900 truncate">{task.title}</p>
              <p className="text-xs text-zinc-400">{formatRelative(task.created_at)}</p>
            </div>
            <span
              className={clsx(
                'rounded-full px-2 py-0.5 text-xs font-medium',
                statusColors[task.status],
              )}
            >
              {statusLabels[task.status]}
            </span>
          </Link>
        )
      })}
    </div>
  )
}

// ── Settings Tab ──

function SettingsTab({ project, onSaved }: { project: Project; onSaved: () => void }) {
  const [branchStrategy, setBranchStrategy] = useState<BranchStrategy>(project.branch_strategy)
  const [verificationCommand, setVerificationCommand] = useState(
    project.verification_command ?? '',
  )
  const [devCommand, setDevCommand] = useState(project.dev_command ?? '')
  const [devPort, setDevPort] = useState(project.dev_port?.toString() ?? '')
  const [deployCommand, setDeployCommand] = useState(project.deploy_command ?? '')
  const [deployPort, setDeployPort] = useState(project.deploy_port?.toString() ?? '')
  const [saving, setSaving] = useState(false)
  const [saved, setSaved] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const devPortNum = devPort ? parseInt(devPort, 10) : undefined
  const deployPortNum = deployPort ? parseInt(deployPort, 10) : undefined

  const hasChanges =
    branchStrategy !== project.branch_strategy ||
    (verificationCommand || null) !== (project.verification_command ?? null) ||
    (devCommand || null) !== (project.dev_command ?? null) ||
    (deployCommand || null) !== (project.deploy_command ?? null) ||
    (devPortNum ?? null) !== (project.dev_port ?? null) ||
    (deployPortNum ?? null) !== (project.deploy_port ?? null)

  async function handleSave() {
    try {
      setSaving(true)
      setError(null)
      await updateProject(project.id, {
        branch_strategy: branchStrategy,
        verification_command: verificationCommand || undefined,
        dev_command: devCommand || undefined,
        deploy_command: deployCommand || undefined,
        dev_port: devPortNum ?? 0,
        deploy_port: deployPortNum ?? 0,
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
    <div className="space-y-6">
      {/* Project info */}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
        <h3 className="mb-3 text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Project Info
        </h3>
        <dl className="space-y-2 text-sm">
          <div className="flex gap-2">
            <dt className="text-zinc-500 w-20">Path</dt>
            <dd className="text-zinc-700 font-mono">{project.path}</dd>
          </div>
          <div className="flex gap-2">
            <dt className="text-zinc-500 w-20">Status</dt>
            <dd>{project.active ? <span className="text-emerald-600">Active</span> : <span className="text-zinc-400">Inactive</span>}</dd>
          </div>
        </dl>
      </div>

      {/* Branch Strategy */}
      <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5 space-y-4">
        <h3 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          Configuration
        </h3>

        <div>
          <label className="text-sm font-medium text-zinc-700">Branch Strategy</label>
          <div className="mt-1.5 flex gap-4">
            <label className="flex items-center gap-2 text-sm text-zinc-600 cursor-pointer">
              <input
                type="radio"
                name="branch-strategy"
                checked={branchStrategy === 'main'}
                onChange={() => setBranchStrategy('main')}
                className="accent-zinc-900"
              />
              Commit to main
            </label>
            <label className="flex items-center gap-2 text-sm text-zinc-600 cursor-pointer">
              <input
                type="radio"
                name="branch-strategy"
                checked={branchStrategy === 'feature_branch'}
                onChange={() => setBranchStrategy('feature_branch')}
                className="accent-zinc-900"
              />
              Feature branches + PR
            </label>
          </div>
        </div>

        <div>
          <label htmlFor="verify-cmd" className="text-sm font-medium text-zinc-700">
            Verification Command
          </label>
          <input
            id="verify-cmd"
            type="text"
            value={verificationCommand}
            onChange={(e) => setVerificationCommand(e.target.value)}
            placeholder="e.g., make test, go test ./..."
            className="mt-1 w-full rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
          />
        </div>

        <div>
          <label htmlFor="dev-cmd" className="text-sm font-medium text-zinc-700">
            Dev Command
          </label>
          <div className="mt-1 flex gap-2">
            <input
              id="dev-cmd"
              type="text"
              value={devCommand}
              onChange={(e) => setDevCommand(e.target.value)}
              placeholder="e.g., make frontend-dev"
              className="flex-1 rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
            <input
              id="dev-port"
              type="number"
              value={devPort}
              onChange={(e) => setDevPort(e.target.value)}
              placeholder="Port"
              min="1"
              max="65535"
              className="w-20 rounded-md border border-zinc-300 bg-zinc-50 px-2 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
          </div>
        </div>

        <div>
          <label htmlFor="deploy-cmd" className="text-sm font-medium text-zinc-700">
            Deploy Command
          </label>
          <div className="mt-1 flex gap-2">
            <input
              id="deploy-cmd"
              type="text"
              value={deployCommand}
              onChange={(e) => setDeployCommand(e.target.value)}
              placeholder="e.g., make deploy"
              className="flex-1 rounded-md border border-zinc-300 bg-zinc-50 px-3 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
            <input
              id="deploy-port"
              type="number"
              value={deployPort}
              onChange={(e) => setDeployPort(e.target.value)}
              placeholder="Port"
              min="1"
              max="65535"
              className="w-20 rounded-md border border-zinc-300 bg-zinc-50 px-2 py-1.5 text-sm text-zinc-900 placeholder:text-zinc-400 focus:border-zinc-500 focus:outline-none focus:ring-1 focus:ring-zinc-500"
            />
          </div>
        </div>

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
        </div>

        {error && <p className="text-sm text-red-500">{error}</p>}
      </div>
    </div>
  )
}

// ── Shared Components ──

function TabLoader() {
  return (
    <div className="flex h-32 items-center justify-center">
      <Loader2 className="h-5 w-5 animate-spin text-zinc-400" />
    </div>
  )
}

function TabError({ message }: { message: string }) {
  return (
    <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{message}</div>
  )
}

// ── Commands Section ──

function CommandsSection({ project }: { project: Project }) {
  const [commands, setCommands] = useState<RunningCommandStatus[]>([])
  const [toast, setToast] = useState<string | null>(null)
  const [confirmDeploy, setConfirmDeploy] = useState(false)
  const intervalRef = useRef<ReturnType<typeof setInterval> | null>(null)

  const hasDevCommand = !!project.dev_command
  const hasDeployCommand = !!project.deploy_command

  const loadCommands = useCallback(async () => {
    try {
      const cmds = await fetchProjectCommands(project.id)
      setCommands(cmds)
    } catch {
      // ignore polling errors
    }
  }, [project.id])

  useEffect(() => {
    if (!hasDevCommand && !hasDeployCommand) return
    loadCommands()
    intervalRef.current = setInterval(loadCommands, 5000)
    return () => {
      if (intervalRef.current) clearInterval(intervalRef.current)
    }
  }, [loadCommands, hasDevCommand, hasDeployCommand])

  if (!hasDevCommand && !hasDeployCommand) return null

  const runningDev = commands.find((c) => c.command_type === 'dev')
  const runningDeploy = commands.find((c) => c.command_type === 'deploy')

  async function handleRun(type: 'dev' | 'deploy') {
    try {
      const result = await runProjectCommand(project.id, type)
      setToast(`${type === 'dev' ? 'Dev' : 'Deploy'} started (PID ${result.pid})`)
      setTimeout(() => setToast(null), 3000)
      loadCommands()
    } catch (err) {
      setToast(err instanceof Error ? err.message : 'Failed to start command')
      setTimeout(() => setToast(null), 4000)
    }
  }

  async function handleKill(pid: number) {
    try {
      await killProjectCommand(project.id, pid)
      setToast('Process stopped')
      setTimeout(() => setToast(null), 3000)
      loadCommands()
    } catch {
      setToast('Failed to stop process')
      setTimeout(() => setToast(null), 4000)
    }
  }

  return (
    <div className="flex flex-wrap items-center gap-2">
      {/* Dev button */}
      {hasDevCommand && (
        runningDev ? (
          <button
            onClick={() => handleKill(runningDev.pid)}
            className="inline-flex items-center gap-1.5 rounded-md bg-amber-100 px-3 py-1.5 text-sm font-medium text-amber-800 hover:bg-amber-200 transition-colors"
          >
            <Square className="h-3.5 w-3.5" />
            Stop Dev{runningDev.port ? ` :${runningDev.port}` : ''} (PID {runningDev.pid})
          </button>
        ) : (
          <button
            onClick={() => handleRun('dev')}
            className="inline-flex items-center gap-1.5 rounded-md bg-emerald-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-emerald-700 transition-colors"
          >
            <Play className="h-3.5 w-3.5" />
            Start Dev
          </button>
        )
      )}

      {/* Deploy button */}
      {hasDeployCommand && (
        runningDeploy ? (
          <button
            onClick={() => handleKill(runningDeploy.pid)}
            className="inline-flex items-center gap-1.5 rounded-md bg-amber-100 px-3 py-1.5 text-sm font-medium text-amber-800 hover:bg-amber-200 transition-colors"
          >
            <Square className="h-3.5 w-3.5" />
            Stop Deploy{runningDeploy.port ? ` :${runningDeploy.port}` : ''} (PID {runningDeploy.pid})
          </button>
        ) : (
          <button
            onClick={() => setConfirmDeploy(true)}
            className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700 transition-colors"
          >
            <Rocket className="h-3.5 w-3.5" />
            Deploy
          </button>
        )
      )}

      {/* Toast */}
      {toast && (
        <span className="rounded-md bg-zinc-800 px-3 py-1.5 text-sm text-white">
          {toast}
        </span>
      )}

      {/* Deploy confirmation dialog */}
      {confirmDeploy && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/50" onClick={() => setConfirmDeploy(false)}>
          <div className="mx-4 w-full max-w-md rounded-lg bg-white p-6 shadow-xl" onClick={(e) => e.stopPropagation()}>
            <h3 className="text-lg font-semibold text-zinc-900">Confirm Deploy</h3>
            <p className="mt-2 text-sm text-zinc-600">
              This will execute the following command:
            </p>
            <pre className="mt-2 rounded-md bg-zinc-100 px-3 py-2 text-sm font-mono text-zinc-800">
              {project.deploy_command}
            </pre>
            <div className="mt-4 flex justify-end gap-3">
              <button
                onClick={() => setConfirmDeploy(false)}
                className="rounded-md px-3 py-1.5 text-sm font-medium text-zinc-600 hover:bg-zinc-100"
              >
                Cancel
              </button>
              <button
                onClick={() => {
                  setConfirmDeploy(false)
                  handleRun('deploy')
                }}
                className="rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700"
              >
                Deploy
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

// ── Main Page ──

export default function ProjectDetailPage() {
  const { id } = useParams<{ id: string }>()
  const [project, setProject] = useState<Project | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<TabId>('stats')

  const load = useCallback(async () => {
    try {
      const p = await fetchProject(id!)
      setProject(p)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load project')
    } finally {
      setLoading(false)
    }
  }, [id])

  useEffect(() => {
    load()
  }, [load])

  useRefreshOnFocus(load)
  useDocumentTitle(project?.name || 'Project')

  if (loading) {
    return (
      <div className="flex h-48 items-center justify-center">
        <Loader2 className="h-6 w-6 animate-spin text-zinc-400" />
      </div>
    )
  }

  if (error && !project) {
    return (
      <div className="mx-auto max-w-5xl">
        <Link
          to="/projects"
          className="mb-4 inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-700"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to projects
        </Link>
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      </div>
    )
  }

  if (!project) return null

  return (
    <div className="mx-auto max-w-5xl space-y-6">
      {/* Back link + header */}
      <div>
        <Link
          to="/projects"
          className="inline-flex items-center gap-1 text-sm text-zinc-500 hover:text-zinc-700"
        >
          <ArrowLeft className="h-4 w-4" />
          Back to projects
        </Link>
        <div className="mt-3 flex items-center gap-3">
          <FolderGit2 className="h-6 w-6 text-zinc-400" />
          <h1 className="text-2xl font-bold text-zinc-900">{project.name}</h1>
          <span
            className={clsx(
              'rounded-full px-2 py-0.5 text-xs font-medium',
              project.active ? 'bg-emerald-100 text-emerald-700' : 'bg-zinc-100 text-zinc-400',
            )}
          >
            {project.active ? 'Active' : 'Inactive'}
          </span>
        </div>
        <p className="mt-1 text-sm text-zinc-400 font-mono">{project.path}</p>

        {/* Command buttons */}
        <div className="mt-3">
          <CommandsSection project={project} />
        </div>
      </div>

      {/* Tabs */}
      <div className="border-b border-zinc-200">
        <nav className="-mb-px flex gap-4 overflow-x-auto" aria-label="Tabs">
          {tabs.map(({ id: tabId, label, icon: Icon }) => (
            <button
              key={tabId}
              onClick={() => setActiveTab(tabId)}
              className={clsx(
                'flex items-center gap-1.5 whitespace-nowrap border-b-2 px-1 py-2.5 text-sm font-medium transition-colors',
                activeTab === tabId
                  ? 'border-zinc-900 text-zinc-900'
                  : 'border-transparent text-zinc-500 hover:border-zinc-300 hover:text-zinc-700',
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
        {activeTab === 'stats' && <StatsTab projectId={project.id} />}
        {activeTab === 'git-status' && <GitStatusTab projectId={project.id} />}
        {activeTab === 'git-history' && <GitHistoryTab projectId={project.id} />}
        {activeTab === 'tasks' && <TasksTab projectId={project.id} />}
        {activeTab === 'settings' && <SettingsTab project={project} onSaved={load} />}
      </div>
    </div>
  )
}
