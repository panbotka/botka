import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import BoxAutoOffCard from './BoxAutoOffCard'

vi.mock('../hooks/useBoxAutoOff', () => ({
  useBoxAutoOff: vi.fn(),
}))

import { useBoxAutoOff } from '../hooks/useBoxAutoOff'
import type { UseBoxAutoOffResult } from '../hooks/useBoxAutoOff'
import type { BoxAutoOffStatus } from '../types'

const mockHook = vi.mocked(useBoxAutoOff)

const baseStatus: BoxAutoOffStatus = {
  enabled: true,
  streak: 1,
  required_checks: 3,
  interval_seconds: 600,
  next_check_at: new Date(Date.now() + 300_000).toISOString(),
  earliest_shutdown_at: null,
  last_evaluation: null,
  recent: null,
  events: [],
}

function mockResult(overrides: Partial<UseBoxAutoOffResult> = {}): UseBoxAutoOffResult {
  return {
    status: baseStatus,
    loading: false,
    error: null,
    countdown: null,
    refresh: vi.fn(),
    setEnabled: vi.fn(),
    ...overrides,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('BoxAutoOffCard', () => {
  it('shows the countdown when there is one', () => {
    mockHook.mockReturnValue(
      mockResult({
        countdown: 1634,
        status: { ...baseStatus, earliest_shutdown_at: new Date().toISOString() },
      }),
    )

    render(<BoxAutoOffCard />)

    expect(screen.getByText('27:14')).toBeInTheDocument()
  })

  it('formats a countdown over an hour with hours', () => {
    mockHook.mockReturnValue(
      mockResult({
        countdown: 8225,
        status: { ...baseStatus, earliest_shutdown_at: new Date().toISOString() },
      }),
    )

    render(<BoxAutoOffCard />)

    expect(screen.getByText('2:17:05')).toBeInTheDocument()
  })

  it('names the blocker instead of a countdown when blocked', () => {
    mockHook.mockReturnValue(
      mockResult({
        status: {
          ...baseStatus,
          last_evaluation: {
            checked_at: new Date().toISOString(),
            idle: false,
            blockers: [{ name: 'kukatko_queue', detail: '12 queued, 1 running' }],
            box: null,
            kukatko: { status_code: 200, queued: 12, running: 1 },
          },
        },
      }),
    )

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/Kukátko has queued jobs/)).toBeInTheDocument()
    expect(screen.getAllByText(/12 queued, 1 running/).length).toBeGreaterThan(0)
  })

  it('shows the Kukátko failure detail', () => {
    mockHook.mockReturnValue(
      mockResult({
        status: {
          ...baseStatus,
          last_evaluation: {
            checked_at: new Date().toISOString(),
            idle: false,
            blockers: [
              { name: 'kukatko_queue', detail: 'queue could not be read: HTTP 502: bad gateway' },
            ],
            box: null,
            kukatko: {
              status_code: 502,
              queued: 0,
              running: 0,
              error: 'bad gateway',
              body_excerpt: 'upstream is down',
            },
          },
        },
      }),
    )

    render(<BoxAutoOffCard />)

    // "HTTP 502" appears twice on purpose: once as the blocker's reason and
    // once in the readings line beneath it.
    expect(screen.getAllByText(/HTTP 502/).length).toBeGreaterThan(0)
    expect(screen.getByText(/upstream is down/)).toBeInTheDocument()
  })

  it('shows the streak', () => {
    mockHook.mockReturnValue(mockResult({ status: { ...baseStatus, streak: 2 } }))

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/2 \/ 3 quiet checks/)).toBeInTheDocument()
  })

  it('shows the Box readings when present', () => {
    mockHook.mockReturnValue(
      mockResult({
        status: {
          ...baseStatus,
          last_evaluation: {
            checked_at: new Date().toISOString(),
            idle: true,
            blockers: null,
            box: { uptime_seconds: 33390, load1: 0.08, threads: 24, gpu_max_percent: 0 },
            kukatko: { status_code: 200, queued: 0, running: 0 },
          },
        },
      }),
    )

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/load 0\.08/)).toBeInTheDocument()
    expect(screen.getByText(/GPU 0%/)).toBeInTheDocument()
  })

  it('lists past shutdown attempts', () => {
    mockHook.mockReturnValue(
      mockResult({
        status: {
          ...baseStatus,
          events: [
            { id: 2, occurred_at: new Date().toISOString(), outcome: 'aborted' },
            { id: 1, occurred_at: new Date().toISOString(), outcome: 'shutdown' },
          ],
        },
      }),
    )

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/aborted/)).toBeInTheDocument()
    expect(screen.getByText(/shut down/)).toBeInTheDocument()
  })

  it('renders a load error', () => {
    mockHook.mockReturnValue(mockResult({ error: 'network down', status: null }))

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/network down/)).toBeInTheDocument()
  })
})
