import { describe, it, expect, vi } from 'vitest'
import { render, fireEvent } from '@testing-library/react'
import SelectableOptions from './SelectableOptions'

describe('SelectableOptions', () => {
  it('selects the option matching a pressed number key', () => {
    const onSelect = vi.fn()
    // In production (MarkdownContent.tsx) the <li> elements are passed
    // directly as children, mirroring react-markdown's `ol` renderer.
    const { container } = render(
      <SelectableOptions onSelect={onSelect}>
        <li>First option</li>
        <li>Second option</li>
      </SelectableOptions>,
    )
    // The component attaches its keydown handler to its focusable root.
    const root = container.firstChild as HTMLElement
    root.focus()
    fireEvent.keyDown(root, { key: '2' })
    expect(onSelect).toHaveBeenCalledWith('Second option')
  })
})
