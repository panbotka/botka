import { useState } from 'react';

interface Props {
  name: string;
  input: Record<string, unknown>;
  result?: string;
  isError?: boolean;
  isStreaming?: boolean;
}

const TOOL_ICONS: Record<string, string> = {
  Bash: '$',
  Read: '\u{1F4C4}',
  Edit: '\u{270F}',
  Write: '\u{1F4DD}',
  Grep: '\u{1F50D}',
  Glob: '\u{1F4C1}',
  WebFetch: '\u{1F310}',
  WebSearch: '\u{1F50E}',
};

function getToolLabel(name: string, input: Record<string, unknown>): string {
  switch (name) {
    case 'Bash':
      return String(input.command || '').slice(0, 80) || 'command';
    case 'Read':
      return String(input.file_path || '').split('/').pop() || 'file';
    case 'Edit':
      return String(input.file_path || '').split('/').pop() || 'file';
    case 'Write':
      return String(input.file_path || '').split('/').pop() || 'file';
    case 'Grep':
      return String(input.pattern || '');
    case 'Glob':
      return String(input.pattern || '');
    case 'WebFetch':
      return String(input.url || '').slice(0, 60);
    case 'WebSearch':
      return String(input.query || '');
    default:
      return name;
  }
}

export default function ToolCallPanel({ name, input, result, isError, isStreaming }: Props) {
  const [expanded, setExpanded] = useState(false);

  const icon = TOOL_ICONS[name] || '\u{2699}';
  const label = getToolLabel(name, input);

  return (
    <div className="my-2 rounded-lg border border-zinc-200 bg-zinc-50 overflow-hidden text-xs">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-2 text-left hover:bg-zinc-100 transition-colors cursor-pointer"
      >
        <span className="shrink-0 w-5 text-center font-mono text-zinc-500">{icon}</span>
        <span className="font-medium text-zinc-700">{name}</span>
        <span className="text-zinc-400 truncate flex-1">{label}</span>
        {isStreaming && !result && (
          <span className="inline-block w-1.5 h-1.5 bg-blue-400 rounded-full animate-pulse" />
        )}
        {result && !isError && (
          <span className="text-emerald-600 text-[10px]">done</span>
        )}
        {isError && (
          <span className="text-red-500 text-[10px]">error</span>
        )}
        <svg
          className={`w-3 h-3 text-zinc-400 transition-transform duration-200 ${expanded ? 'rotate-90' : ''}`}
          fill="none"
          viewBox="0 0 24 24"
          stroke="currentColor"
          strokeWidth={2}
        >
          <path strokeLinecap="round" strokeLinejoin="round" d="M9 5l7 7-7 7" />
        </svg>
      </button>

      <div
        className={`grid transition-[grid-template-rows] duration-200 ease-in-out ${
          expanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'
        }`}
      >
        <div className="overflow-hidden">
          <div className="border-t border-zinc-200">
            <div className="px-3 py-2 bg-zinc-100/50">
              <div className="text-zinc-400 mb-1">Input</div>
              <pre className="text-zinc-700 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-40 overflow-y-auto">
                {name === 'Bash' ? String(input.command || '') : JSON.stringify(input, null, 2)}
              </pre>
            </div>

            {result != null && (
              <div className="px-3 py-2 border-t border-zinc-200">
                <div className={`mb-1 ${isError ? 'text-red-500' : 'text-zinc-400'}`}>
                  {isError ? 'Error' : 'Output'}
                </div>
                <pre className={`whitespace-pre-wrap break-all font-mono leading-relaxed max-h-60 overflow-y-auto ${
                  isError ? 'text-red-600' : 'text-zinc-700'
                }`}>
                  {result.length > 2000 ? result.slice(0, 2000) + '\n... (truncated)' : result}
                </pre>
              </div>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}
