import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import ChatTabsBar from './ChatTabsBar'
import type { ProcessInfo } from '../hooks/useProcesses'

function mkProcess(id: number, title: string): ProcessInfo {
  return { thread_id: id, thread_title: title, started_at: '', duration_sec: 0 }
}

describe('ChatTabsBar', () => {
  it('renders nothing when there are no active sessions', () => {
    const { container } = render(
      <ChatTabsBar processes={[]} activeThreadId={null} onSelect={vi.fn()} onKill={vi.fn()} />,
    )
    expect(container).toBeEmptyDOMElement()
  })

  it('renders a tab per active session with the thread title', () => {
    render(
      <ChatTabsBar
        processes={[mkProcess(1, 'Alpha'), mkProcess(2, 'Beta')]}
        activeThreadId={1}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    expect(screen.getByText('Alpha')).toBeInTheDocument()
    expect(screen.getByText('Beta')).toBeInTheDocument()
  })

  it('marks the tab matching activeThreadId as selected', () => {
    render(
      <ChatTabsBar
        processes={[mkProcess(1, 'Alpha'), mkProcess(2, 'Beta')]}
        activeThreadId={2}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    const alpha = screen.getByText('Alpha').closest('[role="tab"]')!
    const beta = screen.getByText('Beta').closest('[role="tab"]')!
    expect(beta.getAttribute('aria-selected')).toBe('true')
    expect(alpha.getAttribute('aria-selected')).toBe('false')
  })

  it('invokes onSelect when the tab body is clicked', () => {
    const onSelect = vi.fn()
    render(
      <ChatTabsBar
        processes={[mkProcess(7, 'Gamma')]}
        activeThreadId={null}
        onSelect={onSelect}
        onKill={vi.fn()}
      />,
    )
    fireEvent.click(screen.getByText('Gamma'))
    expect(onSelect).toHaveBeenCalledWith(7)
  })

  it('invokes onKill (and not onSelect) when the X button is clicked', () => {
    const onSelect = vi.fn()
    const onKill = vi.fn()
    render(
      <ChatTabsBar
        processes={[mkProcess(3, 'Delta')]}
        activeThreadId={null}
        onSelect={onSelect}
        onKill={onKill}
      />,
    )
    fireEvent.click(screen.getByLabelText('Close session for Delta'))
    expect(onKill).toHaveBeenCalledWith(3)
    expect(onSelect).not.toHaveBeenCalled()
  })

  it('preserves open order when processes reorder and appends newcomers at the end', () => {
    const { rerender } = render(
      <ChatTabsBar
        processes={[mkProcess(1, 'Alpha'), mkProcess(2, 'Beta')]}
        activeThreadId={null}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    let tabs = screen.getAllByRole('tab')
    expect(tabs.map(t => t.textContent)).toEqual(['Alpha', 'Beta'])

    // Server reorders and adds a new process; order must stay Alpha, Beta, then new Gamma
    rerender(
      <ChatTabsBar
        processes={[mkProcess(3, 'Gamma'), mkProcess(2, 'Beta'), mkProcess(1, 'Alpha')]}
        activeThreadId={null}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    tabs = screen.getAllByRole('tab')
    expect(tabs.map(t => t.textContent)).toEqual(['Alpha', 'Beta', 'Gamma'])
  })

  it('removes tabs when their session ends', () => {
    const { rerender } = render(
      <ChatTabsBar
        processes={[mkProcess(1, 'Alpha'), mkProcess(2, 'Beta')]}
        activeThreadId={null}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    expect(screen.getByText('Beta')).toBeInTheDocument()
    rerender(
      <ChatTabsBar
        processes={[mkProcess(1, 'Alpha')]}
        activeThreadId={null}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    expect(screen.queryByText('Beta')).not.toBeInTheDocument()
    expect(screen.getByText('Alpha')).toBeInTheDocument()
  })

  it('falls back to "Thread {id}" when the title is empty', () => {
    render(
      <ChatTabsBar
        processes={[mkProcess(42, '')]}
        activeThreadId={null}
        onSelect={vi.fn()}
        onKill={vi.fn()}
      />,
    )
    expect(screen.getByText('Thread 42')).toBeInTheDocument()
  })
})
