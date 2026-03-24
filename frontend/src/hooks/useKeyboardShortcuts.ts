import { useEffect } from 'react';
import type { Thread } from '../types';
import { api } from '../api/client';
import { getSidebarThreads } from '../utils/threadOrder';

interface Options {
  threads: Thread[];
  activeThreadId: number | null;
  onSelectThread: (id: number) => void;
  onNewThread: () => void;
  onThreadsChange: () => void;
  settingsOpen: boolean;
  onCloseSettings: () => void;
  shortcutsModalOpen: boolean;
  onToggleShortcutsModal: () => void;
  commandPaletteOpen: boolean;
  onToggleCommandPalette: () => void;
}

export function useKeyboardShortcuts({
  threads,
  activeThreadId,
  onSelectThread,
  onNewThread,
  onThreadsChange,
  settingsOpen,
  onCloseSettings,
  shortcutsModalOpen,
  onToggleShortcutsModal,
  commandPaletteOpen,
  onToggleCommandPalette,
}: Options) {
  useEffect(() => {
    function handler(e: KeyboardEvent) {
      const target = e.target as HTMLElement;
      const tag = target.tagName;
      const inInput = tag === 'INPUT' || tag === 'TEXTAREA' || target.isContentEditable;

      if (e.key === 'k' && (e.ctrlKey || e.metaKey) && !e.shiftKey && !e.altKey) {
        e.preventDefault();
        onToggleCommandPalette();
        return;
      }

      if (e.key === 'Escape') {
        if (commandPaletteOpen) {
          e.preventDefault();
          onToggleCommandPalette();
          return;
        }
        if (shortcutsModalOpen) {
          e.preventDefault();
          onToggleShortcutsModal();
          return;
        }
        if (settingsOpen) {
          e.preventDefault();
          onCloseSettings();
          return;
        }
        if (inInput) {
          (target as HTMLElement).blur();
          return;
        }
        return;
      }

      if (e.key === '?' && !inInput && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        onToggleShortcutsModal();
        return;
      }

      if (e.ctrlKey && e.shiftKey) {
        if (e.key === 'O' || e.key === 'o') {
          e.preventDefault();
          onNewThread();
          setTimeout(() => {
            const textarea = document.querySelector<HTMLTextAreaElement>(
              'textarea[placeholder]',
            );
            textarea?.focus();
          }, 100);
          return;
        }

        if (e.key === 'P' || e.key === 'p') {
          e.preventDefault();
          if (!activeThreadId) return;
          const thread = threads.find((t) => t.id === activeThreadId);
          if (!thread) return;
          const action = thread.pinned ? api.unpinThread : api.pinThread;
          action(activeThreadId).then(() => onThreadsChange()).catch(() => {});
          return;
        }

        if (e.key === 'ArrowUp' || e.key === 'ArrowDown') {
          e.preventDefault();
          const navigable = getSidebarThreads(threads);
          if (navigable.length === 0) return;
          const currentIdx = navigable.findIndex((t) => t.id === activeThreadId);
          let nextIdx: number;
          if (e.key === 'ArrowUp') {
            nextIdx = currentIdx <= 0 ? navigable.length - 1 : currentIdx - 1;
          } else {
            nextIdx = currentIdx >= navigable.length - 1 ? 0 : currentIdx + 1;
          }
          onSelectThread(navigable[nextIdx]!.id);
          return;
        }
      }

      if (e.key === '/' && !inInput && !e.ctrlKey && !e.metaKey && !e.altKey) {
        e.preventDefault();
        const textarea = document.querySelector<HTMLTextAreaElement>(
          'textarea[placeholder]',
        );
        textarea?.focus();
        return;
      }

      if (e.key === 'l' && e.ctrlKey && !e.shiftKey && !e.altKey && !e.metaKey) {
        e.preventDefault();
        const textarea = document.querySelector<HTMLTextAreaElement>(
          'textarea[placeholder]',
        );
        textarea?.focus();
        return;
      }
    }

    window.addEventListener('keydown', handler);
    return () => window.removeEventListener('keydown', handler);
  }, [
    threads,
    activeThreadId,
    onSelectThread,
    onNewThread,
    onThreadsChange,
    settingsOpen,
    onCloseSettings,
    shortcutsModalOpen,
    onToggleShortcutsModal,
    commandPaletteOpen,
    onToggleCommandPalette,
  ]);
}
