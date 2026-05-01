import { useEffect, useMemo, useState } from 'react'
import { html as renderDiff2Html } from 'diff2html'
import 'diff2html/bundles/css/diff2html.min.css'
import { Loader2, FileText } from 'lucide-react'
import { fetchTaskDiff, ApiError } from '../api/client'
import type { TaskDiff, TaskDiffFile, TaskDiffFileStatus } from '../types'

interface Props {
  taskId: string
  baseCommitSha: string
  headCommitSha: string
}

const LARGE_DIFF_LINE_THRESHOLD = 5000

const statusColor: Record<TaskDiffFileStatus, string> = {
  added: 'text-emerald-700',
  deleted: 'text-red-700',
  modified: 'text-zinc-700',
  renamed: 'text-blue-700',
  copied: 'text-blue-700',
  typechange: 'text-amber-700',
}

const statusBadge: Record<TaskDiffFileStatus, string> = {
  added: 'A',
  deleted: 'D',
  modified: 'M',
  renamed: 'R',
  copied: 'C',
  typechange: 'T',
}

// splitDiffByPath partitions a unified diff string into per-file chunks keyed
// by the post-image path. Lines in a file's chunk include the leading
// "diff --git" header so diff2html can render the file correctly on its own.
function splitDiffByPath(rawDiff: string): Map<string, string> {
  const result = new Map<string, string>()
  if (!rawDiff) return result
  const lines = rawDiff.split('\n')
  let currentPath = ''
  let currentLines: string[] = []
  const flush = () => {
    if (currentPath) result.set(currentPath, currentLines.join('\n'))
  }
  for (const line of lines) {
    const m = /^diff --git a\/(.+) b\/(.+)$/.exec(line)
    if (m && m[2]) {
      flush()
      currentPath = m[2]
      currentLines = [line]
    } else if (currentPath) {
      currentLines.push(line)
    }
  }
  flush()
  return result
}

