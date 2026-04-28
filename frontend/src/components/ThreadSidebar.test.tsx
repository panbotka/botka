import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import type { Thread, GlobalSearchResults } from '../types'

// ── Mocks ───────────────────────────────────────────────────────────────────

const globalSearchMock = vi.fn<(q: string, limit?: number) => Promise<GlobalSearchResults>>()

vi.mock('../api/client', () => ({
  api: {
    renameThread: vi.fn(),
    deleteThread: vi.fn(),
    pinThread: vi.fn(),
    unpinThread: vi.fn(),
    archiveThread: vi.fn(),
    unarchiveThread: vi.fn(),
    getThread: vi.fn().mockResolvedValue({ messages: [] }),
  },
  globalSearch: (q: string, limit?: number) => globalSearchMock(q, limit),
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
}))

vi.mock('../context/SSEContext', () => ({
  useStreamingThreadIds: () => new Set<number>(),
}))

vi.mock('./ChatInput', () => ({
  clearDraft: vi.fn(),
}))

vi.mock('./BoxStatusBadge', () => ({
  default: () => null,
}))

vi.mock('../utils/exportThread', () => ({
  downloadExport: vi.fn(),
}))

// Import after mocks.
import ThreadSidebar from './ThreadSidebar'

// ── Helpers ─────────────────────────────────────────────────────────────────

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: 1,
    title: 'Test thread',
    model: 'sonnet',
    system_prompt: '',
    custom_context: '',
    pinned: false,
    archived: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    tags: [],
    ...overrides,
  }
}

function renderSidebar(props: Partial<React.ComponentProps<typeof ThreadSidebar>> = {}) {
  const defaults: React.ComponentProps<typeof ThreadSidebar> = {
    threads: [makeThread()],
    activeThreadId: null,
    onSelectThread: vi.fn(),
    onNewThread: vi.fn(),
    onThreadsChange: vi.fn(),
    showArchived: false,
    onToggleArchived: vi.fn(),
    personas: [],
    projects: [],
    activeProcessThreadIds: new Set<number>(),
  }
  return render(<ThreadSidebar {...defaults} {...props} />)
}

beforeEach(() => {
  vi.clearAllMocks()
  globalSearchMock.mockReset()
  globalSearchMock.mockResolvedValue({ threads: [], messages: [], projects: [], tasks: [] })
})

function emptyResults(): GlobalSearchResults {
  return { threads: [], messages: [], projects: [], tasks: [] }
}

async function typeSearch(value: string) {
  const input = screen.getByPlaceholderText('Search messages...') as HTMLInputElement
  await act(async () => {
    fireEvent.change(input, { target: { value } })
  })
  // Wait past the 300ms debounce so the search effect fires.
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 350))
  })
}

// ── Tests ───────────────────────────────────────────────────────────────────

describe('ThreadSidebar action menu', () => {
  it('shows exactly the 5 core actions in order: Pin, Rename, Archive, Export, Delete', () => {
    renderSidebar()
    fireEvent.click(screen.getByTitle('Actions'))

    const menuButtons = screen.getAllByRole('button').filter((btn) => {
      const text = btn.textContent?.trim() ?? ''
      return ['Pin', 'Unpin', 'Rename', 'Archive', 'Unarchive', 'Export', 'Delete'].includes(text)
    })

    expect(menuButtons.map((b) => b.textContent?.trim())).toEqual([
      'Pin',
      'Rename',
      'Archive',
      'Export',
      'Delete',
    ])
  })

  it('shows Unpin instead of Pin when the thread is pinned', () => {
    renderSidebar({ threads: [makeThread({ pinned: true })] })
    fireEvent.click(screen.getByTitle('Actions'))
    expect(screen.getByText('Unpin')).toBeInTheDocument()
    expect(screen.queryByText(/^Pin$/)).not.toBeInTheDocument()
  })

  it('hides the Pin entry for archived threads but keeps Unarchive', () => {
    renderSidebar({ threads: [makeThread({ archived: true })], showArchived: true })
    fireEvent.click(screen.getByTitle('Actions'))
    expect(screen.queryByText(/^Pin$/)).not.toBeInTheDocument()
    expect(screen.queryByText(/^Unpin$/)).not.toBeInTheDocument()
    expect(screen.getByText('Unarchive')).toBeInTheDocument()
  })

  it('does not include the removed entries (Change model, Sources, Custom Context, Signal Bridge, MCP Servers, Color, Tags)', () => {
    renderSidebar({
      threads: [makeThread({
        tags: [{ id: 1, name: 'work', color: '#3b82f6', created_at: '2026-01-01T00:00:00Z' }],
      })],
    })
    fireEvent.click(screen.getByTitle('Actions'))

    const menuRoot = screen.getByText('Pin').closest('div')!.parentElement!
    expect(menuRoot.textContent).not.toMatch(/change model/i)
    expect(menuRoot.textContent).not.toMatch(/sources/i)
    expect(menuRoot.textContent).not.toMatch(/custom context/i)
    expect(menuRoot.textContent).not.toMatch(/signal bridge/i)
    expect(menuRoot.textContent).not.toMatch(/mcp servers/i)
    expect(menuRoot.textContent).not.toMatch(/color/i)
    // The "Tags" label specifically lived inside the menu — it should be gone.
    expect(menuRoot.textContent).not.toMatch(/^tags$/i)
  })

  it('hides the menu trigger entirely when readOnly', () => {
    renderSidebar({ readOnly: true })
    expect(screen.queryByTitle('Actions')).not.toBeInTheDocument()
  })
})

