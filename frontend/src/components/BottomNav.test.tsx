import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import BottomNav from './BottomNav'

const mockNavigate = vi.fn()

vi.mock('react-router-dom', async () => {
  const actual = await vi.importActual('react-router-dom')
  return {
    ...actual,
    useNavigate: () => mockNavigate,
  }
})

vi.mock('../context/AuthContext', () => ({
  useAuth: vi.fn(),
}))

import { useAuth } from '../context/AuthContext'

const mockUseAuth = vi.mocked(useAuth)

beforeEach(() => {
  vi.clearAllMocks()
})

function renderNav(path = '/') {
  return render(
    <MemoryRouter initialEntries={[path]}>
      <BottomNav />
    </MemoryRouter>,
  )
}

const adminAuth = {
  user: { id: 1, username: 'admin', role: 'admin' as const, passkey_count: 0 },
  isAuthenticated: true,
  isLoading: false,
  login: vi.fn(),
  logout: vi.fn(),
  refreshUser: vi.fn(),
}

const externalAuth = {
  ...adminAuth,
  user: { id: 2, username: 'viewer', role: 'external' as const, passkey_count: 0 },
}

function slotLabels() {
  return within(screen.getByRole('navigation'))
    .getAllByRole('button')
    .map((b) => b.textContent)
}

describe('BottomNav', () => {
  it('renders exactly 5 slots for admin user', () => {
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav()

    expect(slotLabels()).toEqual(['Chat', 'Tasky', 'Box', 'Dashboard', 'Více'])
    // Secondary items live in the sheet, not in the bar.
    expect(screen.queryByText('Settings')).not.toBeInTheDocument()
    expect(screen.queryByText('Stats')).not.toBeInTheDocument()
  })

  it('renders only allowed slots for external user', async () => {
    const user = userEvent.setup()
    mockUseAuth.mockReturnValue(externalAuth)

    renderNav('/chat')

    expect(slotLabels()).toEqual(['Chat', 'Více'])

    await user.click(screen.getByText('Více'))
    const sheet = screen.getByRole('dialog', { name: 'Více' })
    expect(within(sheet).getAllByRole('button').map((b) => b.textContent)).toEqual(['Help'])
  })

  it('opens the Více sheet listing secondary items', async () => {
    const user = userEvent.setup()
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav()
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()

    await user.click(screen.getByText('Více'))
    const sheet = screen.getByRole('dialog', { name: 'Více' })
    expect(within(sheet).getAllByRole('button').map((b) => b.textContent)).toEqual([
      'Stats',
      'Schedules',
      'Cron Jobs',
      'Settings',
      'Help',
    ])
  })

  it('navigates from the sheet and closes it', async () => {
    const user = userEvent.setup()
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav()
    await user.click(screen.getByText('Více'))
    await user.click(within(screen.getByRole('dialog')).getByText('Settings'))

    expect(mockNavigate).toHaveBeenCalledWith('/settings')
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('closes the sheet on backdrop click', async () => {
    const user = userEvent.setup()
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav()
    await user.click(screen.getByText('Více'))
    await user.click(screen.getByTestId('more-backdrop'))

    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('marks Více active on a secondary route', () => {
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav('/settings')

    expect(screen.getByText('Více').closest('button')!.className).toContain('text-amber-600')
    expect(screen.getByText('Dashboard').closest('button')!.className).not.toContain('text-amber-600')
  })

  it('does not mark Více active on a primary route', () => {
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav('/tasks/abc')

    expect(screen.getByText('Tasky').closest('button')!.className).toContain('text-amber-600')
    expect(screen.getByText('Více').closest('button')!.className).not.toContain('text-amber-600')
  })

  it('navigates on tab click', async () => {
    const user = userEvent.setup()
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav()

    await user.click(screen.getByText('Tasky'))
    expect(mockNavigate).toHaveBeenCalledWith('/tasks')
  })

  it('highlights Dashboard only for exact / path', () => {
    mockUseAuth.mockReturnValue(adminAuth)

    renderNav('/chat/123')

    const dashButton = screen.getByText('Dashboard').closest('button')!
    expect(dashButton.className).not.toContain('text-amber-600')

    // Chat should be active (subtree of /chat)
    const chatButton = screen.getByText('Chat').closest('button')!
    expect(chatButton.className).toContain('text-amber-600')
  })
})