export default function TaskChangesSection({ taskId, baseCommitSha, headCommitSha }: Props) {
  const [diff, setDiff] = useState<TaskDiff | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [missing, setMissing] = useState(false)
  const [selectedPath, setSelectedPath] = useState<string | null>(null)
  const [showFullDiff, setShowFullDiff] = useState(false)

  useEffect(() => {
    let cancelled = false
    setLoading(true)
    setError(null)
    setMissing(false)
    fetchTaskDiff(taskId)
      .then(d => {
        if (cancelled) return
        setDiff(d)
        setSelectedPath(d.files[0]?.path ?? null)
      })
      .catch(err => {
        if (cancelled) return
        if (err instanceof ApiError && err.status === 404) {
          setMissing(true)
        } else {
          setError(err instanceof Error ? err.message : 'Failed to load diff')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [taskId, baseCommitSha, headCommitSha])

  const perFileDiff = useMemo(
    () => (diff ? splitDiffByPath(diff.diff) : new Map<string, string>()),
    [diff],
  )

  const totalLines = useMemo(() => (diff ? diff.diff.split('\n').length : 0), [diff])
  const isLarge = totalLines > LARGE_DIFF_LINE_THRESHOLD

  const selectedDiffHtml = useMemo(() => {
    if (!selectedPath) return ''
    const chunk = perFileDiff.get(selectedPath)
    if (!chunk) return ''
    return renderDiff2Html(chunk, {
      drawFileList: false,
      matching: 'lines',
      outputFormat: 'line-by-line',
      colorScheme: 'light' as never,
    })
  }, [selectedPath, perFileDiff])

  if (missing) return null

  return (
    <div className="rounded-lg border border-zinc-200 bg-zinc-50 p-5">
      <div className="mb-3 flex items-center justify-between">
        <h2 className="text-sm font-semibold uppercase tracking-wide text-zinc-500">Changes</h2>
        {diff && (
          <span className="font-mono text-xs text-zinc-500">
            {diff.base_commit_sha.slice(0, 7)}..{diff.head_commit_sha.slice(0, 7)}
          </span>
        )}
      </div>

      {loading && (
        <div className="flex items-center gap-2 text-sm text-zinc-500">
          <Loader2 className="h-4 w-4 animate-spin" /> Loading diff…
        </div>
      )}

      {error && (
        <div className="rounded-md bg-red-50 px-4 py-3 text-sm text-red-700">{error}</div>
      )}

      {diff && diff.files.length === 0 && (
        <p className="text-sm text-zinc-400">No file changes between commits.</p>
      )}

      {diff && diff.files.length > 0 && (
        <>
          {isLarge && !showFullDiff ? (
            <CollapsedDiffSummary
              files={diff.files}
              totalLines={totalLines}
              onShow={() => setShowFullDiff(true)}
            />
          ) : (
            <div className="grid grid-cols-1 gap-4 md:grid-cols-[minmax(0,18rem)_minmax(0,1fr)]">
              <FileTree
                files={diff.files}
                selected={selectedPath}
                onSelect={setSelectedPath}
              />
              <div className="overflow-x-auto rounded border border-zinc-200 bg-white">
                {selectedDiffHtml ? (
                  <div
                    className="task-diff-view text-xs"
                    dangerouslySetInnerHTML={{ __html: selectedDiffHtml }}
                  />
                ) : (
                  <p className="p-4 text-sm text-zinc-400">Select a file to view its diff.</p>
                )}
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}

function CollapsedDiffSummary({
  files, totalLines, onShow,
}: { files: TaskDiffFile[]; totalLines: number; onShow: () => void }) {
  const totals = files.reduce(
    (acc, f) => ({ adds: acc.adds + f.additions, dels: acc.dels + f.deletions }),
    { adds: 0, dels: 0 },
  )
  return (
    <div className="rounded border border-zinc-200 bg-white p-4 text-sm text-zinc-600">
      <p className="mb-3">
        Diff is large — {files.length.toLocaleString('en-US')} files,{' '}
        <span className="text-emerald-700">+{totals.adds.toLocaleString('en-US')}</span>{' '}
        <span className="text-red-700">−{totals.dels.toLocaleString('en-US')}</span>,{' '}
        {totalLines.toLocaleString('en-US')} lines total.
      </p>
      <button
        onClick={onShow}
        className="inline-flex items-center gap-1.5 rounded-md bg-blue-600 px-3 py-1.5 text-sm font-medium text-white hover:bg-blue-700"
      >
        Show full diff
      </button>
    </div>
  )
}

function FileTree({
  files, selected, onSelect,
}: { files: TaskDiffFile[]; selected: string | null; onSelect: (path: string) => void }) {
  return (
    <ul className="max-h-[32rem] overflow-y-auto rounded border border-zinc-200 bg-white p-1 text-sm">
      {files.map(f => {
        const isSelected = f.path === selected
        return (
          <li key={f.path}>
            <button
              onClick={() => onSelect(f.path)}
              className={`flex w-full items-center gap-2 rounded px-2 py-1 text-left transition-colors ${
                isSelected ? 'bg-blue-50 text-blue-900' : 'hover:bg-zinc-100 text-zinc-700'
              }`}
              title={f.old_path ? `${f.old_path} → ${f.path}` : f.path}
            >
              <span
                className={`inline-flex h-5 w-5 shrink-0 items-center justify-center rounded font-mono text-[11px] font-semibold ${statusColor[f.status]}`}
                aria-label={f.status}
              >
                {statusBadge[f.status]}
              </span>
              <FileText className="h-3.5 w-3.5 shrink-0 text-zinc-400" />
              <span className="min-w-0 truncate font-mono text-xs">{f.path}</span>
              <span className="ml-auto flex shrink-0 gap-1 font-mono text-[11px] tabular-nums">
                {f.additions > 0 && <span className="text-emerald-700">+{f.additions}</span>}
                {f.deletions > 0 && <span className="text-red-700">−{f.deletions}</span>}
              </span>
            </button>
          </li>
        )
      })}
    </ul>
  )
}
