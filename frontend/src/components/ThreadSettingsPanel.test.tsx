import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import ThreadSettingsPanel from './ThreadSettingsPanel'
import type { Tag, Thread } from '../types'

// ── Mocks ───────────────────────────────────────────────────────────────────

vi.mock('../api/client', () => ({
  api: {
    updateModel: vi.fn(),
    updateThread: vi.fn(),
    updateThreadTags: vi.fn(),
    getModels: vi.fn().mockResolvedValue({ models: [] }),
  },
  // ThreadSourcesEditor / SignalBridgeEditor / CustomContextEditor / MCPServerToggle
  // pull these directly. They aren't called unless those modals are opened in tests.
  fetchThreadSources: vi.fn().mockResolvedValue([]),
  createThreadSource: vi.fn(),
  updateThreadSource: vi.fn(),
  deleteThreadSource: vi.fn(),
  reorderThreadSources: vi.fn(),
  updateCustomContext: vi.fn(),
  getSignalGroups: vi.fn().mockResolvedValue([]),
  getSignalBridge: vi.fn().mockResolvedValue(null),
  setSignalBridge: vi.fn(),
  removeSignalBridge: vi.fn(),
  ApiError: class ApiError extends Error {
    status: number
    constructor(status: number, message: string) {
      super(message)
      this.status = status
    }
  },
}))

vi.mock('../hooks/useMCPServers', () => ({
  useMCPServers: () => ({
    servers: [],
    loading: false,
    error: null,
    toggle: vi.fn(),
  }),
}))

// ── Helpers ─────────────────────────────────────────────────────────────────

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: 1,
    title: 'Test thread',
    model: 'sonnet',
    system_prompt: '',
    custom_context: '',
    persona_id: 7,
    persona_icon: '🤖',
    persona_name: 'Helper',
    pinned: false,
    archived: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    tags: [],
    ...overrides,
  }
}

function makeTag(id: number, name: string, color = '#3b82f6'): Tag {
  return { id, name, color, created_at: '2026-01-01T00:00:00Z' }
}

function renderPanel(props: Partial<React.ComponentProps<typeof ThreadSettingsPanel>> = {}) {
  const defaults = {
    thread: makeThread(),
    tags: [],
    onClose: vi.fn(),
    onThreadsChange: vi.fn(),
  }
  return render(
    <MemoryRouter>
      <ThreadSettingsPanel {...defaults} {...props} />
    </MemoryRouter>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
})

// ── Tests ───────────────────────────────────────────────────────────────────

describe('ThreadSettingsPanel', () => {
  it('renders all 8 sections when a thread is provided', () => {
    renderPanel({ tags: [makeTag(1, 'Work'), makeTag(2, 'Personal')] })

    // Each section's label is rendered as uppercase text — check by name (case-insensitive).
    const labels = [
      'Persona',
      'Model',
      'Color',
      'Tags',
      'Sources',
      'Custom Context',
      'Signal Bridge',
      'MCP Servers',
    ]
    for (const label of labels) {
      expect(screen.getByText(label)).toBeInTheDocument()
    }

    // Persona name is shown
    expect(screen.getByText('Helper')).toBeInTheDocument()

    // Tag names are shown
    expect(screen.getByText('Work')).toBeInTheDocument()
    expect(screen.getByText('Personal')).toBeInTheDocument()
  })

  it('shows "No tags created yet." when there are no tags', () => {
    renderPanel({ tags: [] })
    expect(screen.getByText(/no tags created yet/i)).toBeInTheDocument()
  })

  it('clicking the close button calls onClose', () => {
    const onClose = vi.fn()
    renderPanel({ onClose })
    fireEvent.click(screen.getByLabelText(/close settings/i))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('clicking the backdrop calls onClose', () => {
    const onClose = vi.fn()
    renderPanel({ onClose })
    fireEvent.click(screen.getByTestId('thread-settings-backdrop'))
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('pressing Escape calls onClose', () => {
    const onClose = vi.fn()
    renderPanel({ onClose })
    fireEvent.keyDown(document, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(1)
  })

  it('shows the active Signal Bridge badge when signal_bridge_active is true', () => {
    renderPanel({ thread: makeThread({ signal_bridge_active: true }) })
    expect(screen.getByText('Active')).toBeInTheDocument()
  })

  it('shows the character-count badge when custom_context exists', () => {
    renderPanel({ thread: makeThread({ custom_context: 'hello world' }) })
    expect(screen.getByText(/11 chars/i)).toBeInTheDocument()
  })

})
