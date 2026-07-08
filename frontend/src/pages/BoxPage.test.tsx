import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, waitFor, act } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { BoxStatus } from '../types'

// ── Mocks ───────────────────────────────────────────────────────────────────

const { fetchBoxStatus, shutdownBox, wakeBox, startBoxService, stopBoxService } = vi.hoisted(() => ({
  fetchBoxStatus: vi.fn(),
  shutdownBox: vi.fn(),
  wakeBox: vi.fn(),
  startBoxService: vi.fn(),
  stopBoxService: vi.fn(),
}))

vi.mock('../api/client', () => ({
  fetchBoxStatus,
  shutdownBox,
  wakeBox,
  startBoxService,
  stopBoxService,
}))

vi.mock('../hooks/useDocumentTitle', () => ({
  useDocumentTitle: () => {},
}))

import BoxPage from './BoxPage'

const onlineStatus: BoxStatus = {
  online: true,
  host: '10.0.0.1',
  services: [],
}

/** Renders the page and drives the Shutdown button through its confirmation dialog. */
async function clickShutdown(user: ReturnType<typeof userEvent.setup>) {
  render(<BoxPage />)
  await screen.findByText('Box Server')
  await user.click(screen.getByRole('button', { name: /^Shutdown$/ }))
  await user.click(await screen.findByRole('button', { name: /Yes, shut down/ }))
}

describe('BoxPage shutdown errors', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    fetchBoxStatus.mockResolvedValue(onlineStatus)
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('shows the server error message when shutdown fails', async () => {
    shutdownBox.mockRejectedValue(
      new Error('shutdown failed: exit status 1: sudo: a password is required'),
    )

    await clickShutdown(userEvent.setup())

    const alert = await screen.findByRole('alert')
    expect(alert).toHaveTextContent('shutdown failed')
    expect(alert).toHaveTextContent('sudo: a password is required')
  })

  it('keeps the shutdown error visible across a successful status poll', async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
    const user = userEvent.setup({ advanceTimers: vi.advanceTimersByTime })
    shutdownBox.mockRejectedValue(new Error('shutdown failed: Permission denied (publickey).'))

    await clickShutdown(user)
    await screen.findByRole('alert')

    // A background poll succeeds; it must not erase the action error.
    const pollsBefore = fetchBoxStatus.mock.calls.length
    await act(async () => {
      await vi.advanceTimersByTimeAsync(11_000)
    })
    expect(fetchBoxStatus.mock.calls.length).toBeGreaterThan(pollsBefore)
    expect(screen.getByRole('alert')).toHaveTextContent('Permission denied')
  })

  it('shows no error banner when shutdown succeeds', async () => {
    shutdownBox.mockResolvedValue({ message: 'shutdown command sent' })

    await clickShutdown(userEvent.setup())

    await waitFor(() => expect(shutdownBox).toHaveBeenCalled())
    expect(screen.queryByRole('alert')).toBeNull()
  })
})
