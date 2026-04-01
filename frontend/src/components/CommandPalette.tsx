import { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import type { Thread } from '../types';
import { COMMANDS } from './SlashCommandMenu';

interface Props {
  open: boolean;
  onClose: () => void;
  threads: Thread[];
  onSelectThread: (id: number) => void;
  onNewThread: () => void;
  onOpenSettings: () => void;
  onToggleTheme: () => void;
  onOpenSearch: () => void;
}

interface ResultItem {
  id: string;
  category: string;
  categoryLabel: string;
  icon: 'chat' | 'action' | 'slash';
  label: string;
  subtitle?: string;
  shortcut?: string[];
  action: () => void;
}

function fuzzyMatch(query: string, text: string, keywords?: string[]): number {
  const q = query.toLowerCase();
  const t = text.toLowerCase();

  if (t.startsWith(q)) return 100;

  const words = t.split(/[\s/]+/);
  if (words.some((w) => w.startsWith(q))) return 80;

  if (keywords?.some((k) => k.startsWith(q))) return 70;

  if (t.includes(q)) return 50;

  if (keywords?.some((k) => k.includes(q))) return 30;

  return 0;
}

function highlightMatch(text: string, query: string) {
  if (!query) return text;
  const idx = text.toLowerCase().indexOf(query.toLowerCase());
  if (idx === -1) return text;
  return (
    <>
      {text.slice(0, idx)}
      <span className="text-emerald-600">{text.slice(idx, idx + query.length)}</span>
      {text.slice(idx + query.length)}
    </>
  );
}

function insertIntoInput(text: string) {
  const textarea = document.querySelector<HTMLTextAreaElement>('textarea[placeholder]');
  if (!textarea) return;
  const setter = Object.getOwnPropertyDescriptor(
    window.HTMLTextAreaElement.prototype,
    'value',
  )?.set;
  setter?.call(textarea, text);
  textarea.dispatchEvent(new Event('input', { bubbles: true }));
  textarea.focus();
}

interface ActionDef {
  id: string;
  label: string;
  keywords: string[];
  shortcut?: string[];
}

const ACTIONS: ActionDef[] = [
  { id: 'new-chat', label: 'New chat', keywords: ['new', 'create', 'chat'], shortcut: ['Ctrl', '\u21E7', 'O'] },
  { id: 'settings', label: 'Settings', keywords: ['settings', 'preferences', 'config'] },
  { id: 'export', label: 'Export thread', keywords: ['export', 'download', 'save'] },
  { id: 'toggle-theme', label: 'Toggle theme', keywords: ['theme', 'dark', 'light', 'green', 'blue'] },
  { id: 'search', label: 'Search messages', keywords: ['search', 'find'] },
];

function ChatIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
      <path fillRule="evenodd" d="M3.43 2.524A41.29 41.29 0 0110 2c2.236 0 4.43.18 6.57.524 1.437.231 2.43 1.49 2.43 2.902v5.148c0 1.413-.993 2.67-2.43 2.902a41.202 41.202 0 01-5.183.501.78.78 0 00-.528.224l-3.579 3.58A.75.75 0 016 17.03v-3.49a.75.75 0 00-.663-.744 41.18 41.18 0 01-1.907-.33C2.993 12.244 2 10.987 2 9.574V5.426c0-1.413.993-2.67 2.43-2.902z" clipRule="evenodd" />
    </svg>
  );
}

function ActionIcon() {
  return (
    <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" className="w-4 h-4">
      <path fillRule="evenodd" d="M11.3 1.046A1 1 0 0112 2v5h4a1 1 0 01.82 1.573l-7 10A1 1 0 018 18v-5H4a1 1 0 01-.82-1.573l7-10a1 1 0 011.12-.381z" clipRule="evenodd" />
    </svg>
  );
}

