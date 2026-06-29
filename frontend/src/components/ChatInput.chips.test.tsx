import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { SettingsProvider } from '../context/SettingsContext'
import ChatInput from './ChatInput'

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
})

// Avoid mic/MediaRecorder in jsdom.
vi.mock('../hooks/useVoiceInput', () => ({
  useVoiceInput: () => ({
    state: 'idle',
    toggle: vi.fn(),
    cancel: vi.fn(),
    isSupported: false,
    mode: 'whisper',
  }),
}))

function renderInput() {
  const onSlashCommand = vi.fn()
  render(
    <SettingsProvider>
      <ChatInput threadId={1} onSend={vi.fn()} onSlashCommand={onSlashCommand} />
    </SettingsProvider>,
  )
  return onSlashCommand
}

describe('ChatInput quick-action chips', () => {
  it('dispatches /new when Nový chat chip is clicked', () => {
    const onSlash = renderInput()
    fireEvent.click(screen.getByLabelText('Nový chat'))
    expect(onSlash).toHaveBeenCalledWith('/new', '')
  })

  it('dispatches /find when Hledat chip is clicked', () => {
    const onSlash = renderInput()
    fireEvent.click(screen.getByLabelText('Hledat'))
    expect(onSlash).toHaveBeenCalledWith('/find', '')
  })

  it('dispatches /clear when Smazat historii chip is clicked', () => {
    const onSlash = renderInput()
    fireEvent.click(screen.getByLabelText('Smazat historii'))
    expect(onSlash).toHaveBeenCalledWith('/clear', '')
  })
})
