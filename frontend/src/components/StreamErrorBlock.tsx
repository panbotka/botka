import { useState } from 'react';
import { AlertCircle, ChevronRight, RotateCcw } from 'lucide-react';

interface Props {
  message: string;
  raw?: string | null;
  onRetry?: () => void;
}

export default function StreamErrorBlock({ message, raw, onRetry }: Props) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="my-2 rounded-lg border border-red-200 bg-red-50 overflow-hidden text-xs">
      <div className="flex items-center gap-2 px-3 py-2">
        <button
          onClick={() => setExpanded(!expanded)}
          className="flex items-center gap-2 flex-1 min-w-0 text-left hover:bg-red-100/50 -mx-1 px-1 rounded transition-colors cursor-pointer"
        >
          <AlertCircle size={16} className="shrink-0 text-red-500" />
          <span className="font-medium text-red-700">Error</span>
          <span className="text-red-600 truncate flex-1">{message}</span>
          {raw && (
            <ChevronRight
              size={12}
              className={`text-red-400 transition-transform duration-200 shrink-0 ${expanded ? 'rotate-90' : ''}`}
            />
          )}
        </button>
        {onRetry && (
          <button
            onClick={onRetry}
            className="flex items-center gap-1 shrink-0 px-2.5 py-1 rounded-lg bg-red-100 hover:bg-red-200 text-red-700 text-xs font-medium transition-colors cursor-pointer"
          >
            <RotateCcw size={11} />
            Retry
          </button>
        )}
      </div>

      {raw && (
        <div
          className={`grid transition-[grid-template-rows] duration-200 ease-in-out ${
            expanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'
          }`}
        >
          <div className="overflow-hidden">
            <div className="border-t border-red-200 px-3 py-2 bg-red-100/30">
              <pre className="text-red-700 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-60 overflow-y-auto">
                {formatRaw(raw)}
              </pre>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

function formatRaw(raw: string): string {
  try {
    return JSON.stringify(JSON.parse(raw), null, 2);
  } catch {
    return raw;
  }
}
