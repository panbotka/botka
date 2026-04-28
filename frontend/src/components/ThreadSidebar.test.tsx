import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import type { Thread } from '../types'

// ── Mocks ───────────────────────────────────────────────────────────────────

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
  searchMessages: vi.fn().mockResolvedValue([]),
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
    tags: [],
    selectedTagIds: [],
    onToggleTagFilter: vi.fn(),
    onClearTagFilter: vi.fn(),
    personas: [],
    projects: [],
    selectedProjectId: null,
    onSelectProject: vi.fn(),
    activeProcessThreadIds: new Set<number>(),
  }
  return render(<ThreadSidebar {...defaults} {...props} />)
}

beforeEach(() => {
  vi.clearAllMocks()
})

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
      threads: [makeThread()],
      // Even with tags passed in, the menu should not show a Tags submenu.
      tags: [{ id: 1, name: 'work', color: '#3b82f6', created_at: '2026-01-01T00:00:00Z' }],
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
