import { describe, it, expect, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import BottomSheet from './BottomSheet'

function renderSheet(props: Partial<React.ComponentProps<typeof BottomSheet>> = {}) {
  const onClose = vi.fn()
  render(
    <BottomSheet open onClose={onClose} ariaLabel="Sheet" backdropTestId="sheet-backdrop" {...props}>
      <p>Body</p>
    </BottomSheet>,
  )
  return onClose
}

describe('BottomSheet', () => {
  it('renders nothing while closed', () => {
    renderSheet({ open: false })
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument()
  })

  it('renders a labelled dialog with a drag handle', () => {
    renderSheet()
    expect(screen.getByRole('dialog', { name: 'Sheet' })).toHaveTextContent('Body')
    expect(screen.getByTestId('bottom-sheet-handle')).toBeInTheDocument()
  })

  it('closes on backdrop tap and Escape', () => {
    const onClose = renderSheet()
    fireEvent.click(screen.getByTestId('sheet-backdrop'))
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).toHaveBeenCalledTimes(2)
  })

  it('leaves Escape to the content when closeOnEscape is off', () => {
    const onClose = renderSheet({ closeOnEscape: false })
    fireEvent.keyDown(window, { key: 'Escape' })
    expect(onClose).not.toHaveBeenCalled()
  })

  it('closes when the handle is dragged down far enough, not on a short drag', () => {
    const onClose = renderSheet()
    const handle = screen.getByTestId('bottom-sheet-handle')
    fireEvent.pointerDown(handle, { clientY: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientY: 130, pointerId: 1 })
    fireEvent.pointerUp(handle, { clientY: 130, pointerId: 1 })
    expect(onClose).not.toHaveBeenCalled()

    fireEvent.pointerDown(handle, { clientY: 100, pointerId: 1 })
    fireEvent.pointerMove(handle, { clientY: 250, pointerId: 1 })
    fireEvent.pointerUp(handle, { clientY: 250, pointerId: 1 })
    expect(onClose).toHaveBeenCalledTimes(1)
  })
})