export default function CommandPalette({
  open,
  onClose,
  threads,
  onSelectThread,
  onNewThread,
  onOpenSettings,
  onToggleTheme,
  onOpenSearch,
}: Props) {
  const [query, setQuery] = useState('');
  const [selectedIndex, setSelectedIndex] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (open) {
      setQuery('');
      setSelectedIndex(0);
      setTimeout(() => inputRef.current?.focus(), 0);
    }
  }, [open]);

  const actionHandlers: Record<string, () => void> = useMemo(
    () => ({
      'new-chat': () => { onClose(); onNewThread(); },
      'settings': () => { onClose(); onOpenSettings(); },
      'export': () => { onClose(); insertIntoInput('/export '); },
      'toggle-theme': () => { onClose(); onToggleTheme(); },
      'search': () => { onClose(); onOpenSearch(); },
    }),
    [onClose, onNewThread, onOpenSettings, onToggleTheme, onOpenSearch],
  );

  const results = useMemo((): ResultItem[] => {
    const items: ResultItem[] = [];
    const q = query.trim();

    if (!q) {
      const recent = [...threads]
        .filter((t) => !t.archived)
        .sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
        .slice(0, 5);

      for (const thread of recent) {
        items.push({
          id: `thread-${thread.id}`,
          category: 'recent',
          categoryLabel: 'Recent',
          icon: 'chat',
          label: thread.title || 'New conversation',
          subtitle: thread.model || 'Default',
          action: () => { onClose(); onSelectThread(thread.id); },
        });
      }

      for (const act of ACTIONS) {
        items.push({
          id: `action-${act.id}`,
          category: 'actions',
          categoryLabel: 'Actions',
          icon: 'action',
          label: act.label,
          shortcut: act.shortcut,
          action: actionHandlers[act.id] ?? (() => {}),
        });
      }

      return items;
    }

    type ScoredItem = ResultItem & { score: number };
    const scored: ScoredItem[] = [];

    for (const thread of threads.filter((t) => !t.archived)) {
      const title = thread.title || 'New conversation';
      const score = fuzzyMatch(q, title);
      if (score > 0) {
        scored.push({
          id: `thread-${thread.id}`,
          category: 'threads',
          categoryLabel: 'Threads',
          icon: 'chat',
          label: title,
          subtitle: thread.model || 'Default',
          action: () => { onClose(); onSelectThread(thread.id); },
          score,
        });
      }
    }

    for (const act of ACTIONS) {
      const score = fuzzyMatch(q, act.label, [...act.keywords]);
      if (score > 0) {
        scored.push({
          id: `action-${act.id}`,
          category: 'actions',
          categoryLabel: 'Actions',
          icon: 'action',
          label: act.label,
          shortcut: act.shortcut,
          action: actionHandlers[act.id] ?? (() => {}),
          score,
        });
      }
    }

    for (const cmd of COMMANDS) {
      const score = Math.max(
        fuzzyMatch(q, cmd.name, [cmd.name.slice(1)]),
        fuzzyMatch(q, cmd.description),
      );
      if (score > 0) {
        scored.push({
          id: `cmd-${cmd.name}`,
          category: 'commands',
          categoryLabel: 'Slash Commands',
          icon: 'slash',
          label: cmd.name,
          subtitle: cmd.description,
          action: () => { onClose(); insertIntoInput(cmd.name + ' '); },
          score,
        });
      }
    }

    scored.sort((a, b) => b.score - a.score);

    const categoryOrder = ['threads', 'actions', 'commands'];
    const grouped = new Map<string, ScoredItem[]>();
    for (const item of scored) {
      const list = grouped.get(item.category) || [];
      list.push(item);
      grouped.set(item.category, list);
    }

    for (const cat of categoryOrder) {
      const list = grouped.get(cat);
      if (list) items.push(...list);
    }

    return items.slice(0, 10);
  }, [query, threads, onClose, onSelectThread, actionHandlers]);

  useEffect(() => {
    setSelectedIndex(0);
  }, [results.length, query]);

  useEffect(() => {
    const el = listRef.current?.querySelector('[data-selected="true"]') as HTMLElement | null;
    el?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev + 1) % Math.max(results.length, 1));
      } else if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedIndex((prev) => (prev - 1 + results.length) % Math.max(results.length, 1));
      } else if (e.key === 'Enter' && results.length > 0) {
        e.preventDefault();
        results[selectedIndex]?.action();
      } else if (e.key === 'Escape') {
        e.preventDefault();
        onClose();
      }
    },
    [results, selectedIndex, onClose],
  );

  if (!open) return null;

  let lastCategory = '';

  return (
    <div className="fixed inset-0 z-50 flex items-start justify-center pt-[15vh]">
      <div className="absolute inset-0 bg-black/30 backdrop-blur-sm" onClick={onClose} />
      <div
        className="relative bg-zinc-100 border border-zinc-200 rounded-2xl shadow-2xl w-full max-w-md mx-4 overflow-hidden animate-palette-in"
        onKeyDown={handleKeyDown}
      >
        <div className="flex items-center gap-3 px-4 py-3 border-b border-zinc-200">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            viewBox="0 0 20 20"
            fill="currentColor"
            className="w-4 h-4 text-zinc-400 flex-shrink-0"
          >
            <path
              fillRule="evenodd"
              d="M9 3.5a5.5 5.5 0 100 11 5.5 5.5 0 000-11zM2 9a7 7 0 1112.452 4.391l3.328 3.329a.75.75 0 11-1.06 1.06l-3.329-3.328A7 7 0 012 9z"
              clipRule="evenodd"
            />
          </svg>
          <input
            ref={inputRef}
            type="text"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search threads, actions..."
            className="flex-1 bg-transparent text-sm text-zinc-900 placeholder-zinc-400 outline-none"
          />
          {query && (
            <button
              onClick={() => setQuery('')}
              className="text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer"
            >
              <svg
                xmlns="http://www.w3.org/2000/svg"
                viewBox="0 0 20 20"
                fill="currentColor"
                className="w-4 h-4"
              >
                <path d="M6.28 5.22a.75.75 0 00-1.06 1.06L8.94 10l-3.72 3.72a.75.75 0 101.06 1.06L10 11.06l3.72 3.72a.75.75 0 101.06-1.06L11.06 10l3.72-3.72a.75.75 0 00-1.06-1.06L10 8.94 6.28 5.22z" />
              </svg>
            </button>
          )}
          <kbd className="text-[10px] text-zinc-400 bg-zinc-200 px-1.5 py-0.5 rounded border border-zinc-300 font-mono">
            Esc
          </kbd>
        </div>

        <div ref={listRef} className="max-h-72 overflow-y-auto py-1">
          {results.length === 0 && query && (
            <div className="px-4 py-8 text-center text-sm text-zinc-400">
              No results for &ldquo;{query}&rdquo;
            </div>
          )}
          {results.map((item, i) => {
            const showHeader = item.categoryLabel !== lastCategory;
            lastCategory = item.categoryLabel;

            return (
              <div key={item.id}>
                {showHeader && (
                  <div className="px-4 pt-2 pb-1 text-[11px] font-medium text-zinc-400 uppercase tracking-wider">
                    {item.categoryLabel}
                  </div>
                )}
                <button
                  type="button"
                  data-selected={i === selectedIndex}
                  className={`w-full text-left px-4 py-2 flex items-center gap-3 cursor-pointer transition-colors text-sm
                    ${i === selectedIndex ? 'bg-zinc-200 text-zinc-900' : 'text-zinc-600 hover:bg-zinc-50 dark:hover:bg-zinc-200'}`}
                  onClick={() => item.action()}
                  onMouseEnter={() => setSelectedIndex(i)}
                >
                  <span className="flex-shrink-0 w-5 h-5 flex items-center justify-center text-zinc-400">
                    {item.icon === 'chat' && <ChatIcon />}
                    {item.icon === 'action' && <ActionIcon />}
                    {item.icon === 'slash' && (
                      <span className="font-mono text-emerald-500 text-xs font-bold">/</span>
                    )}
                  </span>

                  <div className="flex-1 min-w-0 truncate">
                    <span className={item.icon === 'slash' ? 'font-mono text-emerald-600' : ''}>
                      {highlightMatch(item.label, query)}
                    </span>
                    {item.subtitle && (
                      <span className="ml-2 text-zinc-400 text-xs">{item.subtitle}</span>
                    )}
                  </div>

                  {item.shortcut && (
                    <div className="flex items-center gap-0.5 flex-shrink-0">
                      {item.shortcut.map((k) => (
                        <kbd
                          key={k}
                          className="px-1.5 py-0.5 text-[10px] font-mono text-zinc-400 bg-zinc-200 border border-zinc-300 rounded"
                        >
                          {k}
                        </kbd>
                      ))}
                    </div>
                  )}
                </button>
              </div>
            );
          })}
        </div>
      </div>
    </div>
  );
}
