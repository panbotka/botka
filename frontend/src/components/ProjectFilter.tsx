import { useEffect, useMemo, useRef, useState } from 'react'
import { clsx } from 'clsx'
import { Check, ChevronDown, FolderOpen, Search } from 'lucide-react'
import { fuzzyScore } from './ProjectPicker'

// Strips diacritics so "kukatko" finds "Kukátko".
function fold(s: string): string {
  return s.normalize('NFD').replace(/[̀-ͯ]/g, '')
}

// filterProjectNames ranks names by fuzzy match (prefix > substring > in-order
// characters), keeping the incoming order within a rank. An empty query keeps
// every name.
export function filterProjectNames(names: string[], query: string): string[] {
  const q = fold(query.trim())
  if (!q) return names
  return names
    .map((name, i) => ({ name, i, score: fuzzyScore(q, fold(name)) }))
    .filter((r) => r.score >= 0)
    .sort((a, b) => b.score - a.score || a.i - b.i)
    .map((r) => r.name)
}

interface Props {
  names: string[]
  value: string | null
  onChange: (name: string | null) => void
}

// ProjectFilter is a searchable project dropdown for the task list. The first
// row is always "All projects"; typing filters the rest, ↑/↓ move, Enter picks,
// Esc closes.
export default function ProjectFilter({ names, value, onChange }: Props) {
  const [open, setOpen] = useState(false)
  const [query, setQuery] = useState('')
  const [highlight, setHighlight] = useState(0)
  const containerRef = useRef<HTMLDivElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLUListElement>(null)

  const matches = useMemo(() => filterProjectNames(names, query), [names, query])
  // Row 0 is "All projects" (null), hidden while searching so Enter picks the best match.
  const options: (string | null)[] = query.trim() ? matches : [null, ...matches]

  useEffect(() => {
    if (!open) return
    const handle = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) setOpen(false)
    }
    document.addEventListener('mousedown', handle)
    return () => document.removeEventListener('mousedown', handle)
  }, [open])

  useEffect(() => {
    if (open) {
      requestAnimationFrame(() => inputRef.current?.focus())
    } else {
      setQuery('')
    }
  }, [open])

  useEffect(() => setHighlight(0), [query])

  useEffect(() => {
    listRef.current?.children[highlight]?.scrollIntoView?.({ block: 'nearest' })
  }, [highlight])

  function pick(name: string | null) {
    onChange(name)
    setOpen(false)
  }

  function handleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setHighlight((h) => Math.min(h + 1, options.length - 1))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setHighlight((h) => Math.max(h - 1, 0))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (highlight < options.length) pick(options[highlight] ?? null)
    } else if (e.key === 'Escape') {
      setOpen(false)
    }
  }

  return (
    <div className="relative ml-auto" ref={containerRef}>
      <button
        type="button"
        onClick={() => setOpen(!open)}
        aria-haspopup="listbox"
        aria-expanded={open}
        className={clsx(
          'flex max-w-[14rem] items-center gap-1.5 rounded-md border bg-white dark:bg-zinc-100 px-2.5 py-1.5 text-sm focus:border-zinc-400 focus:outline-none',
          value ? 'border-zinc-400 text-zinc-900' : 'border-zinc-200 text-zinc-700',
        )}
      >
        <FolderOpen className="h-3.5 w-3.5 flex-shrink-0 text-zinc-400" />
        <span className="truncate">{value ?? 'All projects'}</span>
        <ChevronDown className="h-3.5 w-3.5 flex-shrink-0 text-zinc-400" />
      </button>

      {open && (
        <div className="absolute right-0 z-50 mt-1 w-64 overflow-hidden rounded-xl border border-zinc-200 bg-zinc-100 py-1 shadow-xl shadow-black/10">
          <div className="px-2 py-1.5">
            <div className="relative">
              <Search className="absolute left-2 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-zinc-400" />
              <input
                ref={inputRef}
                type="text"
                role="combobox"
                aria-label="Search projects"
                aria-expanded
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                onKeyDown={handleKeyDown}
                placeholder="Search projects…"
                className="w-full rounded-lg border border-zinc-200 bg-white dark:bg-zinc-200 py-1 pl-7 pr-2 text-sm text-zinc-700 outline-none placeholder:text-zinc-400 focus:border-zinc-400"
              />
            </div>
          </div>
          <ul ref={listRef} role="listbox" className="max-h-72 overflow-y-auto">
            {options.map((name, i) => {
              const selected = name === value
              return (
                <li
                  key={name ?? '__all__'}
                  role="option"
                  aria-selected={selected}
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => pick(name)}
                  onMouseEnter={() => setHighlight(i)}
                  className={clsx(
                    'flex cursor-pointer items-center gap-2 px-3 py-1.5 text-sm',
                    i === highlight ? 'bg-white dark:bg-zinc-200' : '',
                    name === null ? 'text-zinc-500' : 'text-zinc-700',
                  )}
                >
                  <Check className={clsx('h-3.5 w-3.5 flex-shrink-0', selected ? 'text-zinc-900' : 'invisible')} />
                  <span className="truncate">{name ?? 'All projects'}</span>
                </li>
              )
            })}
            {options.length === 0 && <li className="px-3 py-1.5 text-xs text-zinc-400">No matching project</li>}
          </ul>
        </div>
      )}
    </div>
  )
}
