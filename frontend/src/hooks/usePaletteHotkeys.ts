import { useEffect } from 'react'

interface Options {
  onNewChat: () => void
  onTogglePalette: () => void
}

/**
 * Global keyboard shortcuts for the chat surface:
 * - Ctrl/Cmd + Shift + O → new chat
 * - Ctrl/Cmd + P        → toggle the command palette (threads/actions/commands)
 *
 * Ctrl/Cmd + K is deliberately NOT bound here: it is owned by the global
 * full-text message search overlay (App.tsx). Binding the palette to Cmd+K too
 * made both open at once, with the title-only palette covering the real search.
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
      if (!e.shiftKey && (e.key === 'P' || e.key === 'p')) {
        e.preventDefault()
        onTogglePalette()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onNewChat, onTogglePalette])
}
