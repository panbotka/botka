import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, screen, act } from '@testing-library/react'
import TaskStatsCards, { formatStopwatch, taskTimes } from './TaskStatsCards'
import type { Task } from '../types'

const base = {
  id: 't1',
  title: 'x',
  spec: '',
  status: 'done',
  priority: 1,
  project_id: 'p1',
  retry_count: 0,
  started_at: '2026-10-14T10:00:00Z',
  completed_at: '2026-10-14T11:02:05Z',
  cost_usd: 1.234,
  created_at: '2026-10-14T09:00:00Z',
  updated_at: '2026-10-14T11:02:05Z',
} as unknown as Task

describe('formatStopwatch', () => {
  it('formats minutes and hours', () => {
    expect(formatStopwatch(0)).toBe('0:00')
    expect(formatStopwatch(65_000)).toBe('1:05')
    expect(formatStopwatch(3_725_000)).toBe('1:02:05')
    expect(formatStopwatch(-5)).toBe('0:00')
  })
})

describe('taskTimes', () => {
  it('drops a stale completed_at while running', () => {
    const t = taskTimes({ ...base, status: 'running' })
    expect(t.started).not.toBeNull()
    expect(t.finished).toBeNull()
  })
  it('drops completed_at older than started_at', () => {
    expect(taskTimes({ ...base, completed_at: '2026-10-14T09:00:00Z' }).finished).toBeNull()
  })
  it('hides times of a queued task', () => {
    expect(taskTimes({ ...base, status: 'queued' })).toEqual({ started: null, finished: null })
  })
})

describe('TaskStatsCards', () => {
  afterEach(() => vi.useRealTimers())

  it('shows the final duration and cost of a finished task', () => {
    render(<TaskStatsCards task={base} />)
    expect(screen.getByLabelText('Duration').textContent).toBe('1:02:05')
    expect(screen.getByLabelText('Cost').textContent).toBe('$1.23')
    expect(screen.getByLabelText('Status').textContent).toBe('Done')
  })

  it('ticks the stopwatch while running', () => {
    vi.useFakeTimers()
    vi.setSystemTime(new Date('2026-10-14T10:00:30Z'))
    render(<TaskStatsCards task={{ ...base, status: 'running', run_phase: 'agent' }} />)
    expect(screen.getByLabelText('Duration').textContent).toBe('0:30')
    expect(screen.getByLabelText('Finished').textContent).toBe('—')
    act(() => vi.advanceTimersByTime(2000))
    expect(screen.getByLabelText('Duration').textContent).toBe('0:32')
  })
})
