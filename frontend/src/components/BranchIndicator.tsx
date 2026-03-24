import type { ForkPoint } from '../types';

interface Props {
  forkPoint: ForkPoint;
  onSwitch: (childId: number) => void;
}

export default function BranchIndicator({ forkPoint, onSwitch }: Props) {
  const { children, active_index } = forkPoint;
  const total = children.length;
  const current = active_index + 1;

  const handlePrev = () => {
    if (active_index > 0) {
      onSwitch(children[active_index - 1]!.id);
    }
  };

  const handleNext = () => {
    if (active_index < total - 1) {
      onSwitch(children[active_index + 1]!.id);
    }
  };

  return (
    <div className="flex items-center gap-1 px-2 py-1 rounded-lg bg-zinc-100 border border-zinc-200 text-xs text-zinc-500 select-none">
      <button
        onClick={handlePrev}
        disabled={active_index === 0}
        className="p-0.5 rounded hover:bg-zinc-200 hover:text-zinc-900 disabled:opacity-30 disabled:cursor-default transition-colors cursor-pointer"
        title="Previous branch"
        aria-label="Previous branch"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
          <path fillRule="evenodd" d="M9.78 4.22a.75.75 0 0 1 0 1.06L7.06 8l2.72 2.72a.75.75 0 1 1-1.06 1.06L5.22 8.53a.75.75 0 0 1 0-1.06l3.5-3.5a.75.75 0 0 1 1.06 0Z" clipRule="evenodd" />
        </svg>
      </button>
      <span className="tabular-nums font-medium min-w-[2.5rem] text-center">
        {current}/{total}
      </span>
      <button
        onClick={handleNext}
        disabled={active_index === total - 1}
        className="p-0.5 rounded hover:bg-zinc-200 hover:text-zinc-900 disabled:opacity-30 disabled:cursor-default transition-colors cursor-pointer"
        title="Next branch"
        aria-label="Next branch"
      >
        <svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="currentColor" className="w-3 h-3">
          <path fillRule="evenodd" d="M6.22 4.22a.75.75 0 0 1 1.06 0l3.5 3.5a.75.75 0 0 1 0 1.06l-3.5 3.5a.75.75 0 0 1-1.06-1.06L8.94 8 6.22 5.28a.75.75 0 0 1 0-1.06Z" clipRule="evenodd" />
        </svg>
      </button>
    </div>
  );
}
