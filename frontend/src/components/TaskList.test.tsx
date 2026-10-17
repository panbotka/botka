import { useState } from 'react'
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'

import { TaskList } from './TaskList'
import { bulkUpdateTasks } from '../api/client'
import type { Task } from '../types'

let mobile = false
vi.mock('../hooks/useIsMobile', () => ({
  useIsMobile: () => mobile,
}))

vi.mock('../api/client', () => ({
  bulkUpdateTasks: vi.fn(),
  reorderTasks: vi.fn(),
  updateTask: vi.fn(),
}))

function task(over: Partial<Task>): Task {
  return {
    id: 't',
    title: 'Task',
    spec: '',
    status: 'pending',
    priority: 0,
    project_id: 'p1',
    project_name: 'alpha',
    retry_count: 0,
    started_at: null,
    completed_at: null,
    created_at: '2026-10-17T08:00:00Z',
    updated_at: '2026-10-17T08:00:00Z',
    ...over,
  } as unknown as Task
}

const tasks = [task({ id: 'a', title: 'Alpha' }), task({ id: 'b', title: 'Beta' })]

// Harness owns the selection state the way TasksPage does, so the tests see
// the real mode/selection round-trip rather than a single callback call.
function Harness({ initialMode = false }: { initialMode?: boolean }) {
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [mode, setMode] = useState(initialMode)
  return (
    <>
      <div data-testid="mode">{mode ? 'on' : 'off'}</div>
      <div data-testid="selected">{Array.from(selected).sort().join(',')}</div>
      <TaskList
        tasks={tasks}
        onReorder={async () => {}}
        selectedIds={selected}
        onSelectionChange={setSelected}
        selectionMode={mode}
        onSelectionModeChange={(on) => {
          setMode(on)
          if (!on) setSelected(new Set())
        }}
        onStatusChange={() => {}}
      />
    </>
  )
}

function renderList(initialMode = false) {
  render(
    <MemoryRouter initialEntries={['/tasks']}>
      <Routes>
        <Route path="/tasks" element={<Harness initialMode={initialMode} />} />
        <Route path="/tasks/:id" element={<div>DETAIL PAGE</div>} />
      </Routes>
    </MemoryRouter>,
  )
}

const row = (id: string) => screen.getByTestId(`task-row-${id}`)
const selected = () => screen.getByTestId('selected').textContent

afterEach(() => {
  vi.useRealTimers()
  mobile = false
})

describe('TaskList selection mode', () => {
  it('navigates to the detail on row click outside selection mode', () => {
    renderList(false)
    fireEvent.click(row('a'))
    expect(screen.getByText('DETAIL PAGE')).toBeInTheDocument()
  })

  it('toggles the row on click in selection mode and never navigates', () => {
    renderList(true)
    fireEvent.click(screen.getByText('Alpha'))
    expect(selected()).toBe('a')
    expect(screen.queryByText('DETAIL PAGE')).not.toBeInTheDocument()

    fireEvent.click(row('b'))
    expect(selected()).toBe('a,b')

    fireEvent.click(row('a'))
    expect(selected()).toBe('b')
    expect(screen.queryByText('DETAIL PAGE')).not.toBeInTheDocument()
  })

  it('enters selection mode when a checkbox is ticked outside it', () => {
    renderList(false)
    fireEvent.click(screen.getByLabelText('Vybrat Alpha'))
    expect(screen.getByTestId('mode').textContent).toBe('on')
    expect(selected()).toBe('a')
    expect(screen.queryByText('DETAIL PAGE')).not.toBeInTheDocument()

    // Row taps now toggle rather than navigate.
    fireEvent.click(row('b'))
    expect(selected()).toBe('a,b')
  })

  it('long-press enters selection mode, selects the row, and swallows the click', () => {
    vi.useFakeTimers()
    renderList(false)
    fireEvent.pointerDown(row('a'), { button: 0, clientX: 10, clientY: 10 })
    act(() => {
      vi.advanceTimersByTime(500)
    })
    expect(screen.getByTestId('mode').textContent).toBe('on')
    expect(selected()).toBe('a')

    fireEvent.pointerUp(row('a'))
    fireEvent.click(row('a'))
    expect(selected()).toBe('a')
    expect(screen.queryByText('DETAIL PAGE')).not.toBeInTheDocument()
  })

  it('a short press still navigates', () => {
    vi.useFakeTimers()
    renderList(false)
    fireEvent.pointerDown(row('a'), { button: 0, clientX: 10, clientY: 10 })
    act(() => {
      vi.advanceTimersByTime(200)
    })
    fireEvent.pointerUp(row('a'))
    fireEvent.click(row('a'))
    expect(screen.getByText('DETAIL PAGE')).toBeInTheDocument()
  })

  it('"Vybrat vše" selects every row and then deselects them', () => {
    renderList(true)
    fireEvent.click(screen.getByRole('button', { name: /Vybrat vše/ }))
    expect(selected()).toBe('a,b')
    fireEvent.click(screen.getByRole('button', { name: /Odznačit vše/ }))
    expect(selected()).toBe('')
  })

  it('hides the drag handle and the status dropdown in selection mode', () => {
    renderList(true)
    // Neither the grip button nor the status dropdown toggle is rendered.
    const rowButtons = row('a').querySelectorAll('button')
    expect(rowButtons).toHaveLength(0)
  })

  it('shows the bulk bar with the count and exits selection mode via its clear button', () => {
    renderList(true)
    fireEvent.click(row('a'))
    expect(screen.getByText('1 vybráno')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /Do fronty/ })).toBeInTheDocument()
    fireEvent.click(screen.getByTitle('Zrušit výběr'))
    expect(screen.getByTestId('mode').textContent).toBe('off')
    expect(selected()).toBe('')
  })

  it('exits selection mode after a fully successful bulk action', async () => {
    vi.mocked(bulkUpdateTasks).mockResolvedValue({ succeeded: ['a', 'b'], failed: [] } as never)
    renderList(true)
    fireEvent.click(row('a'))
    fireEvent.click(row('b'))
    await act(async () => {
      fireEvent.click(screen.getByRole('button', { name: /Do fronty/ }))
    })
    expect(bulkUpdateTasks).toHaveBeenCalledWith(['a', 'b'], 'requeue', undefined)
    expect(screen.getByTestId('mode').textContent).toBe('off')
    expect(selected()).toBe('')
  })

  it('on mobile puts the secondary actions behind the overflow menu', () => {
    mobile = true
    renderList(true)
    fireEvent.click(row('a'))
    expect(screen.getByRole('button', { name: /Do fronty/ })).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /Smazat/ })).not.toBeInTheDocument()
    fireEvent.click(screen.getByRole('button', { name: 'Další akce' }))
    expect(screen.getByRole('menuitem', { name: /Smazat/ })).toBeInTheDocument()
    expect(screen.getByRole('menuitem', { name: /Do Pending/ })).toBeInTheDocument()
  })
})
