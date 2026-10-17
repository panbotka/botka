import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { MemoryRouter } from 'react-router-dom'
import { CHAT_COMMAND_EVENT, type ChatCommandDetail } from '../utils/chatCommands'

// ── Mocks ───────────────────────────────────────────────────────────────────

vi.mock('../components/ChatView', () => ({
  default: () => <div data-testid="chat-view" />,
}))
vi.mock('../components/ThreadSidebar', () => ({
  default: () => <div data-testid="thread-sidebar" />,
}))
vi.mock('../components/ChatTabsBar', () => ({
  default: () => <div data-testid="chat-tabs-bar" />,
}))
vi.mock('../components/CommandButtons', () => ({
  default: () => <div data-testid="command-buttons" />,
}))
vi.mock('../components/ThreadSettingsPanel', () => ({
  default: ({ mobile }: { mobile?: boolean }) => (
    <div data-testid="thread-settings-panel" data-mobile={String(!!mobile)} />
  ),
}))
vi.mock('../components/ThreadForkBadges', () => ({
  default: () => null,
}))
vi.mock('../components/BookmarksBar', () => ({
  default: ({ variant }: { variant: string }) => <div data-testid="bookmarks-bar" data-variant={variant} />,
}))

vi.mock('../hooks/useIsMobile', () => ({
  useIsMobile: () => true,
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
vi.mock('../context/AuthContext', () => ({
  useAuth: () => ({ user: { id: 1, username: 'tester', role: 'admin', passkey_count: 0 } }),
}))
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
    fetchFolders: vi.fn().mockResolvedValue([]),
    fetchPersonas: vi.fn().mockResolvedValue([]),
    fetchProjects: vi.fn().mockResolvedValue([]),
    getThread: vi.fn().mockResolvedValue({ thread: { id: 42 }, messages: [] }),
    getModels: vi.fn().mockResolvedValue({ models: ['sonnet', 'opus'] }),
    fetchBoxProjects: vi.fn().mockResolvedValue({ online: true, projects: [] }),
    createThread: vi.fn(),
    updateThreadProject: vi.fn(),
    updateModel: vi.fn(),
  },
}))

import ChatPage from './ChatPage'

function renderPage() {
  return render(
    <MemoryRouter initialEntries={['/chat/42']}>
      <ChatPage />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('ChatPage mobile header', () => {
  it('is a single row with title, model chip and the ⋯ menu — no bookmarks row', async () => {
    renderPage()
    expect(await screen.findByText('Test thread')).toBeInTheDocument()
    expect(await screen.findByTestId('model-chip')).toHaveTextContent('sonnet')
    expect(screen.getByLabelText('Další akce')).toBeInTheDocument()
    expect(screen.queryByTestId('bookmarks-bar')).not.toBeInTheDocument()
    expect(screen.queryByTestId('command-buttons')).not.toBeInTheDocument()
    expect(screen.queryByLabelText(/Otevřít příkazy/)).not.toBeInTheDocument()
  })

  it('opens the thread settings as a bottom sheet from the title', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(await screen.findByRole('button', { name: 'Thread settings' }))
    expect(screen.getByTestId('thread-settings-panel')).toHaveAttribute('data-mobile', 'true')
  })

  it('routes "Hledat ve vlákně" from the ⋯ menu to the open ChatView', async () => {
    const user = userEvent.setup()
    const seen: ChatCommandDetail[] = []
    const listener = (e: Event) => seen.push((e as CustomEvent<ChatCommandDetail>).detail)
    window.addEventListener(CHAT_COMMAND_EVENT, listener)

    renderPage()
    await screen.findByText('Test thread')
    await user.click(screen.getByLabelText('Další akce'))
    const menu = screen.getByRole('dialog', { name: 'Menu chatu' })
    expect(within(menu).getByTestId('command-buttons')).toBeInTheDocument()
    await user.click(within(menu).getByText('Hledat ve vlákně'))

    expect(seen).toEqual([{ threadId: 42, command: '/find' }])
    expect(screen.queryByRole('dialog', { name: 'Menu chatu' })).not.toBeInTheDocument()
    window.removeEventListener(CHAT_COMMAND_EVENT, listener)
  })
})
