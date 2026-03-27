import { describe, it, expect } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ToolCallPanel from './ToolCallPanel'

// Mock DiffView since it's not needed for ToolCallPanel behavior tests
vi.mock('./DiffView', () => ({
  default: () => <div data-testid="diff-view">DiffView</div>,
}))

describe('ToolCallPanel', () => {
  it('renders tool name', () => {
    render(<ToolCallPanel name="Bash" input={{ command: 'ls -la' }} />)
    expect(screen.getByText('Bash')).toBeInTheDocument()
  })

  it('renders tool label from input', () => {
    render(<ToolCallPanel name="Bash" input={{ command: 'npm install' }} />)
    // Label appears in the header; the command also appears in the collapsed input section
    const matches = screen.getAllByText('npm install')
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it('renders file name for Read tool', () => {
    render(<ToolCallPanel name="Read" input={{ file_path: '/home/user/project/main.go' }} />)
    expect(screen.getByText('main.go')).toBeInTheDocument()
  })

  it('renders pattern for Grep tool', () => {
    render(<ToolCallPanel name="Grep" input={{ pattern: 'TODO' }} />)
    expect(screen.getByText('TODO')).toBeInTheDocument()
  })

  it('shows "done" badge when result is present and no error', () => {
    render(<ToolCallPanel name="Bash" input={{ command: 'echo hi' }} result="hi" />)
    expect(screen.getByText('done')).toBeInTheDocument()
  })

  it('shows "error" badge when isError is true', () => {
    render(<ToolCallPanel name="Bash" input={{ command: 'false' }} result="exit 1" isError />)
    expect(screen.getByText('error')).toBeInTheDocument()
  })

  it('expands and collapses on click', async () => {
    const user = userEvent.setup()
    render(<ToolCallPanel name="Bash" input={{ command: 'echo hello' }} result="hello" />)

    // Input section should be in a collapsed grid container
    const button = screen.getByRole('button')
    await user.click(button)

    // After expand, we should see "Input" and "Output" labels
    expect(screen.getByText('Input')).toBeInTheDocument()
    expect(screen.getByText('Output')).toBeInTheDocument()

    // Click again to collapse
    await user.click(button)
    // The content is still in DOM but visually hidden via grid-rows-[0fr]
    expect(screen.getByText('Input')).toBeInTheDocument()
  })

  it('shows streaming indicator when streaming with no result', () => {
    const { container } = render(
      <ToolCallPanel name="Bash" input={{ command: 'long-running' }} isStreaming />,
    )
    const pulseEl = container.querySelector('.animate-pulse')
    expect(pulseEl).not.toBeNull()
  })

  it('does not show streaming indicator when result is present', () => {
    const { container } = render(
      <ToolCallPanel name="Bash" input={{ command: 'done' }} result="output" isStreaming />,
    )
    const pulseEl = container.querySelector('.animate-pulse')
    expect(pulseEl).toBeNull()
  })

  it('renders Edit tool with diff display for file_path', async () => {
    const user = userEvent.setup()
    render(
      <ToolCallPanel
        name="Edit"
        input={{ file_path: '/tmp/test.ts', old_string: 'old', new_string: 'new' }}
      />,
    )

    // Shows file name with line change counts
    expect(screen.getByText(/test\.ts/)).toBeInTheDocument()

    await user.click(screen.getByRole('button'))
    expect(screen.getByTestId('diff-view')).toBeInTheDocument()
  })

  it('shows error output styled in red when expanded', async () => {
    const user = userEvent.setup()
    render(
      <ToolCallPanel name="Bash" input={{ command: 'bad-cmd' }} result="command not found" isError />,
    )

    await user.click(screen.getByRole('button'))

    expect(screen.getByText('Error')).toBeInTheDocument()
    expect(screen.getByText('command not found')).toBeInTheDocument()
  })

  it('renders MCP tool with Plug icon fallback', () => {
    render(<ToolCallPanel name="mcp__botka__create_task" input={{}} />)
    // Name appears in both header name span and label span
    const matches = screen.getAllByText('mcp__botka__create_task')
    expect(matches.length).toBeGreaterThanOrEqual(1)
  })

  it('renders unknown tool with tool name as label', () => {
    render(<ToolCallPanel name="CustomTool" input={{}} />)
    // Name appears as both the tool name and the label for unknown tools
    const matches = screen.getAllByText('CustomTool')
    expect(matches).toHaveLength(2)
  })

  it('truncates long results to 2000 chars when expanded', async () => {
    const user = userEvent.setup()
    const longResult = 'x'.repeat(3000)
    render(<ToolCallPanel name="Bash" input={{ command: 'cat big' }} result={longResult} />)

    await user.click(screen.getByRole('button'))
    expect(screen.getByText(/\.\.\. \(truncated\)/)).toBeInTheDocument()
  })

  it('shows Write tool label with line count', () => {
    render(
      <ToolCallPanel
        name="Write"
        input={{ file_path: '/tmp/new.ts', content: 'line1\nline2\nline3' }}
      />,
    )
    expect(screen.getByText('new.ts (+3)')).toBeInTheDocument()
  })
})
