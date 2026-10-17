import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SettingsProvider } from '../context/SettingsContext'
import ChatInput from './ChatInput'

const desktopWidth = window.innerWidth

// SettingsProvider reads system theme via matchMedia, which jsdom lacks.
beforeEach(() => {
  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn().mockImplementation((query: string) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })
  // useIsMobile reads innerWidth on first render.
  Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: 390 })
})

afterEach(() => {
  Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: desktopWidth })
})

vi.mock('../hooks/useVoiceInput', () => ({
  useVoiceInput: () => ({
    state: 'idle',
    toggle: vi.fn(),
    cancel: vi.fn(),
    isSupported: false,
    mode: 'whisper',
  }),
}))

describe('ChatInput on mobile', () => {
  it('drops the New chat / Search / Clear history chips', () => {
    render(
      <SettingsProvider>
        <ChatInput threadId={1} onSend={vi.fn()} onSlashCommand={vi.fn()} />
      </SettingsProvider>,
    )
    for (const label of ['Nový chat', 'Hledat', 'Smazat historii']) {
      expect(screen.queryByLabelText(label)).not.toBeInTheDocument()
    }
    expect(screen.queryByText('Shift+Tab')).not.toBeInTheDocument()
  })

  it('renders plan/act as an icon toggle in the input row next to attach', () => {
    const onToggle = vi.fn()
    render(
      <SettingsProvider>
        <ChatInput threadId={1} onSend={vi.fn()} onSlashCommand={vi.fn()} planMode={false} onTogglePlanMode={onToggle} />
      </SettingsProvider>,
    )
    const toggle = screen.getByRole('button', { name: /Režim Act/ })
    expect(toggle).toHaveAttribute('aria-pressed', 'false')
    expect(toggle).not.toHaveTextContent('Act')
    const inputRow = screen.getByPlaceholderText('Message Pan Botka...').parentElement!
    expect(inputRow.contains(toggle)).toBe(true)
    expect(inputRow.contains(screen.getByLabelText('Attach files'))).toBe(true)

    fireEvent.click(toggle)
    expect(onToggle).toHaveBeenCalledTimes(1)
  })
})
