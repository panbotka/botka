import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import TaskActionBar, { taskActions, type TaskActionHandlers } from './TaskActionBar'
import type { TaskStatus } from '../types'

function handlers(): TaskActionHandlers {
  return {
    onForceRun: vi.fn(),
    onQueue: vi.fn(),
    onCancel: vi.fn(),
    onRetry: vi.fn(),
    onMarkDone: vi.fn(),
    onRestore: vi.fn(),
    onDelete: vi.fn(),
    onEdit: vi.fn(),
    onKill: vi.fn(),
  }
}

const keys = (status: TaskStatus) => taskActions(status, handlers()).map((a) => a.key)

describe('taskActions', () => {
  it('puts the primary action of each status first and Delete last', () => {
    expect(keys('pending')).toEqual(['queue', 'edit', 'cancel', 'delete'])
    expect(keys('queued')).toEqual(['force-run', 'edit', 'delete'])
    expect(keys('running')).toEqual(['kill', 'edit'])
    expect(keys('failed')).toEqual(['retry', 'edit', 'delete'])
    expect(keys('needs_review')).toEqual(['retry', 'mark-done', 'edit', 'delete'])
    expect(keys('done')).toEqual(['edit', 'delete'])
    expect(keys('cancelled')).toEqual(['edit', 'delete'])
    expect(keys('deleted')).toEqual(['restore', 'edit'])
  })
})

describe('TaskActionBar', () => {
  it('hides actions beyond the first two behind the overflow menu on mobile', () => {
    const h = handlers()
    render(<TaskActionBar status="needs_review" disabled={false} {...h} />)
    // Third+ buttons are CSS-hidden below md, visible inline on desktop.
    expect(screen.getByRole('button', { name: 'Retry' }).className).not.toContain('hidden')
    expect(screen.getByRole('button', { name: 'Mark Done' }).className).not.toContain('hidden')
    expect(screen.getByRole('button', { name: 'Edit' }).className).toContain('hidden md:inline-flex')
    expect(screen.getByRole('button', { name: 'Delete' }).className).toContain('hidden md:inline-flex')

    fireEvent.click(screen.getByRole('button', { name: 'Další akce' }))
    const items = screen.getAllByRole('menuitem').map((el) => el.textContent)
    expect(items).toEqual(['Edit', 'Delete'])

    fireEvent.click(screen.getByRole('menuitem', { name: 'Delete' }))
    expect(h.onDelete).toHaveBeenCalledTimes(1)
    expect(screen.queryByRole('menu')).toBeNull()
  })

  it('omits the overflow menu when everything fits', () => {
    render(<TaskActionBar status="running" disabled={false} {...handlers()} />)
    expect(screen.getByRole('button', { name: 'Kill Task' })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: 'Další akce' })).toBeNull()
  })

  it('disables every action while one is in flight', () => {
    render(<TaskActionBar status="pending" disabled {...handlers()} />)
    expect(screen.getByRole('button', { name: 'Queue' })).toBeDisabled()
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })
})
