import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, screen, fireEvent, act } from '@testing-library/react'
import MessageBubble from './MessageBubble'
import type { Message } from '../types'
import { LONG_PRESS_DELAY_MS } from '../hooks/useLongPress'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: 7,
    thread_id: 1,
    role: 'assistant',
    content: 'hello **world**',
    created_at: new Date().toISOString(),
    ...overrides,
  }
}

const touch = (x: number, y: number) => ({ touches: [{ clientX: x, clientY: y }] })

function longPress(el: HTMLElement) {
  fireEvent.touchStart(el, touch(10, 10))
  act(() => {
    vi.advanceTimersByTime(LONG_PRESS_DELAY_MS)
  })
  fireEvent.touchEnd(el, { touches: [] })
}

function sheetLabels(): string[] {
  const dialog = screen.getByRole('dialog', { name: 'Akce zprávy' })
  return Array.from(dialog.querySelectorAll('button')).map((b) => b.textContent ?? '')
}

const handlers = () => ({
  onEdit: vi.fn(),
  onRegenerate: vi.fn(),
  onHide: vi.fn(),
  onFork: vi.fn(),
  onCopied: vi.fn(),
})

describe('MessageBubble long-press action sheet', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })
  afterEach(() => {
    vi.useRealTimers()
  })

  it('opens with Copy, Edit, Fork and Hide for a user message', () => {
    const h = handlers()
    render(<MessageBubble message={makeMessage({ role: 'user' })} isMobile {...h} />)
    longPress(screen.getByTestId('message-bubble'))
    expect(sheetLabels()).toEqual(['Kopírovat', 'Upravit', 'Forknout odsud', 'Skrýt'])
  })

  it('opens with Copy, Regenerate, Fork and Hide for the last assistant message', () => {
    const h = handlers()
    render(<MessageBubble message={makeMessage()} isMobile isLastAssistant {...h} />)
    longPress(screen.getByTestId('message-bubble'))
    expect(sheetLabels()).toEqual(['Kopírovat', 'Regenerovat', 'Forknout odsud', 'Skrýt'])
  })

  it('omits Regenerate for an earlier assistant message and Edit for assistants', () => {
    const h = handlers()
    render(<MessageBubble message={makeMessage()} isMobile isLastAssistant={false} {...h} />)
    longPress(screen.getByTestId('message-bubble'))
    expect(sheetLabels()).toEqual(['Kopírovat', 'Forknout odsud', 'Skrýt'])
  })

  it('fork calls onFork with the message id and closes the sheet', () => {
    const h = handlers()
    render(<MessageBubble message={makeMessage({ id: 42 })} isMobile {...h} />)
    longPress(screen.getByTestId('message-bubble'))
    fireEvent.click(screen.getByText('Forknout odsud'))
    expect(h.onFork).toHaveBeenCalledWith(42)
    expect(screen.queryByRole('dialog', { name: 'Akce zprávy' })).toBeNull()
  })

  it('copy writes the markdown to the clipboard and reports it', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', { value: { writeText }, configurable: true })
    const h = handlers()
    render(<MessageBubble message={makeMessage()} isMobile {...h} />)
    longPress(screen.getByTestId('message-bubble'))
    await act(async () => {
      fireEvent.click(screen.getByText('Kopírovat'))
    })
    expect(writeText).toHaveBeenCalledWith('hello **world**')
    expect(h.onCopied).toHaveBeenCalledOnce()
  })

  it('vibrates lightly when the press fires', () => {
    const vibrate = vi.fn()
    Object.defineProperty(navigator, 'vibrate', { value: vibrate, configurable: true })
    render(<MessageBubble message={makeMessage()} isMobile {...handlers()} />)
    longPress(screen.getByTestId('message-bubble'))
    expect(vibrate).toHaveBeenCalledWith(10)
  })

  it('does not open when the finger moves (scroll)', () => {
    render(<MessageBubble message={makeMessage()} isMobile {...handlers()} />)
    const bubble = screen.getByTestId('message-bubble')
    fireEvent.touchStart(bubble, touch(10, 10))
    fireEvent.touchMove(bubble, touch(10, 30))
    act(() => {
      vi.advanceTimersByTime(LONG_PRESS_DELAY_MS * 2)
    })
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('does not open on a short tap', () => {
    render(<MessageBubble message={makeMessage()} isMobile {...handlers()} />)
    const bubble = screen.getByTestId('message-bubble')
    fireEvent.touchStart(bubble, touch(10, 10))
    act(() => {
      vi.advanceTimersByTime(100)
    })
    fireEvent.touchEnd(bubble, { touches: [] })
    act(() => {
      vi.advanceTimersByTime(LONG_PRESS_DELAY_MS)
    })
    expect(screen.queryByRole('dialog')).toBeNull()
  })

  it('suppresses text selection while pressing', () => {
    render(<MessageBubble message={makeMessage()} isMobile {...handlers()} />)
    const bubble = screen.getByTestId('message-bubble')
    expect(bubble.className).not.toContain('select-none')
    fireEvent.touchStart(bubble, touch(10, 10))
    expect(bubble.className).toContain('select-none')
    fireEvent.touchEnd(bubble, { touches: [] })
    expect(bubble.className).not.toContain('select-none')
  })

  it('renders no hover action row on mobile', () => {
    render(<MessageBubble message={makeMessage()} isMobile {...handlers()} />)
    expect(screen.queryByLabelText('Copy message')).toBeNull()
  })

  it('keeps hover actions (incl. fork) and ignores long-press on desktop', () => {
    render(<MessageBubble message={makeMessage()} isMobile={false} {...handlers()} />)
    expect(screen.getByLabelText('Copy message')).toBeTruthy()
    expect(screen.getByLabelText('Forknout odsud')).toBeTruthy()
    longPress(screen.getByTestId('message-bubble'))
    expect(screen.queryByRole('dialog')).toBeNull()
  })
})
