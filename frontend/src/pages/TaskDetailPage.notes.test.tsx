import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render } from '@testing-library/react'
import { MemoryRouter, Route, Routes } from 'react-router-dom'
import type { Task } from '../types'

// ── Mocks ───────────────────────────────────────────────────────────────────

const { fetchTask, fetchTaskNotes, fetchTaskTags } = vi.hoisted(() => ({
  fetchTask: vi.fn(),
  fetchTaskNotes: vi.fn(),
  fetchTaskTags: vi.fn(),
}))

vi.mock('../api/client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('../api/client')>()),
  fetchTask,
  fetchTaskNotes,
  fetchTaskTags,
}))

// The SSE hook needs an EventSource, which jsdom does not provide.
vi.mock('../hooks/useTaskEvents', () => ({ useTaskEvents: () => {} }))

import TaskDetailPage from './TaskDetailPage'

declare global {
  // Set by @testing-library/react; toggled off below to observe real render cycles.
  var IS_REACT_ACT_ENVIRONMENT: boolean | undefined
}

const task = {
  id: 'ef0f448c-6426-4d34-abb2-93464a66ce81',
  title: 'Test task',
  spec: 'do the thing',
  status: 'done',
  priority: 1,
  project_id: 'p1',
  created_at: '2026-07-09T10:00:00Z',
  updated_at: '2026-07-09T10:00:00Z',
} as unknown as Task

/**
 * Lets the component render, settle its promises and re-render for real.
 *
 * Deliberately outside `act()`: a self-sustaining refetch loop enqueues work
 * forever, so `act()` would never return and the failure would surface as an
 * opaque timeout instead of a call count.
 */
async function letRenderCyclesElapse(ms = 200) {
  const prev = globalThis.IS_REACT_ACT_ENVIRONMENT
  globalThis.IS_REACT_ACT_ENVIRONMENT = false
  try {
    await new Promise((resolve) => setTimeout(resolve, ms))
  } finally {
    globalThis.IS_REACT_ACT_ENVIRONMENT = prev
  }
}

describe('TaskDetailPage notes loading', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchTask.mockResolvedValue(task)
    fetchTaskNotes.mockResolvedValue([])
    fetchTaskTags.mockResolvedValue([])
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('fetches the notes exactly once per mount, no matter how many renders follow', async () => {
    render(
      <MemoryRouter initialEntries={[`/tasks/${task.id}`]}>
        <Routes>
          <Route path="/tasks/:id" element={<TaskDetailPage />} />
        </Routes>
      </MemoryRouter>,
    )

    await letRenderCyclesElapse()

    expect(fetchTaskNotes).toHaveBeenCalledTimes(1)
  })
})
