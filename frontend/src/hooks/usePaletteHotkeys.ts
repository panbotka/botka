import { useEffect } from 'react'

interface Options {
  onNewChat: () => void
  onTogglePalette: () => void
}

/**
 * Global keyboard shortcuts for the chat surface:
 * - Ctrl/Cmd + Shift + O → new chat
 * - Ctrl/Cmd + K        → toggle the command palette
 *
 * Deliberately small and self-contained: the repo also has an unwired
 * `useKeyboardShortcuts` hook, but it depends on a shortcuts-help modal we
 * do not render, so we bind only the two hotkeys we actually need here.
 */
export function usePaletteHotkeys({ onNewChat, onTogglePalette }: Options): void {
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      const mod = e.ctrlKey || e.metaKey
      if (!mod || e.altKey) return

      if (e.shiftKey && (e.key === 'O' || e.key === 'o')) {
        e.preventDefault()
        onNewChat()
        return
      }
      if (!e.shiftKey && (e.key === 'K' || e.key === 'k')) {
        e.preventDefault()
        onTogglePalette()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onNewChat, onTogglePalette])
}
