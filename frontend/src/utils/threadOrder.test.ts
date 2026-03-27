import { describe, it, expect } from 'vitest'
import { getSidebarThreads } from './threadOrder'
import type { Thread } from '../types'

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: 1,
    title: 'Test Thread',
    model: 'sonnet',
    system_prompt: '',
    pinned: false,
    archived: false,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('getSidebarThreads', () => {
  it('returns empty array for no threads', () => {
    expect(getSidebarThreads([])).toEqual([])
  })

  it('excludes archived threads', () => {
    const threads = [
      makeThread({ id: 1, archived: false }),
      makeThread({ id: 2, archived: true }),
      makeThread({ id: 3, archived: false }),
    ]
    const result = getSidebarThreads(threads)
    expect(result).toHaveLength(2)
    expect(result.map((t) => t.id)).toEqual([1, 3])
  })

  it('places pinned threads before regular threads', () => {
    const threads = [
      makeThread({ id: 1, pinned: false }),
      makeThread({ id: 2, pinned: true }),
      makeThread({ id: 3, pinned: false }),
      makeThread({ id: 4, pinned: true }),
    ]
    const result = getSidebarThreads(threads)
    expect(result.map((t) => t.id)).toEqual([2, 4, 1, 3])
  })

  it('preserves relative order within pinned and regular groups', () => {
    const threads = [
      makeThread({ id: 10, pinned: false }),
      makeThread({ id: 20, pinned: true }),
      makeThread({ id: 30, pinned: false }),
      makeThread({ id: 40, pinned: true }),
    ]
    const result = getSidebarThreads(threads)
    // Pinned: 20, 40 (order preserved), Regular: 10, 30 (order preserved)
    expect(result.map((t) => t.id)).toEqual([20, 40, 10, 30])
  })

  it('excludes archived pinned threads', () => {
    const threads = [
      makeThread({ id: 1, pinned: true, archived: true }),
      makeThread({ id: 2, pinned: true, archived: false }),
      makeThread({ id: 3, pinned: false, archived: false }),
    ]
    const result = getSidebarThreads(threads)
    expect(result.map((t) => t.id)).toEqual([2, 3])
  })

  it('returns only pinned threads when all regular are archived', () => {
    const threads = [
      makeThread({ id: 1, pinned: true }),
      makeThread({ id: 2, pinned: false, archived: true }),
    ]
    const result = getSidebarThreads(threads)
    expect(result).toHaveLength(1)
    expect(result[0]!.id).toBe(1)
  })
})
