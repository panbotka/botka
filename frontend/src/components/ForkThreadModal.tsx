import { useEffect, useRef, useState } from 'react';
import { GitFork, X } from 'lucide-react';

interface Props {
  sourceTitle: string;
  forkPointPreview: string;
  onConfirm: (title: string) => Promise<void> | void;
  onCancel: () => void;
}

// ForkThreadModal asks the user to confirm forking and accept or edit the
// proposed title before the new thread is created.
export default function ForkThreadModal({ sourceTitle, forkPointPreview, onConfirm, onCancel }: Props) {
  const [title, setTitle] = useState(`${sourceTitle} (fork)`);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape' && !submitting) onCancel();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [onCancel, submitting]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (submitting) return;
    setSubmitting(true);
    setError(null);
    try {
      await onConfirm(title.trim() || `${sourceTitle} (fork)`);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to fork thread');
      setSubmitting(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-zinc-950/40 px-4" onClick={onCancel}>
      <form
        onSubmit={handleSubmit}
        onClick={(e) => e.stopPropagation()}
        className="w-full max-w-md rounded-xl bg-white shadow-xl ring-1 ring-zinc-200"
      >
        <div className="flex items-center gap-2 border-b border-zinc-100 px-4 py-3">
          <GitFork className="w-4 h-4 text-zinc-500" />
          <h2 className="text-sm font-medium text-zinc-900 flex-1">Fork thread from here</h2>
          <button
            type="button"
            onClick={onCancel}
            className="text-zinc-400 hover:text-zinc-700 transition-colors"
            aria-label="Cancel"
          >
            <X className="w-4 h-4" />
          </button>
        </div>

        <div className="px-4 py-3 space-y-3 text-sm text-zinc-700">
          <div className="text-zinc-600">
            A new conversation will be created with all messages up to and including this fork point.
            The original thread is not affected.
          </div>
          <div className="rounded-md bg-zinc-50 ring-1 ring-zinc-200 p-3 text-zinc-600">
            <div className="text-[11px] uppercase tracking-wide text-zinc-400 mb-1">Fork point</div>
            <div className="line-clamp-2 text-zinc-700">{forkPointPreview || '(empty message)'}</div>
          </div>
          <label className="block">
            <span className="text-[11px] uppercase tracking-wide text-zinc-400">New title</span>
            <input
              ref={inputRef}
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              maxLength={500}
              className="mt-1 w-full rounded-md border border-zinc-200 px-2.5 py-1.5 text-sm focus:outline-none focus:ring-2 focus:ring-amber-300 focus:border-amber-300"
              placeholder={`${sourceTitle} (fork)`}
            />
          </label>
          {error && <div className="text-xs text-red-600">{error}</div>}
        </div>

        <div className="flex items-center justify-end gap-2 border-t border-zinc-100 px-4 py-3">
          <button
            type="button"
            onClick={onCancel}
            disabled={submitting}
            className="rounded-md px-3 py-1.5 text-sm text-zinc-600 hover:bg-zinc-100 disabled:opacity-50"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={submitting}
            className="rounded-md bg-amber-500 px-3 py-1.5 text-sm font-medium text-white hover:bg-amber-400 active:bg-amber-600 disabled:opacity-60 inline-flex items-center gap-1.5"
          >
            <GitFork className="w-3.5 h-3.5" />
            {submitting ? 'Forking…' : 'Fork thread'}
          </button>
        </div>
      </form>
    </div>
  );
}
