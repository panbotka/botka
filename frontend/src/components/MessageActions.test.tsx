import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import MessageActions from './MessageActions'

describe('MessageActions', () => {
  it('does not render Branch or Fork actions for an assistant message', () => {
    render(
      <MessageActions
        role="assistant"
        content="hi"
        isLastAssistant={false}
        onRegenerate={vi.fn()}
        onHide={vi.fn()}
      />,
    )
    expect(screen.queryByLabelText('Branch from here')).toBeNull()
    expect(screen.queryByLabelText('Fork thread from here')).toBeNull()
  })

  it('renders "Forknout odsud" when onFork is passed and calls it', () => {
    const onFork = vi.fn()
    render(
      <MessageActions role="assistant" content="hi" isLastAssistant={false} onFork={onFork} />,
    )
    const btn = screen.getByLabelText('Forknout odsud')
    btn.click()
    expect(onFork).toHaveBeenCalledOnce()
  })

  it('omits the fork button without onFork', () => {
    render(<MessageActions role="user" content="hi" isLastAssistant={false} />)
    expect(screen.queryByLabelText('Forknout odsud')).toBeNull()
  })

  it('still renders Copy', () => {
    render(
      <MessageActions role="assistant" content="hi" isLastAssistant={false} />,
    )
    // Copy button has aria-label "Copy message" in MessageActions.tsx.
    expect(screen.getByLabelText('Copy message')).toBeTruthy()
  })
})
