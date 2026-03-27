import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useTasks } from './useTasks'

vi.mock('../api/client', () => ({
  fetchTasks: vi.fn(),
}))

import { fetchTasks } from '../api/client'

const mockFetchTasks = vi.mocked(fetchTasks)

beforeEach(() => {
  vi.clearAllMocks()
})

describe('useTasks', () => {
  it('starts in loading state', () => {
    mockFetchTasks.mockReturnValue(new Promise(() => {})) // never resolves
    const { result } = renderHook(() => useTasks())
    expect(result.current.loading).toBe(true)
    expect(result.current.tasks).toEqual([])
    expect(result.current.error).toBeNull()
  })

  it('fetches tasks on mount and returns data', async () => {
    const tasks = [
      { id: '1', title: 'Task 1', status: 'done' },
      { id: '2', title: 'Task 2', status: 'queued' },
    ]
    mockFetchTasks.mockResolvedValue({ data: tasks, total: 2 } as never)

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.loading).toBe(false))

    expect(result.current.tasks).toEqual(tasks)
    expect(result.current.total).toBe(2)
    expect(result.current.error).toBeNull()
    expect(mockFetchTasks).toHaveBeenCalledTimes(1)
  })

  it('passes status filter to API', async () => {
    mockFetchTasks.mockResolvedValue({ data: [], total: 0 } as never)

    renderHook(() => useTasks({ status: 'running' }))

    await waitFor(() => expect(mockFetchTasks).toHaveBeenCalled())
    expect(mockFetchTasks).toHaveBeenCalledWith(
      expect.objectContaining({ status: 'running' }),
    )
  })

  it('passes project_id filter to API', async () => {
    mockFetchTasks.mockResolvedValue({ data: [], total: 0 } as never)

    renderHook(() => useTasks({ project_id: 'abc-123' }))

    await waitFor(() => expect(mockFetchTasks).toHaveBeenCalled())
    expect(mockFetchTasks).toHaveBeenCalledWith(
      expect.objectContaining({ project_id: 'abc-123' }),
    )
  })

  it('does not pass undefined filters', async () => {
    mockFetchTasks.mockResolvedValue({ data: [], total: 0 } as never)

    renderHook(() => useTasks())

    await waitFor(() => expect(mockFetchTasks).toHaveBeenCalled())
    const callArg = mockFetchTasks.mock.calls[0]![0] as Record<string, unknown>
    expect(callArg).not.toHaveProperty('status')
    expect(callArg).not.toHaveProperty('project_id')
  })

  it('sets error state on fetch failure', async () => {
    mockFetchTasks.mockRejectedValue(new Error('Network error'))

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.loading).toBe(false))

    expect(result.current.error).toBe('Network error')
    expect(result.current.tasks).toEqual([])
  })

  it('sets generic error for non-Error rejections', async () => {
    mockFetchTasks.mockRejectedValue('unknown')

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.error).toBe('Failed to fetch tasks')
  })

  it('refetch re-fetches tasks', async () => {
    mockFetchTasks
      .mockResolvedValueOnce({ data: [{ id: '1', title: 'Old' }], total: 1 } as never)
      .mockResolvedValueOnce({ data: [{ id: '1', title: 'New' }], total: 1 } as never)

    const { result } = renderHook(() => useTasks())

    await waitFor(() => expect(result.current.loading).toBe(false))
    expect(result.current.tasks[0]!.title).toBe('Old')

    await act(async () => {
      await result.current.refetch()
    })

    expect(result.current.tasks[0]!.title).toBe('New')
    expect(mockFetchTasks).toHaveBeenCalledTimes(2)
  })
})
