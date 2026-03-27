import { describe, it, expect, beforeEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import type { ReactNode } from 'react'
import { SettingsProvider, useSettings } from './SettingsContext'

// Mock matchMedia for system theme detection
beforeEach(() => {
  localStorage.clear()
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: query === '(prefers-color-scheme: dark)' ? false : false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
})

function wrapper({ children }: { children: ReactNode }) {
  return <SettingsProvider>{children}</SettingsProvider>
}

describe('useSettings', () => {
  it('throws when used outside SettingsProvider', () => {
    expect(() => {
      renderHook(() => useSettings())
    }).toThrow('useSettings must be used within SettingsProvider')
  })

  it('returns default settings', () => {
    const { result } = renderHook(() => useSettings(), { wrapper })
    expect(result.current.settings).toEqual({
      theme: 'light',
      fontSize: 'medium',
      sendOnEnter: true,
      notificationsEnabled: false,
      notificationSound: false,
    })
  })

  it('updates settings partially', () => {
    const { result } = renderHook(() => useSettings(), { wrapper })

    act(() => {
      result.current.updateSettings({ theme: 'dark' })
    })

    expect(result.current.settings.theme).toBe('dark')
    // Other settings unchanged
    expect(result.current.settings.fontSize).toBe('medium')
    expect(result.current.settings.sendOnEnter).toBe(true)
  })

  it('persists settings to localStorage', () => {
    const { result } = renderHook(() => useSettings(), { wrapper })

    act(() => {
      result.current.updateSettings({ fontSize: 'large' })
    })

    const stored = JSON.parse(localStorage.getItem('botka-settings')!)
    expect(stored.fontSize).toBe('large')
  })

  it('loads settings from localStorage', () => {
    localStorage.setItem(
      'botka-settings',
      JSON.stringify({ theme: 'dark-blue', fontSize: 'small', sendOnEnter: false, notificationsEnabled: true, notificationSound: true }),
    )

    const { result } = renderHook(() => useSettings(), { wrapper })
    expect(result.current.settings.theme).toBe('dark-blue')
    expect(result.current.settings.fontSize).toBe('small')
    expect(result.current.settings.sendOnEnter).toBe(false)
  })

  it('merges partial stored settings with defaults', () => {
    localStorage.setItem('botka-settings', JSON.stringify({ theme: 'dark-green' }))

    const { result } = renderHook(() => useSettings(), { wrapper })
    expect(result.current.settings.theme).toBe('dark-green')
    expect(result.current.settings.fontSize).toBe('medium') // default
  })

  it('resets settings to defaults', () => {
    const { result } = renderHook(() => useSettings(), { wrapper })

    act(() => {
      result.current.updateSettings({ theme: 'dark', fontSize: 'large' })
    })
    expect(result.current.settings.theme).toBe('dark')

    act(() => {
      result.current.resetSettings()
    })
    expect(result.current.settings.theme).toBe('light')
    expect(result.current.settings.fontSize).toBe('medium')
  })

  it('handles corrupt localStorage gracefully', () => {
    localStorage.setItem('botka-settings', 'not json')

    const { result } = renderHook(() => useSettings(), { wrapper })
    // Falls back to defaults
    expect(result.current.settings.theme).toBe('light')
  })

  it('resolvedTheme returns light when theme is light', () => {
    const { result } = renderHook(() => useSettings(), { wrapper })
    expect(result.current.resolvedTheme).toBe('light')
  })

  it('resolvedTheme returns actual theme for non-system values', () => {
    const { result } = renderHook(() => useSettings(), { wrapper })

    act(() => {
      result.current.updateSettings({ theme: 'dark-green' })
    })

    expect(result.current.resolvedTheme).toBe('dark-green')
  })
})
