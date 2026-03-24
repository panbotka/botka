import { useState } from 'react';
import MarkdownContent from './MarkdownContent';

interface Props {
  content: string;
  durationMs?: number;
  isStreaming?: boolean;
}

function formatDuration(ms: number): string {
  if (ms < 1000) return '<1s';
  const seconds = Math.round(ms / 1000);
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remaining = seconds % 60;
  return remaining > 0 ? `${minutes}m ${remaining}s` : `${minutes}m`;
}

export default function ThinkingSection({ content, durationMs, isStreaming }: Props) {
  const [expanded, setExpanded] = useState(false);

  const label = isStreaming && durationMs == null
    ? 'Thinking...'
    : durationMs != null
      ? `Thought for ${formatDuration(durationMs)}`
      : 'Thought';

  return (
    <div className="mb-3">
      <button
        onClick={() => setExpanded(!expanded)}
        aria-expanded={expanded}
        className="flex items-center gap-1.5 text-xs text-zinc-500 hover:text-zinc-700 transition-colors cursor-pointer"
      >
        <svg
          className={`w-3 h-3 transition-transform duration-200 ${expanded ? 'rotate-90' : ''}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
        <span>{label}</span>
        {isStreaming && durationMs == null && (
          <span className="inline-block w-1 h-3 bg-zinc-400 animate-pulse rounded-sm" />
        )}
      </button>
      <div
        className={`grid transition-[grid-template-rows] duration-200 ease-in-out ${
          expanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'
        }`}
      >
        <div className="overflow-hidden">
          <div className="mt-2 pl-4 border-l-2 border-zinc-200 text-xs text-zinc-500 leading-relaxed">
            <MarkdownContent content={content} />
          </div>
        </div>
      </div>
    </div>
  );
}
