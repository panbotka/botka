import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'

const submitToolResult = vi.fn().mockResolvedValue(undefined)
vi.mock('../api/client', () => ({
  api: {},
  submitToolResult: (...args: unknown[]) => submitToolResult(...args),
}))

import AskUserPanel from './AskUserPanel'

// Minimal tool call with one single-select question.
const toolCall = {
  id: 'tc1',
  name: 'AskUserQuestion',
  input: {
    questions: [
      {
        question: 'Pick one',
        header: 'X',
        multiSelect: false,
        options: [
          { label: 'Alpha', description: '' },
          { label: 'Beta', description: '' },
        ],
      },
    ],
  },
} as never

describe('AskUserPanel', () => {
  it('selects an option when its number key is pressed', () => {
    render(<AskUserPanel toolCall={toolCall} threadId={1} />)
    fireEvent.keyDown(window, { key: '2' })
    const beta = screen.getByText('Beta').closest('button')!
    expect(beta.getAttribute('aria-pressed')).toBe('true')
    const alpha = screen.getByText('Alpha').closest('button')!
    expect(alpha.getAttribute('aria-pressed')).toBe('false')
  })

  it('ignores number keys typed into an editable element', () => {
    render(<AskUserPanel toolCall={toolCall} threadId={1} />)
    const textarea = document.createElement('textarea')
    document.body.appendChild(textarea)
    fireEvent.keyDown(textarea, { key: '2' })
    const beta = screen.getByText('Beta').closest('button')!
    expect(beta.getAttribute('aria-pressed')).toBe('false')
    const alpha = screen.getByText('Alpha').closest('button')!
    expect(alpha.getAttribute('aria-pressed')).toBe('false')
    document.body.removeChild(textarea)
  })
})
