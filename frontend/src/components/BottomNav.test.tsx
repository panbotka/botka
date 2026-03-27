import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
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

describe('BottomNav', () => {
  it('renders all tabs for admin user', () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: 'admin', role: 'admin', passkey_count: 0 },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshUser: vi.fn(),
    })

    renderNav()

    expect(screen.getByText('Dashboard')).toBeInTheDocument()
    expect(screen.getByText('Chat')).toBeInTheDocument()
    expect(screen.getByText('Tasks')).toBeInTheDocument()
    expect(screen.getByText('Cost')).toBeInTheDocument()
    expect(screen.getByText('Settings')).toBeInTheDocument()
    expect(screen.getByText('Help')).toBeInTheDocument()
  })

  it('renders only non-admin tabs for regular user', () => {
    mockUseAuth.mockReturnValue({
      user: { id: 2, username: 'viewer', role: 'external', passkey_count: 0 },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshUser: vi.fn(),
    })

    renderNav()

    expect(screen.getByText('Chat')).toBeInTheDocument()
    expect(screen.getByText('Help')).toBeInTheDocument()
    expect(screen.queryByText('Dashboard')).not.toBeInTheDocument()
    expect(screen.queryByText('Tasks')).not.toBeInTheDocument()
    expect(screen.queryByText('Cost')).not.toBeInTheDocument()
    expect(screen.queryByText('Settings')).not.toBeInTheDocument()
  })

  it('highlights active tab', () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: 'admin', role: 'admin', passkey_count: 0 },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshUser: vi.fn(),
    })

    renderNav('/chat')

    const chatButton = screen.getByText('Chat').closest('button')!
    expect(chatButton.className).toContain('text-amber-600')

    const dashButton = screen.getByText('Dashboard').closest('button')!
    expect(dashButton.className).not.toContain('text-amber-600')
  })

  it('navigates on tab click', async () => {
    const user = userEvent.setup()
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: 'admin', role: 'admin', passkey_count: 0 },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshUser: vi.fn(),
    })

    renderNav()

    await user.click(screen.getByText('Tasks'))
    expect(mockNavigate).toHaveBeenCalledWith('/tasks')
  })

  it('highlights Dashboard only for exact / path', () => {
    mockUseAuth.mockReturnValue({
      user: { id: 1, username: 'admin', role: 'admin', passkey_count: 0 },
      isAuthenticated: true,
      isLoading: false,
      login: vi.fn(),
      logout: vi.fn(),
      refreshUser: vi.fn(),
    })

    renderNav('/chat/123')

    const dashButton = screen.getByText('Dashboard').closest('button')!
    expect(dashButton.className).not.toContain('text-amber-600')

    // Chat should be active (startsWith /chat)
    const chatButton = screen.getByText('Chat').closest('button')!
    expect(chatButton.className).toContain('text-amber-600')
  })
})
