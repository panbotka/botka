import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useBoxAutoOff } from './useBoxAutoOff'

vi.mock('../api/client', () => ({
  fetchBoxAutoOff: vi.fn(),
  updateServerSettings: vi.fn(),
}))

import { fetchBoxAutoOff, updateServerSettings } from '../api/client'
import type { BoxAutoOffStatus } from '../types'

const mockFetch = vi.mocked(fetchBoxAutoOff)
const mockUpdate = vi.mocked(updateServerSettings)

const baseStatus: BoxAutoOffStatus = {
  enabled: true,
  streak: 1,
  required_checks: 3,
  interval_seconds: 600,
  next_check_at: null,
  earliest_shutdown_at: null,
  last_evaluation: null,
  recent: null,
  events: [],
}

beforeEach(() => {
  vi.clearAllMocks()
  vi.useFakeTimers({ shouldAdvanceTime: true })
})

afterEach(() => {
  vi.useRealTimers()
})

describe('useBoxAutoOff', () => {
  it('loads the status', async () => {
    mockFetch.mockResolvedValue({ ...baseStatus })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.status?.streak).toBe(1))
    expect(result.current.error).toBeNull()
    expect(result.current.loading).toBe(false)
  })

  it('counts down to earliest_shutdown_at and ticks', async () => {
    mockFetch.mockResolvedValue({
      ...baseStatus,
      earliest_shutdown_at: new Date(Date.now() + 90_000).toISOString(),
    })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.countdown).not.toBeNull())
    const first = result.current.countdown as number

    await act(async () => {
      vi.advanceTimersByTime(5_000)
    })

    expect(result.current.countdown as number).toBeLessThan(first)
  })

  it('has no countdown when the server gives none', async () => {
    mockFetch.mockResolvedValue({ ...baseStatus })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.status).not.toBeNull())
    expect(result.current.countdown).toBeNull()
  })

  it('clamps a countdown that has already elapsed to zero', async () => {
    mockFetch.mockResolvedValue({
      ...baseStatus,
      earliest_shutdown_at: new Date(Date.now() - 30_000).toISOString(),
    })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.countdown).toBe(0))
  })

  it('surfaces a fetch failure', async () => {
    mockFetch.mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.error).toBe('boom'))
  })

  it('writes the switch through the settings endpoint and refreshes', async () => {
    mockFetch.mockResolvedValue({ ...baseStatus, enabled: false })
    mockUpdate.mockResolvedValue({ max_workers: 2, box_auto_off: true })

    const { result } = renderHook(() => useBoxAutoOff())
    await waitFor(() => expect(result.current.status).not.toBeNull())

    mockFetch.mockResolvedValue({ ...baseStatus, enabled: true })
    await act(async () => {
      await result.current.setEnabled(true)
    })

    expect(mockUpdate).toHaveBeenCalledWith({ box_auto_off: true })
    await waitFor(() => expect(result.current.status?.enabled).toBe(true))
  })
})
