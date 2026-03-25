import type { ProcessInfo } from '../hooks/useProcesses';

function formatDuration(sec: number): string {
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return `${m}m ${s}s`;
}

interface Props {
  processes: ProcessInfo[];
  onKill: (threadId: number) => void;
}

export default function ProcessBar({ processes, onKill }: Props) {
  const hasProcesses = processes.length > 0;

  return (
    <div className="flex items-center gap-2 px-4 py-1 bg-zinc-100 border-b border-zinc-200 text-[11px] text-zinc-500 overflow-x-auto">
      {hasProcesses ? (
        <>
          <span className="text-emerald-500 animate-pulse shrink-0">●</span>
          <span className="text-zinc-400 shrink-0">Active:</span>
        </>
      ) : (
        <span className="text-zinc-400 shrink-0">No active sessions</span>
      )}
      {processes.map((p) => (
        <span
          key={p.thread_id}
          className="inline-flex items-center gap-1.5 bg-zinc-200 rounded-md px-2 py-0.5 shrink-0"
        >
          <span className="text-zinc-700 truncate max-w-[200px]">
            {p.thread_title || `Thread ${p.thread_id}`}
          </span>
          <span className="text-zinc-400">{formatDuration(p.duration_sec)}</span>
          <button
            onClick={() => onKill(p.thread_id)}
            className="text-zinc-400 hover:text-red-500 transition-colors ml-0.5 cursor-pointer"
            title="Kill process"
          >
            <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
              <path d="M5.28 4.22a.75.75 0 0 0-1.06 1.06L6.94 8l-2.72 2.72a.75.75 0 1 0 1.06 1.06L8 9.06l2.72 2.72a.75.75 0 1 0 1.06-1.06L9.06 8l2.72-2.72a.75.75 0 0 0-1.06-1.06L8 6.94 5.28 4.22Z" />
            </svg>
          </button>
        </span>
      ))}
    </div>
  );
}
