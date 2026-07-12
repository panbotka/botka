import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { Task } from '../types'

const { fetchTask, fetchTaskNotes, fetchTaskTags, forceRunTask } = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskNotes: vi.fn(),
  fetchTaskTags: vi.fn(),
  forceRunTask: vi.fn(),
}))

vi.mock('../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/client')>()),
  fetchTask,
  fetchTaskNotes,
  fetchTaskTags,
  forceRunTask,
}))

// The SSE hook needs an EventSource, which jsdom does not provide.
vi.mock('../hooks/useTaskEvents', () => ({ useTaskEvents: () => {} }))

import TaskDetailPage from './TaskDetailPage'

const baseTask = {
  id: 'ef0f448c-6426-4d34-abb2-93464a66ce81',
  title: 'Stuck task',
  spec: 'do the thing',
  status: 'queued',
  priority: 1,
  project_id: 'p1',
  created_at: '2026-07-09T10:00:00Z',
  updated_at: '2026-07-09T10:00:00Z',
} as unknown as Task

function renderPage() {
  return render(
    <MemoryRouter initialEntries={[`/tasks/${baseTask.id}`]}>
      <Routes>
        <Route path="/tasks/:id" element={<TaskDetailPage />} />
      </Routes>
    </MemoryRouter>,
  )
}

describe('TaskDetailPage force run', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchTaskNotes.mockResolvedValue([])
    fetchTaskTags.mockResolvedValue([])
    forceRunTask.mockResolvedValue({ ...baseTask, status: 'running' } as Task)
  })
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('shows "Spustit teď" for a queued task and calls forceRunTask on click', async () => {
    fetchTask.mockResolvedValue(baseTask)
    renderPage()
    const btn = await screen.findByText('Spustit teď')
    fireEvent.click(btn)
    await waitFor(() => expect(forceRunTask).toHaveBeenCalledWith(baseTask.id))
  })

  it('does not show "Spustit teď" for a non-queued task', async () => {
    fetchTask.mockResolvedValue({ ...baseTask, status: 'done' } as Task)
    renderPage()
    await screen.findByText('Stuck task')
    expect(screen.queryByText('Spustit teď')).toBeNull()
  })
})
