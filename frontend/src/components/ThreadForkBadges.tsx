import { useEffect, useRef, useState } from 'react';
import { GitFork, CornerLeftUp } from 'lucide-react';
import type { Thread } from '../types';
import { api } from '../api/client';

interface Props {
  thread: Thread;
  onSelectThread: (id: number) => void;
}

// ThreadForkBadges renders two small affordances next to the thread title:
// 1. "↳ forked from <parent>" — a link to the parent thread (if this thread is a fork).
// 2. "Forks (N)" — a popover listing threads forked from this one (if any exist).
export default function ThreadForkBadges({ thread, onSelectThread }: Props) {
  const [parentTitle, setParentTitle] = useState<string | null>(null);
  const [forks, setForks] = useState<Thread[] | null>(null);
  const [popoverOpen, setPopoverOpen] = useState(false);
  const popoverRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let cancelled = false;
    if (thread.parent_thread_id) {
      api.getThread(thread.parent_thread_id)
        .then((d) => { if (!cancelled) setParentTitle(d.thread.title); })
        .catch(() => { if (!cancelled) setParentTitle(null); });
    } else {
      setParentTitle(null);
    }
    return () => { cancelled = true; };
  }, [thread.parent_thread_id]);

  useEffect(() => {
    let cancelled = false;
    api.fetchThreadForks(thread.id)
      .then((list) => { if (!cancelled) setForks(list); })
      .catch(() => { if (!cancelled) setForks([]); });
    return () => { cancelled = true; };
  }, [thread.id]);

  useEffect(() => {
    if (!popoverOpen) return;
    const handleClick = (e: MouseEvent) => {
      if (popoverRef.current && !popoverRef.current.contains(e.target as Node)) {
        setPopoverOpen(false);
      }
    };
    document.addEventListener('mousedown', handleClick);
    return () => document.removeEventListener('mousedown', handleClick);
  }, [popoverOpen]);

  const hasParent = thread.parent_thread_id && parentTitle;
  const forkCount = forks?.length ?? 0;

  if (!hasParent && forkCount === 0) return null;

  return (
    <div className="flex items-center gap-1.5 flex-shrink-0">
      {hasParent && (
        <button
          type="button"
          onClick={() => onSelectThread(thread.parent_thread_id!)}
          className="inline-flex items-center gap-1 max-w-[180px] text-[11px] text-zinc-500 hover:text-zinc-800 hover:bg-zinc-100 px-2 py-0.5 rounded-md transition-colors"
          title={`Forked from: ${parentTitle}`}
        >
          <CornerLeftUp className="w-3 h-3 flex-shrink-0" />
          <span className="truncate">{parentTitle}</span>
        </button>
      )}
      {forkCount > 0 && (
        <div ref={popoverRef} className="relative">
          <button
            type="button"
            onClick={() => setPopoverOpen((v) => !v)}
            className="inline-flex items-center gap-1 text-[11px] text-zinc-500 hover:text-zinc-800 hover:bg-zinc-100 px-2 py-0.5 rounded-md transition-colors"
            title={`${forkCount} fork${forkCount === 1 ? '' : 's'}`}
          >
            <GitFork className="w-3 h-3 flex-shrink-0" />
            <span>Forks ({forkCount})</span>
          </button>
          {popoverOpen && forks && (
            <div className="absolute right-0 top-full mt-1 z-30 min-w-[260px] max-w-[320px] max-h-[320px] overflow-y-auto rounded-lg bg-white shadow-lg ring-1 ring-zinc-200 py-1">
              <div className="px-3 py-1.5 text-[11px] uppercase tracking-wide text-zinc-400 border-b border-zinc-100">
                Forks of this thread
              </div>
              {forks.map((f) => (
                <button
                  key={f.id}
                  type="button"
                  onClick={() => { onSelectThread(f.id); setPopoverOpen(false); }}
                  className="block w-full text-left px-3 py-2 text-sm text-zinc-700 hover:bg-zinc-50 truncate"
                  title={f.title}
                >
                  {f.title}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
