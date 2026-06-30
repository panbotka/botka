import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { usePaletteHotkeys } from './usePaletteHotkeys'

function press(init: KeyboardEventInit) {
  window.dispatchEvent(new KeyboardEvent('keydown', { ...init, bubbles: true, cancelable: true }))
}

describe('usePaletteHotkeys', () => {
  it('fires onNewChat for Ctrl+Shift+O', () => {
    const onNewChat = vi.fn()
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat, onTogglePalette }))
    press({ key: 'O', ctrlKey: true, shiftKey: true })
    expect(onNewChat).toHaveBeenCalledTimes(1)
    expect(onTogglePalette).not.toHaveBeenCalled()
  })

  it('fires onNewChat for Cmd+Shift+O (meta)', () => {
    const onNewChat = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat, onTogglePalette: vi.fn() }))
    press({ key: 'o', metaKey: true, shiftKey: true })
    expect(onNewChat).toHaveBeenCalledTimes(1)
  })

  it('fires onTogglePalette for Cmd+P', () => {
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat: vi.fn(), onTogglePalette }))
    press({ key: 'p', metaKey: true })
    expect(onTogglePalette).toHaveBeenCalledTimes(1)
  })

  it('does NOT fire onTogglePalette for Cmd+K (owned by full-text search)', () => {
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat: vi.fn(), onTogglePalette }))
    press({ key: 'k', metaKey: true })
    expect(onTogglePalette).not.toHaveBeenCalled()
  })

  it('ignores plain O / plain P', () => {
    const onNewChat = vi.fn()
    const onTogglePalette = vi.fn()
    renderHook(() => usePaletteHotkeys({ onNewChat, onTogglePalette }))
    press({ key: 'o' })
    press({ key: 'p' })
    expect(onNewChat).not.toHaveBeenCalled()
    expect(onTogglePalette).not.toHaveBeenCalled()
  })

  it('unbinds on unmount', () => {
    const onTogglePalette = vi.fn()
    const { unmount } = renderHook(() => usePaletteHotkeys({ onNewChat: vi.fn(), onTogglePalette }))
    unmount()
    press({ key: 'p', metaKey: true })
    expect(onTogglePalette).not.toHaveBeenCalled()
  })
})
