import { useEffect, useRef } from 'react';

export interface SlashCommand {
  name: string;
  description: string;
}

export const COMMANDS: SlashCommand[] = [
  { name: '/new', description: 'Start a new thread' },
  { name: '/status', description: 'Show current model info' },
  { name: '/model', description: 'Switch the active model' },
  { name: '/export', description: 'Export thread (md or json)' },
  { name: '/search', description: 'Open the search panel' },
  { name: '/clear', description: "Clear this thread's messages" },
  { name: '/compact', description: 'Compact Claude context' },
  { name: '/reset', description: 'Reset Claude session (fresh start)' },
];

interface Props {
  filter: string;
  selectedIndex: number;
  onSelect: (command: string) => void;
}

export function getFilteredCommands(filter: string): SlashCommand[] {
  const lower = filter.toLowerCase();
  return COMMANDS.filter((cmd) => cmd.name.startsWith(lower));
}

export default function SlashCommandMenu({ filter, selectedIndex, onSelect }: Props) {
  const listRef = useRef<HTMLDivElement>(null);
  const filtered = getFilteredCommands(filter);

  useEffect(() => {
    const selected = listRef.current?.children[selectedIndex] as HTMLElement | undefined;
    selected?.scrollIntoView({ block: 'nearest' });
  }, [selectedIndex]);

  if (filtered.length === 0) return null;

  return (
    <div
      ref={listRef}
      className="absolute bottom-full left-0 right-0 mb-2
                 bg-zinc-100 border border-zinc-200 rounded-xl shadow-xl shadow-black/10
                 max-h-60 overflow-y-auto z-10"
    >
      {filtered.map((cmd, i) => (
        <button
          key={cmd.name}
          type="button"
          className={`w-full text-left px-4 py-2.5 flex items-center gap-3 cursor-pointer
                     transition-all duration-150 text-sm first:rounded-t-xl last:rounded-b-xl
                     ${i === selectedIndex
                       ? 'bg-zinc-200 text-zinc-900'
                       : 'text-zinc-600 hover:bg-zinc-50 dark:hover:bg-zinc-200'}`}
          onMouseDown={(e) => {
            e.preventDefault();
            onSelect(cmd.name);
          }}
        >
          <span className="font-mono text-emerald-600 font-medium">{cmd.name}</span>
          <span className="text-zinc-400">{cmd.description}</span>
        </button>
      ))}
    </div>
  );
}
