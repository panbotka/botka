import { describe, it, expect, vi, afterEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useElapsedTime } from './useElapsedTime'

afterEach(() => {
  vi.restoreAllMocks()
})

describe('useElapsedTime', () => {
  it('returns null when since is null', () => {
    const { result } = renderHook(() => useElapsedTime(null))
    expect(result.current).toBeNull()
  })

  it('formats seconds when under 60s', () => {
    vi.spyOn(Date, 'now').mockReturnValue(new Date('2024-01-01T00:00:30Z').getTime())
    const since = new Date('2024-01-01T00:00:00Z')
    const { result } = renderHook(() => useElapsedTime(since))
    expect(result.current).toBe('30s ago')
  })

  it('formats minutes and seconds when under 1 hour', () => {
    vi.spyOn(Date, 'now').mockReturnValue(new Date('2024-01-01T00:05:15Z').getTime())
    const since = new Date('2024-01-01T00:00:00Z')
    const { result } = renderHook(() => useElapsedTime(since))
    expect(result.current).toBe('5m 15s ago')
  })

  it('formats hours and minutes when 1+ hours', () => {
    vi.spyOn(Date, 'now').mockReturnValue(new Date('2024-01-01T02:30:00Z').getTime())
    const since = new Date('2024-01-01T00:00:00Z')
    const { result } = renderHook(() => useElapsedTime(since))
    expect(result.current).toBe('2h 30m ago')
  })

  it('returns 0s ago when since equals now', () => {
    const now = new Date('2024-01-01T00:00:00Z')
    vi.spyOn(Date, 'now').mockReturnValue(now.getTime())
    const { result } = renderHook(() => useElapsedTime(now))
    expect(result.current).toBe('0s ago')
  })

  it('clamps negative elapsed to 0', () => {
    // since is in the future
    vi.spyOn(Date, 'now').mockReturnValue(new Date('2024-01-01T00:00:00Z').getTime())
    const since = new Date('2024-01-01T00:01:00Z')
    const { result } = renderHook(() => useElapsedTime(since))
    expect(result.current).toBe('0s ago')
  })
})
