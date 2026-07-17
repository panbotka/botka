import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import type { AuthUser } from '../api/client'

// ── Mocks ───────────────────────────────────────────────────────────────────

// Mock all child components to keep this test focused on the cog button visibility logic.
vi.mock('../components/ChatView', () => ({
  default: () => <div data-testid="chat-view" />,
}))
vi.mock('../components/ThreadSidebar', () => ({
  default: () => <div data-testid="thread-sidebar" />,
}))
vi.mock('../components/ChatTabsBar', () => ({
  default: () => <div data-testid="chat-tabs-bar" />,
}))
vi.mock('../components/ProjectPicker', () => ({
  default: () => <div data-testid="project-picker" />,
  isBoxProject: () => false,
}))
vi.mock('../components/CommandButtons', () => ({
  default: () => <div data-testid="command-buttons" />,
}))
vi.mock('../components/BoxRunningIndicator', () => ({
  default: () => <div data-testid="box-running" />,
}))
vi.mock('../components/ThreadSettingsPanel', () => ({
  default: () => <div data-testid="thread-settings-panel" />,
}))
vi.mock('../components/ThreadForkBadges', () => ({
  default: () => <div data-testid="thread-fork-badges" />,
}))
vi.mock('../components/BookmarksBar', () => ({
  default: () => <div data-testid="bookmarks-bar" />,
}))

vi.mock('../hooks/useIsMobile', () => ({
  useIsMobile: () => false,
}))
vi.mock('../hooks/useProcesses', () => ({
  useProcesses: () => ({ processes: [], poll: vi.fn(), killProcess: vi.fn() }),
}))
vi.mock('../hooks/useRefreshOnFocus', () => ({
  useRefreshOnFocus: vi.fn(),
}))
vi.mock('../hooks/useDocumentTitle', () => ({
  useDocumentTitle: vi.fn(),
}))

const mockUseAuth = vi.fn()
vi.mock('../context/AuthContext', () => ({
  useAuth: () => mockUseAuth(),
}))

// ChatPage calls useSettings() (for the command-palette theme toggle); mock it
// like the other contexts so the test needs no SettingsProvider wrapper.
vi.mock('../context/SettingsContext', () => ({
  useSettings: () => ({ settings: { theme: 'light' }, updateSettings: vi.fn() }),
}))

vi.mock('../api/client', () => ({
  api: {
    fetchThreads: vi.fn().mockResolvedValue([
      {
        id: 42,
        title: 'Test thread',
        model: 'sonnet',
        system_prompt: '',
        custom_context: '',
        pinned: false,
        archived: false,
        created_at: '2026-01-01T00:00:00Z',
        updated_at: '2026-01-01T00:00:00Z',
        tags: [],
      },
    ]),
    fetchTags: vi.fn().mockResolvedValue([]),
    fetchPersonas: vi.fn().mockResolvedValue([]),
    fetchProjects: vi.fn().mockResolvedValue([]),
    getThread: vi.fn().mockResolvedValue({ thread: { id: 42 }, messages: [] }),
    createThread: vi.fn(),
    updateThreadProject: vi.fn(),
  },
}))

// Now import the page under test (after mocks are set up).
import ChatPage from './ChatPage'

function renderPage(initialPath = '/chat/42') {
  return render(
    <MemoryRouter initialEntries={[initialPath]}>
      <ChatPage />
    </MemoryRouter>,
  )
}

function fakeUser(role: 'admin' | 'external'): AuthUser {
  return {
    id: 1,
    username: 'tester',
    role,
    passkey_count: 0,
  }
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ChatPage thread settings cog', () => {
  it('renders the cog button for non-external users', async () => {
    mockUseAuth.mockReturnValue({ user: fakeUser('admin') })
    renderPage()
    await waitFor(() => {
      expect(screen.getByLabelText(/thread settings/i)).toBeInTheDocument()
    })
  })

  it('hides the cog button for external users', async () => {
    mockUseAuth.mockReturnValue({ user: fakeUser('external') })
    renderPage()
    // Wait until the chat view has rendered (so the header has had a chance to render too).
    await waitFor(() => {
      expect(screen.getByTestId('chat-view')).toBeInTheDocument()
    })
    expect(screen.queryByLabelText(/thread settings/i)).not.toBeInTheDocument()
  })
})
