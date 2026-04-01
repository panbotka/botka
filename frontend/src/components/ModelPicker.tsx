import { useState, useEffect, useRef, useMemo } from 'react';
import { api } from '../api/client';

interface Props {
  value: string;
  onChange: (model: string) => void;
}

export default function ModelPicker({ value, onChange }: Props) {
  const [models, setModels] = useState<string[] | null>(null);
  const [failed, setFailed] = useState(false);
  const [open, setOpen] = useState(false);
  const [filter, setFilter] = useState('');
  const containerRef = useRef<HTMLDivElement>(null);
  const filterRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    api.getModels()
      .then((res) => setModels(res.models))
      .catch(() => setFailed(true));
  }, []);

  useEffect(() => {
    if (!open) return;
    const handle = (e: MouseEvent) => {
      if (containerRef.current && !containerRef.current.contains(e.target as Node)) {
        setOpen(false);
        setFilter('');
      }
    };
    document.addEventListener('mousedown', handle);
    return () => document.removeEventListener('mousedown', handle);
  }, [open]);

  useEffect(() => {
    if (open && filterRef.current) {
      filterRef.current.focus();
    }
  }, [open]);

  const searchable = (models?.length ?? 0) > 5;

  const filtered = useMemo(() => {
    if (!models) return [];
    if (!filter) return models;
    const q = filter.toLowerCase();
    return models.filter((m) => m.toLowerCase().includes(q));
  }, [models, filter]);

  if (failed || (models !== null && models.length === 0)) {
    return (
      <input
        type="text"
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder="e.g. claude-sonnet-4-20250514"
        className="w-full bg-zinc-50 border border-zinc-300 rounded-xl
                   px-3 py-2 text-sm text-zinc-900 placeholder-zinc-400
                   outline-none focus:border-zinc-500 focus:ring-1 focus:ring-zinc-500 transition-colors"
      />
    );
  }

  if (models === null) {
    return (
      <div className="w-full bg-zinc-50 border border-zinc-200 rounded-xl
                      px-3 py-2 text-sm text-zinc-400">
        Loading models...
      </div>
    );
  }

  const displayValue = value || 'Default (gateway decides)';

  return (
    <div className="relative" ref={containerRef}>
      <button
        type="button"
        onClick={() => { setOpen(!open); setFilter(''); }}
        className="w-full flex items-center justify-between bg-zinc-50 border border-zinc-300
                   rounded-xl px-3 py-2 text-sm text-left transition-colors cursor-pointer
                   hover:border-zinc-400 focus:border-zinc-500 outline-none"
      >
        <span className={value ? 'text-zinc-900' : 'text-zinc-400'}>
          {displayValue}
        </span>
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor"
             className={`w-4 h-4 text-zinc-400 transition-transform ${open ? 'rotate-180' : ''}`}>
          <path fillRule="evenodd" d="M4.22 6.22a.75.75 0 011.06 0L8 8.94l2.72-2.72a.75.75 0 111.06 1.06l-3.25 3.25a.75.75 0 01-1.06 0L4.22 7.28a.75.75 0 010-1.06z" clipRule="evenodd" />
        </svg>
      </button>

      {open && (
        <div className="absolute z-50 mt-1 w-full bg-zinc-100 border border-zinc-200
                        rounded-xl shadow-xl shadow-black/10 py-1 overflow-hidden">
          {searchable && (
            <div className="px-2 pb-1">
              <input
                ref={filterRef}
                type="text"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Escape') { setOpen(false); setFilter(''); }
                }}
                placeholder="Filter models..."
                className="w-full bg-zinc-50 border border-zinc-200 rounded-lg
                           px-2.5 py-1.5 text-sm text-zinc-900 placeholder-zinc-400
                           outline-none focus:border-zinc-400 transition-colors"
              />
            </div>
          )}
          <div className="max-h-60 overflow-y-auto">
            <button
              onClick={() => { onChange(''); setOpen(false); setFilter(''); }}
              className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer
                         ${!value ? 'text-emerald-600 bg-emerald-50' : 'text-zinc-500 hover:bg-zinc-50 dark:hover:bg-zinc-200 hover:text-zinc-900'}`}
            >
              Default (gateway decides)
            </button>
            {filtered.map((m) => (
              <button
                key={m}
                onClick={() => { onChange(m); setOpen(false); setFilter(''); }}
                className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer
                           ${value === m ? 'text-emerald-600 bg-emerald-50' : 'text-zinc-700 hover:bg-zinc-50 dark:hover:bg-zinc-200'}`}
              >
                {m}
              </button>
            ))}
            {searchable && filtered.length === 0 && (
              <div className="px-3 py-2 text-sm text-zinc-400">No matches</div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
