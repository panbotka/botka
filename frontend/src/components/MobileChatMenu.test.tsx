import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import MobileChatMenu from './MobileChatMenu'
import type { Project } from '../types'

const fetchBookmarks = vi.fn()
vi.mock('../api/client', () => ({
  api: {
    fetchBoxProjects: vi.fn().mockResolvedValue({ online: true, projects: [] }),
  },
  fetchBookmarks: (...args: unknown[]) => fetchBookmarks(...args),
  createBookmark: vi.fn(),
  deleteBookmark: vi.fn(),
}))

// CommandButtons polls running commands; stub it to a marker that shows which
// project it was handed.
vi.mock('./CommandButtons', () => ({
  default: ({ project, variant }: { project?: Project; variant?: string }) => (
    <div data-testid="command-buttons" data-variant={variant}>{project?.name ?? 'none'}</div>
  ),
}))

function project(overrides: Partial<Project> = {}): Project {
  return {
    id: 'p1',
    name: 'botka',
    path: '/home/pi/projects/botka',
    branch_strategy: 'main',
    active: true,
    claude_md: '',
    sort_order: 0,
    created_at: '',
    updated_at: '',
    ...overrides,
  } as Project
}

function renderMenu(overrides: Partial<React.ComponentProps<typeof MobileChatMenu>> = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    projects: [project(), project({ id: 'p2', name: 'kukatko', path: '/home/pi/projects/kukatko' })],
    project: project(),
    onSelectProject: vi.fn(),
    onOpenPalette: vi.fn(),
    onOpenSettings: vi.fn(),
    onNewChat: vi.fn(),
    onSearchInThread: vi.fn(),
    onClearHistory: vi.fn(),
    ...overrides,
  }
  render(<MobileChatMenu {...props} />)
  return props
}

beforeEach(() => {
  vi.clearAllMocks()
  fetchBookmarks.mockResolvedValue([])
})

describe('MobileChatMenu', () => {
  it('lists every action that moved out of the header and composer', () => {
    renderMenu()
    const sheet = screen.getByRole('dialog', { name: 'Menu chatu' })
    for (const label of [
      'Projekt',
      'Paleta příkazů',
      'Záložky',
      'Nastavení vlákna',
      'Nový chat',
      'Hledat ve vlákně',
      'Smazat historii',
    ]) {
      expect(within(sheet).getByText(label)).toBeInTheDocument()
    }
    // Current project shown next to the Project row; commands get the project in sheet form.
    expect(within(sheet).getAllByText('botka').length).toBeGreaterThan(0)
    expect(screen.getByTestId('command-buttons')).toHaveAttribute('data-variant', 'sheet')
  })

  it('hides write actions for read-only users', () => {
    renderMenu({ readOnly: true })
    expect(screen.queryByText('Nastavení vlákna')).not.toBeInTheDocument()
    expect(screen.queryByText('Nový chat')).not.toBeInTheDocument()
    expect(screen.queryByText('Smazat historii')).not.toBeInTheDocument()
    expect(screen.getByText('Hledat ve vlákně')).toBeInTheDocument()
  })

  it('closes the sheet and runs the action for simple rows', async () => {
    const user = userEvent.setup()
    const props = renderMenu()
    await user.click(screen.getByText('Paleta příkazů'))
    expect(props.onClose).toHaveBeenCalled()
    expect(props.onOpenPalette).toHaveBeenCalled()

    await user.click(screen.getByText('Nový chat'))
    expect(props.onNewChat).toHaveBeenCalled()
    await user.click(screen.getByText('Hledat ve vlákně'))
    expect(props.onSearchInThread).toHaveBeenCalled()
    await user.click(screen.getByText('Nastavení vlákna'))
    expect(props.onOpenSettings).toHaveBeenCalled()
  })

  it('asks for a second tap before clearing the history', async () => {
    const user = userEvent.setup()
    const props = renderMenu()
    await user.click(screen.getByText('Smazat historii'))
    expect(props.onClearHistory).not.toHaveBeenCalled()
    expect(props.onClose).not.toHaveBeenCalled()

    await user.click(screen.getByText('Opravdu smazat? Klepni znovu'))
    expect(props.onClearHistory).toHaveBeenCalledTimes(1)
    expect(props.onClose).toHaveBeenCalled()
  })

  it('opens the project picker as a sub-view and selects a project', async () => {
    const user = userEvent.setup()
    const props = renderMenu()
    await user.click(screen.getByText('Projekt'))
    await user.click(await screen.findByText('kukatko'))
    expect(props.onSelectProject).toHaveBeenCalledWith('p2')
    expect(props.onClose).toHaveBeenCalled()
  })

  it('shows the bookmarks list as a sub-view with a way back', async () => {
    fetchBookmarks.mockResolvedValue([
      { id: 1, url: 'https://example.com', title: 'Example', favicon_url: '', sort_order: 0, created_at: '', updated_at: '' },
    ])
    const user = userEvent.setup()
    renderMenu()
    await user.click(screen.getByText('Záložky'))
    expect(await screen.findByText('Example')).toBeInTheDocument()
    expect(screen.getByText('Example').closest('a')).toHaveAttribute('href', 'https://example.com')

    await user.click(screen.getByLabelText('Zpět'))
    await waitFor(() => expect(screen.getByText('Paleta příkazů')).toBeInTheDocument())
  })
})
