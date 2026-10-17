import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, within, act, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import RunQueuePanel, { orderQueue } from './RunQueuePanel'
import type { RunnerStatus, Task } from '../types'

function task(over: Partial<Task>): Task {
  return {
    id: 't',
    title: 'Task',
    spec: '',
    status: 'queued',
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

const runner: RunnerStatus = {
  state: 'paused',
  active_tasks: [{ task_id: 'r1', task_title: 'R', project_name: 'alpha', started_at: '2026-10-17T10:00:00Z' }],
  max_workers: 2,
  draining: false,
  usage: null,
  task_limit: 0,
  completed_count: 0,
  paused_until: null,
  pause_reason: null,
  pause_source: null,
}

function renderPanel(props: Partial<Parameters<typeof RunQueuePanel>[0]> = {}) {
  const onShowQueued = vi.fn()
  render(
    <MemoryRouter>
      <RunQueuePanel
        running={[]}
        queued={[]}
        queuedTotal={0}
        runner={runner}
        onShowQueued={onShowQueued}
        {...props}
      />
    </MemoryRouter>,
  )
  return { onShowQueued }
}

function upNextTitles(): string[] {
  const list = screen.getByRole('list', { name: 'Následuje' })
  return within(list)
    .getAllByRole('listitem')
    .map((li) => li.querySelector('.flex-1')?.textContent ?? '')
}

describe('orderQueue', () => {
  it('sorts by priority DESC, then created_at ASC', () => {
    const ordered = orderQueue([
      task({ id: 'a', priority: 0, created_at: '2026-10-17T07:00:00Z' }),
      task({ id: 'b', priority: 5, created_at: '2026-10-17T09:00:00Z' }),
      task({ id: 'c', priority: 5, created_at: '2026-10-17T08:00:00Z' }),
    ])
    expect(ordered.map((t) => t.id)).toEqual(['c', 'b', 'a'])
  })
})

describe('RunQueuePanel', () => {
  afterEach(() => vi.useRealTimers())

  it('collapses into one muted line when nothing runs and the queue is empty', () => {
    renderPanel()
    expect(screen.getByText('Nic neběží, fronta je prázdná')).toBeTruthy()
    expect(screen.queryByText('Následuje')).toBeNull()
    expect(screen.getByTestId('runner-state').textContent).toContain('1/2 workerů')
    expect(screen.getByTestId('runner-state').textContent).toContain('pozastaven')
  })

  it('lists queued tasks in pick order with positions, capped at five', () => {
    const queued = [
      task({ id: 'q1', title: 'low old', priority: 0, created_at: '2026-10-17T01:00:00Z' }),
      task({ id: 'q2', title: 'high new', priority: 3, created_at: '2026-10-17T05:00:00Z' }),
      task({ id: 'q3', title: 'high old', priority: 3, created_at: '2026-10-17T02:00:00Z' }),
      task({ id: 'q4', title: 'mid', priority: 1, created_at: '2026-10-17T03:00:00Z' }),
      task({ id: 'q5', title: 'low new', priority: 0, created_at: '2026-10-17T06:00:00Z' }),
      task({ id: 'q6', title: 'lowest', priority: -1, created_at: '2026-10-17T00:00:00Z' }),
    ]
    const { onShowQueued } = renderPanel({ queued, queuedTotal: 8 })
    expect(screen.getByText('Nic neběží')).toBeTruthy()
    expect(upNextTitles()).toEqual(['high old', 'high new', 'mid', 'low old', 'low new'])
    const rows = screen.getAllByTestId('up-next-row')
    expect(rows[0]!.textContent).toContain('1.')
    expect(rows[0]!.textContent).toContain('P3')
    expect(rows[0]!.textContent).toContain('alpha')
    fireEvent.click(screen.getByRole('button', { name: '+3 dalších' }))
    expect(onShowQueued).toHaveBeenCalledOnce()
  })

  it('badges queued tasks whose project already has a running task', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-17T10:01:05Z'))
    const running = [
      task({ id: 'r1', title: 'running one', status: 'running', run_phase: 'verifying', started_at: '2026-10-17T10:00:00Z' }),
    ]
    const queued = [
      task({ id: 'q1', title: 'same project', project_id: 'p1' }),
      task({ id: 'q2', title: 'other project', project_id: 'p2', project_name: 'beta' }),
    ]
    renderPanel({ running, queued, queuedTotal: 2 })

    const runningList = screen.getByRole('list', { name: 'Běží teď' })
    expect(runningList.textContent).toContain('running one')
    expect(runningList.textContent).toContain('ověřování')
    expect(runningList.textContent).toContain('1:05')
    act(() => vi.advanceTimersByTime(2000))
    expect(runningList.textContent).toContain('1:07')

    const [same, other] = screen.getAllByTestId('up-next-row')
    expect(same!.textContent).toContain('čeká na projekt')
    expect(other!.textContent).not.toContain('čeká na projekt')
    expect(screen.queryByRole('button', { name: /dalších/ })).toBeNull()
  })
})