describe('ThreadSidebar global search', () => {
  it('renders all four section headers when results contain every category', async () => {
    globalSearchMock.mockResolvedValue({
      threads: [{ id: 42, title: 'Hello world', updated_at: '2026-04-01T00:00:00Z' }],
      messages: [{
        id: 1, thread_id: 42, thread_title: 'Hello world',
        snippet: 'matched <mark>hello</mark> snippet', created_at: '2026-04-01T00:00:00Z',
      }],
      projects: [{ id: 'p1', name: 'Hello cargo', path: '/tmp/hello' }],
      tasks: [{
        id: 't1', title: 'Hello task', status: 'pending',
        project_name: 'Other project', updated_at: '2026-04-01T00:00:00Z',
      }],
    })

    renderSidebar()
    await typeSearch('hello')

    await waitFor(() => {
      expect(globalSearchMock).toHaveBeenCalledWith('hello', expect.any(Number))
    })

    expect(screen.getByText('Threads')).toBeInTheDocument()
    expect(screen.getByText('Messages')).toBeInTheDocument()
    expect(screen.getByText('Projects')).toBeInTheDocument()
    expect(screen.getByText('Tasks')).toBeInTheDocument()
    expect(screen.getByText('Hello task')).toBeInTheDocument()
    expect(screen.getByText('Hello cargo')).toBeInTheDocument()
  })

  it('reverts to the thread list when the search input is empty', async () => {
    renderSidebar({ threads: [makeThread({ id: 7, title: 'Local thread' })] })

    // Empty input shows the thread list.
    expect(screen.getByText('Local thread')).toBeInTheDocument()
    expect(globalSearchMock).not.toHaveBeenCalled()

    // A single character is below the minimum and must not trigger a search.
    await typeSearch('a')
    expect(globalSearchMock).not.toHaveBeenCalled()
    expect(screen.getByText('Local thread')).toBeInTheDocument()

    // Clearing back to empty restores the thread list and section headers stay gone.
    await typeSearch('')
    expect(screen.getByText('Local thread')).toBeInTheDocument()
    expect(screen.queryByText('Threads')).not.toBeInTheDocument()
    expect(screen.queryByText('Messages')).not.toBeInTheDocument()
  })

  it('shows "+N more" when a section returns more than 10 results', async () => {
    const manyThreads = Array.from({ length: 13 }, (_, i) => ({
      id: 100 + i,
      title: `Thread ${i}`,
      updated_at: '2026-04-01T00:00:00Z',
    }))
    globalSearchMock.mockResolvedValue({
      threads: manyThreads,
      messages: [],
      projects: [],
      tasks: [],
    })

    renderSidebar()
    await typeSearch('thread')

    await waitFor(() => {
      expect(screen.getByText('Threads')).toBeInTheDocument()
    })

    // 10 visible, 3 collapsed into the "+N more" affordance.
    expect(screen.getByText('Thread 0')).toBeInTheDocument()
    expect(screen.getByText('Thread 9')).toBeInTheDocument()
    expect(screen.queryByText('Thread 10')).not.toBeInTheDocument()
    expect(screen.getByText(/\+3 more threads/i)).toBeInTheDocument()
  })

  it('shows "No results found" when every category is empty', async () => {
    globalSearchMock.mockResolvedValue(emptyResults())

    renderSidebar()
    await typeSearch('zzznomatch')

    await waitFor(() => {
      expect(screen.getByText('No results found')).toBeInTheDocument()
    })
    expect(screen.queryByText('Threads')).not.toBeInTheDocument()
  })
})

