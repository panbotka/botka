import { useState, useCallback, useEffect, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { formatDate as formatDateOnly } from '../utils/dateFormat'
import {
  DndContext,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  verticalListSortingStrategy,
  arrayMove,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { clsx } from 'clsx'
import {
  GripVertical,
  Clock,
  CheckCircle2,
  XCircle,
  AlertTriangle,
  Loader2,
  Ban,
  ChevronDown,
  Trash2,
} from 'lucide-react'

import { reorderTasks, updateTask, batchUpdateTaskStatus } from '../api/client'
import type { Task, TaskStatus } from '../types'

const statusBadge: Record<TaskStatus, { icon: typeof CheckCircle2; bg: string; text: string; label: string; pulse?: boolean; strike?: boolean }> = {
  pending:      { icon: Clock,          bg: 'bg-zinc-100',   text: 'text-zinc-600',   label: 'Pending' },
  queued:       { icon: Clock,          bg: 'bg-blue-50',    text: 'text-blue-700',   label: 'Queued' },
  running:      { icon: Loader2,        bg: 'bg-amber-50',   text: 'text-amber-700',  label: 'Running', pulse: true },
  done:         { icon: CheckCircle2,   bg: 'bg-emerald-50', text: 'text-emerald-700', label: 'Done' },
  failed:       { icon: XCircle,        bg: 'bg-red-50',     text: 'text-red-700',    label: 'Failed' },
  needs_review: { icon: AlertTriangle,  bg: 'bg-orange-50',  text: 'text-orange-700', label: 'Review' },
  cancelled:    { icon: Ban,            bg: 'bg-zinc-50',    text: 'text-zinc-400',   label: 'Cancelled', strike: true },
  deleted:      { icon: Trash2,        bg: 'bg-zinc-50',    text: 'text-zinc-400',   label: 'Deleted', strike: true },
}

const statusTransitions: Partial<Record<TaskStatus, { label: string; target: TaskStatus }[]>> = {
  pending:      [{ label: 'Queue', target: 'queued' }, { label: 'Cancel', target: 'cancelled' }],
  queued:       [{ label: 'Unqueue', target: 'pending' }, { label: 'Cancel', target: 'cancelled' }],
  failed:       [{ label: 'Requeue', target: 'queued' }, { label: 'Cancel', target: 'cancelled' }],
  needs_review: [{ label: 'Requeue', target: 'queued' }, { label: 'Mark Done', target: 'done' }],
  deleted:      [{ label: 'Restore', target: 'pending' }],
}

function StatusBadge({ status }: { status: TaskStatus }) {
  const cfg = statusBadge[status]
  const Icon = cfg.icon
  return (
    <span
      className={clsx(
        'inline-flex items-center gap-1 rounded-full px-2 py-0.5 text-xs font-medium',
        cfg.bg,
        cfg.text,
        cfg.pulse && 'animate-pulse',
        cfg.strike && 'line-through',
      )}
    >
      <Icon className={clsx('h-3 w-3', status === 'running' && 'animate-spin')} />
      {cfg.label}
    </span>
  )
}

function StatusBadgeDropdown({ taskId, status, onStatusChange }: { taskId: string; status: TaskStatus; onStatusChange: () => void }) {
  const [open, setOpen] = useState(false)
  const [openUp, setOpenUp] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const transitions = statusTransitions[status]

  useEffect(() => {
    if (!open) return
    const handler = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) {
        setOpen(false)
      }
    }
    document.addEventListener('click', handler, true)
    return () => document.removeEventListener('click', handler, true)
  }, [open])

  const handleToggle = () => {
    if (!open && ref.current) {
      const rect = ref.current.getBoundingClientRect()
      const spaceBelow = window.innerHeight - rect.bottom
      setOpenUp(spaceBelow < 120)
    }
    setOpen(!open)
  }

  const handleTransition = async (target: TaskStatus) => {
    setOpen(false)
    await updateTask(taskId, { status: target })
    onStatusChange()
  }

  if (!transitions || transitions.length === 0) {
    return <StatusBadge status={status} />
  }

  return (
    <div ref={ref} className="relative inline-block" onClick={(e) => e.stopPropagation()}>
      <button
        className="inline-flex items-center gap-0.5 cursor-pointer"
        onClick={handleToggle}
      >
        <StatusBadge status={status} />
        <ChevronDown className={clsx('h-3 w-3 text-zinc-400 transition-transform', open && openUp && 'rotate-180')} />
      </button>
      {open && (
        <div className={clsx(
          'absolute left-0 z-10 min-w-[120px] rounded-md border border-zinc-200 bg-zinc-100 shadow-lg',
          openUp ? 'bottom-full mb-1' : 'top-full mt-1',
        )}>
          {transitions.map((t) => (
            <button
              key={t.target}
              className="block w-full px-3 py-1.5 text-left text-xs font-medium text-zinc-700 hover:bg-zinc-50 first:rounded-t-md last:rounded-b-md"
              onClick={() => handleTransition(t.target)}
            >
              {t.label}
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function formatDate(iso: string): string {
  const d = new Date(iso)
  const now = new Date()
  const diff = now.getTime() - d.getTime()
  const mins = Math.floor(diff / 60_000)
  if (mins < 1) return 'just now'
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 7) return `${days}d ago`
  return formatDateOnly(d)
}

function formatDuration(startedAt: string | null, completedAt: string | null): string | null {
  if (!startedAt || !completedAt) return null
  const ms = new Date(completedAt).getTime() - new Date(startedAt).getTime()
  if (ms < 0) return null
  const totalMins = Math.floor(ms / 60_000)
  const days = Math.floor(totalMins / 1440)
  const hours = Math.floor((totalMins % 1440) / 60)
  const mins = totalMins % 60
  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0) parts.push(`${hours}h`)
  if (mins > 0 || parts.length === 0) parts.push(`${mins}m`)
  return parts.join(' ')
}

const isDraggable = (status: TaskStatus) => status === 'pending' || status === 'queued'

interface SortableRowProps {
  task: Task
  onClick: (id: string) => void
  selected: boolean
  onSelect: (id: string, checked: boolean) => void
  onStatusChange: () => void
}

function SortableRow({ task, onClick, selected, onSelect, onStatusChange }: SortableRowProps) {
  const draggable = isDraggable(task.status)
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id: task.id,
    disabled: !draggable,
  })

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  }

  return (
    <tr
      ref={setNodeRef}
      style={style}
      className={clsx(
        'group cursor-pointer border-b border-zinc-100 hover:bg-zinc-50',
        isDragging && 'z-10 bg-zinc-50 shadow-sm opacity-80',
        selected && 'bg-blue-50/50',
      )}
      onClick={() => onClick(task.id)}
    >
      <td className="w-8 py-2.5 pl-2 text-center">
        <input
          type="checkbox"
          checked={selected}
          onChange={(e) => onSelect(task.id, e.target.checked)}
          onClick={(e) => e.stopPropagation()}
          className="h-3.5 w-3.5 rounded border-zinc-300 text-blue-600 focus:ring-blue-500"
        />
      </td>
      <td className="w-8 py-2.5 pl-2">
        {draggable ? (
          <button
            className="cursor-grab touch-none rounded p-0.5 text-zinc-300 hover:text-zinc-500 active:cursor-grabbing"
            {...attributes}
            {...listeners}
            onClick={(e) => e.stopPropagation()}
          >
            <GripVertical className="h-4 w-4" />
          </button>
        ) : (
          <span className="inline-block w-5" />
        )}
      </td>
      <td className="w-16 py-2.5 text-center text-xs tabular-nums text-zinc-400">
        {task.priority}
      </td>
      <td className="py-2.5">
        <StatusBadgeDropdown taskId={task.id} status={task.status} onStatusChange={onStatusChange} />
      </td>
      <td className="py-2.5 pl-2 text-sm font-medium text-zinc-900 group-hover:text-blue-600">
        {task.title}
      </td>
      <td className="whitespace-nowrap py-2.5 pl-2 text-xs text-zinc-500">
        {task.project_name || task.project?.name || '\u2014'}
      </td>
      {(() => {
        const duration = formatDuration(task.started_at, task.completed_at)
        return duration ? (
          <td className="whitespace-nowrap py-2.5 pl-2 pr-3 text-right text-xs tabular-nums text-zinc-400" title={`Started: ${new Date(task.started_at!).toLocaleString()}`}>
            {duration}
          </td>
        ) : (
          <td className="py-2.5 pl-2 pr-3 text-right text-xs text-zinc-400">
            {formatDate(task.created_at)}
          </td>
        )
      })()}
    </tr>
  )
}

function BatchToolbar({
  selectedIds,
  tasks,
  onDeselect,
  onStatusChange,
}: {
  selectedIds: Set<string>
  tasks: Task[]
  onDeselect: () => void
  onStatusChange: () => void
}) {
  const selectedTasks = tasks.filter((t) => selectedIds.has(t.id))

  const commonTransitions = selectedTasks.reduce<{ label: string; target: TaskStatus }[] | null>(
    (acc, task) => {
      const transitions = statusTransitions[task.status]
      if (!transitions) return []
      if (acc === null) return [...transitions]
      return acc.filter((t) => transitions.some((tr) => tr.target === t.target && tr.label === t.label))
    },
    null,
  ) ?? []

  const handleBatch = async (target: TaskStatus) => {
    const ids = Array.from(selectedIds)
    await batchUpdateTaskStatus(ids, target)
    onDeselect()
    onStatusChange()
  }

  return (
    <div className="flex items-center gap-3 rounded-lg border border-blue-200 bg-blue-50 px-4 py-2 mb-3">
      <span className="text-sm font-medium text-blue-800">{selectedIds.size} selected</span>
      {commonTransitions.map((t) => (
        <button
          key={t.target}
          onClick={() => handleBatch(t.target)}
          className="rounded-full bg-zinc-50 px-3 py-1 text-xs font-medium text-zinc-700 border border-zinc-200 hover:bg-zinc-100"
        >
          {t.label}
        </button>
      ))}
      <button
        onClick={onDeselect}
        className="ml-auto rounded-full bg-zinc-50 px-3 py-1 text-xs font-medium text-zinc-500 border border-zinc-200 hover:bg-zinc-100"
      >
        Deselect all
      </button>
    </div>
  )
}

interface TaskListProps {
  tasks: Task[]
  onReorder: () => Promise<void>
  selectedIds: Set<string>
  onSelectionChange: (ids: Set<string>) => void
  onStatusChange: () => void
}

export function TaskList({ tasks, onReorder, selectedIds, onSelectionChange, onStatusChange }: TaskListProps) {
  const navigate = useNavigate()
  const [items, setItems] = useState(tasks)
  const [reordering, setReordering] = useState(false)

  // Sync when parent tasks change (new fetch)
  if (tasks !== items && !reordering) {
    setItems(tasks)
  }

  const sensors = useSensors(
    useSensor(PointerSensor, { activationConstraint: { distance: 5 } }),
    useSensor(KeyboardSensor, { coordinateGetter: sortableKeyboardCoordinates }),
  )

  const handleDragEnd = useCallback(
    async (event: DragEndEvent) => {
      const { active, over } = event
      if (!over || active.id === over.id) return

      const oldIndex = items.findIndex((t) => t.id === active.id)
      const newIndex = items.findIndex((t) => t.id === over.id)
      if (oldIndex === -1 || newIndex === -1) return

      const reordered = arrayMove(items, oldIndex, newIndex)

      const updates: { id: string; priority: number }[] = []
      const step = Math.max(1, Math.floor(1000 / reordered.length))
      for (let i = 0; i < reordered.length; i++) {
        const item = reordered[i]!
        const newPriority = 1000 - i * step
        if (item.priority !== newPriority) {
          reordered[i] = { ...item, priority: newPriority }
          updates.push({ id: item.id, priority: newPriority })
        }
      }

      // Optimistic update
      setItems(reordered)
      setReordering(true)

      try {
        if (updates.length > 0) {
          await reorderTasks(updates)
        }
        await onReorder()
      } catch {
        // Revert on error
        setItems(tasks)
      } finally {
        setReordering(false)
      }
    },
    [items, tasks, onReorder],
  )

  const handleRowClick = useCallback(
    (id: string) => navigate(`/tasks/${id}`),
    [navigate],
  )

  const handleSelect = useCallback(
    (id: string, checked: boolean) => {
      const next = new Set(selectedIds)
      if (checked) {
        next.add(id)
      } else {
        next.delete(id)
      }
      onSelectionChange(next)
    },
    [selectedIds, onSelectionChange],
  )

  const handleSelectAll = useCallback(
    (checked: boolean) => {
      if (checked) {
        onSelectionChange(new Set(items.map((t) => t.id)))
      } else {
        onSelectionChange(new Set())
      }
    },
    [items, onSelectionChange],
  )

  const allSelected = items.length > 0 && items.every((t) => selectedIds.has(t.id))

  if (items.length === 0) {
    return (
      <div className="flex h-48 items-center justify-center rounded-lg border border-dashed border-zinc-200">
        <p className="text-sm text-zinc-400">No tasks found</p>
      </div>
    )
  }

  return (
    <div>
      {selectedIds.size > 0 && (
        <BatchToolbar
          selectedIds={selectedIds}
          tasks={items}
          onDeselect={() => onSelectionChange(new Set())}
          onStatusChange={onStatusChange}
        />
      )}
      <div className="overflow-x-clip overflow-y-visible rounded-lg border border-zinc-200">
        <table className="w-full text-left">
          <thead>
            <tr className="border-b border-zinc-200 bg-zinc-50 text-xs font-medium uppercase tracking-wide text-zinc-500">
              <th className="w-8 py-2 pl-2 text-center">
                <input
                  type="checkbox"
                  checked={allSelected}
                  onChange={(e) => handleSelectAll(e.target.checked)}
                  className="h-3.5 w-3.5 rounded border-zinc-300 text-blue-600 focus:ring-blue-500"
                />
              </th>
              <th className="w-8 py-2 pl-2" />
              <th className="w-16 py-2 text-center">#</th>
              <th className="py-2">Status</th>
              <th className="py-2 pl-2">Title</th>
              <th className="py-2 pl-2">Project</th>
              <th className="py-2 pl-2 pr-3 text-right">Created</th>
            </tr>
          </thead>
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleDragEnd}
          >
            <SortableContext items={items.map((t) => t.id)} strategy={verticalListSortingStrategy}>
              <tbody>
                {items.map((task) => (
                  <SortableRow
                    key={task.id}
                    task={task}
                    onClick={handleRowClick}
                    selected={selectedIds.has(task.id)}
                    onSelect={handleSelect}
                    onStatusChange={onStatusChange}
                  />
                ))}
              </tbody>
            </SortableContext>
          </DndContext>
        </table>
      </div>
    </div>
  )
}
