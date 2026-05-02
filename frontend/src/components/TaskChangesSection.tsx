import { useState, useMemo, useEffect } from 'react'
import { ChevronRight, Loader2 } from 'lucide-react'
import { fetchTaskDiff, ApiError } from '../api/client'
import type { TaskDiff } from '../types'

interface Props {
  taskId: string
}

type DiffLineType = 'add' | 'remove' | 'context' | 'noeol'

interface DiffLine {
  type: DiffLineType
  text: string
}

interface DiffHunk {
  header: string
  lines: DiffLine[]
}

interface DiffFile {
  path: string
  oldPath: string
  isBinary: boolean
  additions: number
  deletions: number
  hunks: DiffHunk[]
}

// parseUnifiedDiff turns the raw unified diff text into per-file blocks. Files
// are delimited by `diff --git a/<old> b/<new>` headers; lines starting with
// `@@` open a hunk; subsequent lines are classified by their leading char.
// Binary files are flagged via the `Binary files ... differ` marker so the
// renderer can show a placeholder instead of trying to print bytes.
function parseUnifiedDiff(diff: string): DiffFile[] {
  const files: DiffFile[] = []
  if (!diff) return files

  let current: DiffFile | null = null
  let currentHunk: DiffHunk | null = null
  const flush = () => {
    if (current) files.push(current)
    current = null
    currentHunk = null
  }

  for (const line of diff.split('\n')) {
    const gitMatch = /^diff --git a\/(.+) b\/(.+)$/.exec(line)
    if (gitMatch) {
      flush()
      current = {
        path: gitMatch[2] ?? '',
        oldPath: gitMatch[1] ?? '',
        isBinary: false,
        additions: 0,
        deletions: 0,
        hunks: [],
      }
      continue
    }
    if (!current) continue

    if (
      line.startsWith('--- ') ||
      line.startsWith('+++ ') ||
      line.startsWith('index ') ||
      line.startsWith('similarity index ') ||
      line.startsWith('dissimilarity index ') ||
      line.startsWith('rename ') ||
      line.startsWith('copy ') ||
      line.startsWith('new file mode ') ||
      line.startsWith('deleted file mode ') ||
      line.startsWith('old mode ') ||
      line.startsWith('new mode ')
    ) {
      continue
    }

    if (line.startsWith('Binary files ') && line.endsWith(' differ')) {
      current.isBinary = true
      continue
    }

    if (line.startsWith('@@')) {
      currentHunk = { header: line, lines: [] }
      current.hunks.push(currentHunk)
      continue
    }

    if (!currentHunk) continue

    const ch = line.charAt(0)
    if (ch === '+') {
      current.additions++
      currentHunk.lines.push({ type: 'add', text: line.slice(1) })
    } else if (ch === '-') {
      current.deletions++
      currentHunk.lines.push({ type: 'remove', text: line.slice(1) })
    } else if (ch === ' ') {
      currentHunk.lines.push({ type: 'context', text: line.slice(1) })
    } else if (ch === '\\') {
      currentHunk.lines.push({ type: 'noeol', text: line })
    }
  }
  flush()
  return files
}

