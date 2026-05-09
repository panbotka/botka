import { useEffect, useState } from 'react'

import { fetchTaskTags } from '../api/client'
import type { TaskTag } from '../types'
import { TaskTagChip } from './TaskTagChip'

interface TaskTagFilterBarProps {
  selected: number[]
  onChange: (ids: number[]) => void
}

export function TaskTagFilterBar({ selected, onChange }: TaskTagFilterBarProps) {
  const [tags, setTags] = useState<TaskTag[]>([])

  useEffect(() => {
    fetchTaskTags()
      .then(setTags)
      .catch(() => {
        // Silent — the filter is a non-critical surface; missing tags shouldn't
        // block the rest of the page.
      })
  }, [])

  if (tags.length === 0) return null

  const toggle = (id: number) => {
    const has = selected.includes(id)
    const next = has ? selected.filter((x) => x !== id) : [...selected, id]
    onChange(next)
  }

  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="mr-1 text-xs uppercase tracking-wide text-zinc-400">Tags:</span>
      {tags.map((tag) => (
        <TaskTagChip
          key={tag.id}
          tag={tag}
          onClick={() => toggle(tag.id)}
          selected={selected.includes(tag.id)}
          size="xs"
        />
      ))}
      {selected.length > 0 && (
        <button
          type="button"
          onClick={() => onChange([])}
          className="ml-1 text-xs text-zinc-400 hover:text-zinc-600"
        >
          clear
        </button>
      )}
    </div>
  )
}
