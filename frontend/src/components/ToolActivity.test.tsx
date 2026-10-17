import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ToolActivity from './ToolActivity'
import type { ToolActivityCall } from './ToolActivity'

vi.mock('./DiffView', () => ({
  default: () => <div data-testid="diff-view">DiffView</div>,
}))

const calls: ToolActivityCall[] = [
  { id: 'a', name: 'Read', input: { file_path: '/repo/chat.go' }, result: 'ok' },
  { id: 'b', name: 'Bash', input: { command: 'make test' } },
]

describe('ToolActivity', () => {
  it('renders nothing without tool calls', () => {
    const { container } = render(<ToolActivity toolCalls={[]} isStreaming />)
    expect(container).toBeEmptyDOMElement()
  })

  it('names the running tool with a spinner and a Czech counter while streaming', () => {
    render(<ToolActivity toolCalls={calls} isStreaming />)
    const row = screen.getByRole('button', { expanded: false })
    expect(row).toHaveTextContent('Bash')
    expect(row).toHaveTextContent('make test')
    expect(row).toHaveTextContent('2 nástroje')
    expect(screen.getByLabelText('Probíhá')).toBeInTheDocument()
  })

  it('shows the last tool without a spinner when nothing is running', () => {
    const done = calls.map((c) => ({ ...c, result: 'ok' }))
    render(<ToolActivity toolCalls={done} isStreaming />)
    expect(screen.getByRole('button', { expanded: false })).toHaveTextContent('Bash')
    expect(screen.queryByLabelText('Probíhá')).not.toBeInTheDocument()
  })

  it('shows the "N tool calls" toggle for a saved message', () => {
    render(<ToolActivity toolCalls={[{ name: 'Read', input: {} }]} />)
    expect(screen.getByRole('button', { expanded: false })).toHaveTextContent('1 tool call')
    expect(screen.queryByLabelText('Probíhá')).not.toBeInTheDocument()
  })

  it('keeps the list expanded across new tool events', async () => {
    const user = userEvent.setup()
    const { rerender } = render(<ToolActivity toolCalls={calls} isStreaming />)
    await user.click(screen.getByRole('button', { expanded: false }))
    expect(screen.getByTestId('tool-activity').querySelector('[aria-expanded="true"]')).not.toBeNull()

    rerender(
      <ToolActivity
        toolCalls={[...calls, { id: 'c', name: 'Grep', input: { pattern: 'foo' } }]}
        isStreaming
      />,
    )
    const row = screen.getByTestId('tool-activity').querySelector('[aria-expanded]')!
    expect(row.getAttribute('aria-expanded')).toBe('true')
    expect(row).toHaveTextContent('3 nástroje')
  })
})
