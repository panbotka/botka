import { describe, it, expect, vi, beforeAll } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import CommandPalette from './CommandPalette'

// jsdom does not implement scrollIntoView; the palette calls it in an effect
// to keep the selected row visible. Stub it so the contract tests can run.
beforeAll(() => {
  Element.prototype.scrollIntoView = vi.fn()
})

function renderPalette(overrides = {}) {
  const props = {
    open: true,
    onClose: vi.fn(),
    threads: [],
    onSelectThread: vi.fn(),
    onNewThread: vi.fn(),
    onOpenSettings: vi.fn(),
    onToggleTheme: vi.fn(),
    onOpenSearch: vi.fn(),
    ...overrides,
  }
  render(<CommandPalette {...props} />)
  return props
}

describe('CommandPalette', () => {
  it('renders nothing when closed', () => {
    const { container } = render(
      <CommandPalette
        open={false}
        onClose={vi.fn()}
        threads={[]}
        onSelectThread={vi.fn()}
        onNewThread={vi.fn()}
        onOpenSettings={vi.fn()}
        onToggleTheme={vi.fn()}
        onOpenSearch={vi.fn()}
      />,
    )
    expect(container.firstChild).toBeNull()
  })

  it('shows the New chat action when open', () => {
    renderPalette()
    expect(screen.getByText('New chat')).toBeTruthy()
  })

  it('invokes onNewThread when New chat is activated', () => {
    const props = renderPalette()
    const row = screen.getByText('New chat').closest('button, [role="option"]')
    fireEvent.click(row ?? screen.getByText('New chat'))
    expect(props.onNewThread).toHaveBeenCalledTimes(1)
  })
})
