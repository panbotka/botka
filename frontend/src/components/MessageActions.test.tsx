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

  it('still renders Copy', () => {
    render(
      <MessageActions role="assistant" content="hi" isLastAssistant={false} />,
    )
    // Copy button has aria-label "Copy message" in MessageActions.tsx.
    expect(screen.getByLabelText('Copy message')).toBeTruthy()
  })
})