export default function TaskChangesSection({ taskId }: Props) {
  const [expanded, setExpanded] = useState(false)
  const [diff, setDiff] = useState<TaskDiff | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)

  // Lazy-load: only fetch the diff once, when the user first expands.
  useEffect(() => {
    if (!expanded || diff || loading || error) return
    let cancelled = false
    setLoading(true)
    fetchTaskDiff(taskId)
      .then(d => {
        if (!cancelled) setDiff(d)
      })
      .catch(err => {
        if (cancelled) return
        if (err instanceof ApiError && err.status === 404) {
          setError(err.message || 'commit not found in repository')
        } else {
          setError(err instanceof Error ? err.message : 'Failed to load diff')
        }
      })
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => { cancelled = true }
  }, [expanded, taskId, diff, loading, error])

  const files = useMemo(() => (diff ? parseUnifiedDiff(diff.diff) : []), [diff])

  const headerLabel = diff
    ? `${diff.stats.files_changed} ${diff.stats.files_changed === 1 ? 'file' : 'files'} changed · `
    : 'Changes'

  return (
    <div className="overflow-hidden rounded-lg border border-zinc-200 bg-zinc-50">
      <button
        onClick={() => setExpanded(e => !e)}
        className="flex w-full items-center gap-2 px-5 py-3 text-left hover:bg-zinc-100 cursor-pointer"
      >
        <ChevronRight
          size={14}
          className={`text-zinc-400 transition-transform duration-200 shrink-0 ${expanded ? 'rotate-90' : ''}`}
        />
        <span className="text-sm font-semibold uppercase tracking-wide text-zinc-500">
          {headerLabel}
        </span>
        {diff && (
          <span className="flex items-center gap-2 font-mono text-xs">
            <span className="text-emerald-700">+{diff.stats.insertions}</span>
            <span className="text-red-700">&minus;{diff.stats.deletions}</span>
          </span>
        )}
        {loading && <Loader2 className="ml-2 h-3.5 w-3.5 animate-spin text-zinc-400" />}
      </button>

      {expanded && (
        <div className="border-t border-zinc-200 bg-white">
          {error && (
            <div className="px-5 py-3 text-sm text-red-700">{error}</div>
          )}

          {!error && !loading && diff && diff.truncated && (
            <div className="border-b border-amber-200 bg-amber-50 px-5 py-2 text-xs text-amber-800">
              Diff too large &mdash; only first 5 MB shown.
            </div>
          )}

          {!error && !loading && diff && files.length === 0 && (
            <div className="px-5 py-3 text-sm text-zinc-500">No changes.</div>
          )}

          {!error && diff && files.length > 0 && (
            <div className="divide-y divide-zinc-200">
              {files.map(f => (
                <DiffFileBlock key={f.path} file={f} />
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

function DiffFileBlock({ file }: { file: DiffFile }) {
  const [expanded, setExpanded] = useState(true)
  const fileName = file.path.split('/').pop() || file.path

  return (
    <div>
      <button
        onClick={() => setExpanded(e => !e)}
        className="flex w-full items-center gap-2 bg-zinc-50 px-3 py-1.5 text-left hover:bg-zinc-100 cursor-pointer"
        title={file.oldPath !== file.path ? `${file.oldPath} → ${file.path}` : file.path}
      >
        <ChevronRight
          size={12}
          className={`text-zinc-400 transition-transform duration-200 shrink-0 ${expanded ? 'rotate-90' : ''}`}
        />
        <span className="font-mono text-xs text-zinc-700 truncate">{fileName}</span>
        <span className="ml-auto flex shrink-0 items-center gap-1.5 font-mono text-xs">
          {file.additions > 0 && <span className="text-emerald-700">+{file.additions}</span>}
          {file.deletions > 0 && <span className="text-red-700">&minus;{file.deletions}</span>}
        </span>
      </button>

      {expanded && (
        <div className="overflow-x-auto bg-zinc-50">
          {file.isBinary ? (
            <p className="px-3 py-2 text-xs italic text-zinc-500">Binary file changed</p>
          ) : (
            <table className="w-full font-mono text-xs leading-5">
              <tbody>
                {file.hunks.flatMap((hunk, hi) => [
                  <tr key={`h-${hi}`} className="bg-zinc-100 text-zinc-500">
                    <td className="select-none w-5 text-right pr-2 pl-3 align-top">&nbsp;</td>
                    <td className="pr-3 whitespace-pre-wrap break-all">{hunk.header}</td>
                  </tr>,
                  ...hunk.lines.map((line, li) => (
                    <tr
                      key={`h-${hi}-l-${li}`}
                      className={
                        line.type === 'add' ? 'bg-emerald-50'
                          : line.type === 'remove' ? 'bg-red-50'
                          : ''
                      }
                    >
                      <td className="select-none w-5 text-right pr-2 pl-3 text-zinc-400 align-top">
                        {line.type === 'add' ? '+' : line.type === 'remove' ? '−' : ' '}
                      </td>
                      <td
                        className={`pr-3 whitespace-pre-wrap break-all ${
                          line.type === 'add' ? 'text-emerald-800'
                            : line.type === 'remove' ? 'text-red-800'
                            : line.type === 'noeol' ? 'text-zinc-500 italic'
                            : 'text-zinc-700'
                        }`}
                      >
                        {line.text || ' '}
                      </td>
                    </tr>
                  )),
                ])}
              </tbody>
            </table>
          )}
        </div>
      )}
    </div>
  )
}
