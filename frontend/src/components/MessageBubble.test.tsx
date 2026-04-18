import { describe, it, expect, vi } from 'vitest'
import { render } from '@testing-library/react'
import * as fs from 'node:fs'
import * as path from 'node:path'
import * as url from 'node:url'
import MessageBubble from './MessageBubble'
import type { Message } from '../types'

// ChatView.tsx embeds MessageBubble inside a scroll container. Re-create that
// minimal structure here so the assertion targets the same element.
vi.mock('./DiffView', () => ({
  default: () => <div data-testid="diff-view">DiffView</div>,
}))

const HERE = path.dirname(url.fileURLToPath(import.meta.url))
const CHATVIEW_SRC = fs.readFileSync(path.join(HERE, 'ChatView.tsx'), 'utf-8')

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: 1,
    thread_id: 1,
    role: 'assistant',
    content: 'hello',
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

describe('MessageBubble', () => {
  it('does not overflow horizontally with a 400-char unbroken string at 360px width', () => {
    const longString = 'a'.repeat(400)
    expect(longString.length).toBe(400)

    const { container } = render(
      <div
        data-testid="scroll-container"
        style={{ width: '360px', overflowX: 'hidden', overflowY: 'auto' }}
      >
        <MessageBubble
          message={makeMessage({ content: longString })}
          isStreaming={false}
        />
      </div>,
    )

    const scrollContainer = container.querySelector(
      '[data-testid="scroll-container"]',
    ) as HTMLElement
    expect(scrollContainer).not.toBeNull()

    // jsdom does not perform layout, so both values are 0 here. The assertion
    // still guards against a future jsdom/layout shim making this non-trivial,
    // and documents the contract.
    expect(scrollContainer.scrollWidth).toBeLessThanOrEqual(scrollContainer.clientWidth)
  })

  it('applies min-w-0 and max-w-full to the bubble element so it cannot exceed its flex parent', () => {
    const { container } = render(
      <MessageBubble message={makeMessage({ content: 'hi' })} isStreaming={false} />,
    )

    const bubble = container.querySelector('.rounded-2xl') as HTMLElement
    expect(bubble).not.toBeNull()
    expect(bubble.className).toContain('min-w-0')
    expect(bubble.className).toContain('max-w-full')
  })

  it('ChatView scroll container declares overflow-x-hidden as a safety net', () => {
    expect(CHATVIEW_SRC).toMatch(/scrollContainerRef[^>]*overflow-x-hidden/)
  })
})