describe('ThreadSidebar prefix syntax', () => {
  const workTag = { id: 1, name: 'work', color: '#3b82f6', created_at: '2026-01-01T00:00:00Z' }
  const homeTag = { id: 2, name: 'home', color: '#10b981', created_at: '2026-01-01T00:00:00Z' }

  it('typing tag:work filters the rendered thread list locally without calling the API', async () => {
    const threads = [
      makeThread({ id: 1, title: 'Work thread', tags: [workTag] }),
      makeThread({ id: 2, title: 'Home thread', tags: [homeTag] }),
      makeThread({ id: 3, title: 'No tags thread' }),
    ]
    renderSidebar({ threads })

    expect(screen.getByText('Work thread')).toBeInTheDocument()
    expect(screen.getByText('Home thread')).toBeInTheDocument()
    expect(screen.getByText('No tags thread')).toBeInTheDocument()

    await typeSearch('tag:work')

    expect(globalSearchMock).not.toHaveBeenCalled()
    expect(screen.getByText('Work thread')).toBeInTheDocument()
    expect(screen.queryByText('Home thread')).not.toBeInTheDocument()
    expect(screen.queryByText('No tags thread')).not.toBeInTheDocument()
  })

  it('renders an active filter chip and removes the filter when × is clicked', async () => {
    const threads = [
      makeThread({ id: 1, title: 'Work thread', tags: [workTag] }),
      makeThread({ id: 2, title: 'Home thread', tags: [homeTag] }),
    ]
    renderSidebar({ threads })

    await typeSearch('tag:work')
    const chip = screen.getByLabelText('Remove filter tag: work')
    expect(chip).toBeInTheDocument()
    expect(screen.queryByText('Home thread')).not.toBeInTheDocument()

    await act(async () => {
      fireEvent.click(chip)
    })

    expect(screen.getByText('Work thread')).toBeInTheDocument()
    expect(screen.getByText('Home thread')).toBeInTheDocument()
    expect(screen.queryByLabelText('Remove filter tag: work')).not.toBeInTheDocument()
  })
})

describe('ThreadSidebar new-chat split-button', () => {
  function makePersona(overrides: Partial<import('../types').Persona> = {}): import('../types').Persona {
    return {
      id: 1,
      name: 'Default',
      system_prompt: '',
      default_model: '',
      icon: '🤖',
      starter_message: '',
      sort_order: 0,
      created_at: '2026-01-01T00:00:00Z',
      updated_at: '2026-01-01T00:00:00Z',
      ...overrides,
    }
  }

  it('primary click triggers onNewThread() with no persona id', () => {
    const onNewThread = vi.fn()
    renderSidebar({ onNewThread, personas: [makePersona({ id: 5, name: 'Coder', icon: '💻' })] })

    fireEvent.click(screen.getByTitle('New chat'))

    expect(onNewThread).toHaveBeenCalledTimes(1)
    expect(onNewThread).toHaveBeenCalledWith()
  })

  it('chevron click opens persona menu and selecting a persona calls onNewThread(personaId)', () => {
    const onNewThread = vi.fn()
    const personas = [
      makePersona({ id: 5, name: 'Coder', icon: '💻' }),
      makePersona({ id: 6, name: 'Writer', icon: '✍️' }),
    ]
    renderSidebar({ onNewThread, personas })

    // Menu starts hidden.
    expect(screen.queryByText('Empty chat')).not.toBeInTheDocument()
    expect(screen.queryByText('Coder')).not.toBeInTheDocument()

    fireEvent.click(screen.getByTitle('Start with persona'))

    // Menu visible: empty-chat row plus each persona.
    expect(screen.getByText('Empty chat')).toBeInTheDocument()
    expect(screen.getByText('Coder')).toBeInTheDocument()
    expect(screen.getByText('Writer')).toBeInTheDocument()

    fireEvent.click(screen.getByText('Coder'))

    expect(onNewThread).toHaveBeenCalledTimes(1)
    expect(onNewThread).toHaveBeenCalledWith(5)
  })

  it('hides the chevron when there are no personas', () => {
    renderSidebar({ personas: [] })

    expect(screen.getByTitle('New chat')).toBeInTheDocument()
    expect(screen.queryByTitle('Start with persona')).not.toBeInTheDocument()
  })

  it('hides the entire split-button in readOnly mode', () => {
    renderSidebar({ readOnly: true, personas: [makePersona()] })

    expect(screen.queryByTitle('New chat')).not.toBeInTheDocument()
    expect(screen.queryByTitle('Start with persona')).not.toBeInTheDocument()
  })
})
