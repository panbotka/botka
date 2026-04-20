import { useEffect, useState } from 'react';
import { X } from 'lucide-react';
import { clsx } from 'clsx';
import type { ProcessInfo } from '../hooks/useProcesses';

interface Props {
  processes: ProcessInfo[];
  activeThreadId: number | null;
  onSelect: (threadId: number) => void;
  onKill: (threadId: number) => void;
}

function useOrderedProcesses(processes: ProcessInfo[]): ProcessInfo[] {
  const [order, setOrder] = useState<number[]>(() => processes.map(p => p.thread_id));

  useEffect(() => {
    setOrder(prev => {
      const currentIds = new Set(processes.map(p => p.thread_id));
      const kept = prev.filter(id => currentIds.has(id));
      const keptSet = new Set(kept);
      const appended = processes
        .map(p => p.thread_id)
        .filter(id => !keptSet.has(id));
      if (
        kept.length === prev.length &&
        appended.length === 0
      ) {
        return prev;
      }
      return [...kept, ...appended];
    });
  }, [processes]);

  const byId = new Map(processes.map(p => [p.thread_id, p]));
  return order.map(id => byId.get(id)).filter((p): p is ProcessInfo => !!p);
}

export default function ChatTabsBar({ processes, activeThreadId, onSelect, onKill }: Props) {
  const ordered = useOrderedProcesses(processes);
  if (ordered.length === 0) return null;

  return (
    <div
      role="tablist"
      aria-label="Active chat sessions"
      className="flex items-stretch gap-0.5 px-1 bg-zinc-100 border-b border-zinc-200 overflow-x-auto flex-shrink-0"
    >
      {ordered.map(p => {
        const active = p.thread_id === activeThreadId;
        const label = p.thread_title || `Thread ${p.thread_id}`;
        return (
          <div
            key={p.thread_id}
            role="tab"
            aria-selected={active}
            title={label}
            onClick={() => onSelect(p.thread_id)}
            className={clsx(
              'group relative inline-flex items-center gap-1.5 pl-3 pr-1 py-1 text-[12px] border-t border-l border-r rounded-t-md cursor-pointer shrink-0 max-w-[220px] mt-1 transition-colors',
              active
                ? 'bg-white border-zinc-200 text-zinc-900 -mb-px'
                : 'bg-zinc-50 border-transparent text-zinc-500 hover:bg-white hover:text-zinc-800',
            )}
          >
            <span className="truncate">{label}</span>
            <button
              type="button"
              onClick={(e) => {
                e.stopPropagation();
                onKill(p.thread_id);
              }}
              title="Close session"
              aria-label={`Close session for ${label}`}
              className="shrink-0 rounded p-0.5 text-zinc-400 hover:text-red-500 hover:bg-zinc-100 cursor-pointer transition-colors"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
