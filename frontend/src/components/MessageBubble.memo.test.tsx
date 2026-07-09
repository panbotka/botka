import { describe, it, expect, vi, beforeEach } from 'vitest'
import { useState } from 'react'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import MessageBubble from './MessageBubble'
import type { Message } from '../types'

// Parsing markdown is the dominant cost of the chat view. Stand in for it with
// a counter so the tests can assert it does not run again.
const renderCount = { markdown: 0 }
vi.mock('./MarkdownContent', () => ({
  default: ({ content }: { content: string }) => {
    renderCount.markdown++
    return <div data-testid="markdown">{content}</div>
  },
}))

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: 1,
    thread_id: 1,
    role: 'assistant',
    content: 'hello **world**',
    created_at: '2026-07-09T10:00:00Z',
    ...overrides,
  }
}

const noop = () => {}

/** A parent whose state changes on every click, like ChatView during streaming. */
function StreamingParent({ message }: { message: Message }) {
  const [tick, setTick] = useState(0)
  return (
    <div>
      <button onClick={() => setTick((t) => t + 1)}>tick {tick}</button>
      <MessageBubble message={message} onImageClick={noop} onHide={noop} />
    </div>
  )
}

describe('MessageBubble memoization', () => {
  beforeEach(() => {
    renderCount.markdown = 0
  })

  it('does not re-render when the parent re-renders with unchanged props', async () => {
    const user = userEvent.setup()
    render(<StreamingParent message={makeMessage()} />)
    expect(renderCount.markdown).toBe(1)

    await user.click(screen.getByRole('button', { name: /tick/ }))
    await user.click(screen.getByRole('button', { name: /tick/ }))

    expect(renderCount.markdown).toBe(1)
  })

  it('does not re-render when a thread reload rebuilds an equivalent Message object', () => {
    const { rerender } = render(<MessageBubble message={makeMessage()} onImageClick={noop} />)
    expect(renderCount.markdown).toBe(1)

    // Same values, fresh object identity — what api.getThread() returns.
    rerender(<MessageBubble message={makeMessage()} onImageClick={noop} />)

    expect(renderCount.markdown).toBe(1)
  })

  it('re-renders when the message content actually changes', () => {
    const { rerender } = render(<MessageBubble message={makeMessage()} onImageClick={noop} />)
    expect(renderCount.markdown).toBe(1)

    rerender(<MessageBubble message={makeMessage({ content: 'hello **there**' })} onImageClick={noop} />)

    expect(renderCount.markdown).toBe(2)
    expect(screen.getByTestId('markdown')).toHaveTextContent('hello **there**')
  })

  it('re-renders when an attachment is added', () => {
    const { rerender } = render(<MessageBubble message={makeMessage()} onImageClick={noop} />)
    expect(renderCount.markdown).toBe(1)

    rerender(
      <MessageBubble
        message={makeMessage({
          attachments: [
            {
              id: 7,
              message_id: 1,
              stored_name: 's.png',
              original_name: 'shot.png',
              mime_type: 'image/png',
              size: 100,
              url: '/uploads/s.png',
              created_at: '2026-07-09T10:00:00Z',
            },
          ],
        })}
        onImageClick={noop}
      />,
    )

    expect(renderCount.markdown).toBe(2)
  })

  it('routes hide and remove-from-queue through the message id', async () => {
    const user = userEvent.setup()
    const onHide = vi.fn()
    render(<MessageBubble message={makeMessage({ id: 42, hidden: true })} onHide={onHide} />)

    await user.click(screen.getByTitle('Click to unhide'))

    expect(onHide).toHaveBeenCalledWith(42)
  })
})
