import { useState, useMemo } from 'react';
import { ChevronRight } from 'lucide-react';

interface DiffViewProps {
  filePath: string;
  mode: 'edit' | 'write';
  oldString?: string;
  newString?: string;
  content?: string;
}

interface DiffLine {
  type: 'add' | 'remove';
  text: string;
}

function buildDiffLines(mode: 'edit' | 'write', oldString?: string, newString?: string, content?: string): DiffLine[] {
  if (mode === 'write') {
    return (content || '').split('\n').map(line => ({ type: 'add', text: line }));
  }

  const oldLines = (oldString || '').split('\n');
  const newLines = (newString || '').split('\n');

  const lines: DiffLine[] = [];
  for (const line of oldLines) {
    lines.push({ type: 'remove', text: line });
  }
  for (const line of newLines) {
    lines.push({ type: 'add', text: line });
  }
  return lines;
}

function countChanges(lines: DiffLine[]): { added: number; removed: number } {
  let added = 0;
  let removed = 0;
  for (const line of lines) {
    if (line.type === 'add') added++;
    if (line.type === 'remove') removed++;
  }
  return { added, removed };
}

export default function DiffView({ filePath, mode, oldString, newString, content }: DiffViewProps) {
  const diffLines = useMemo(
    () => buildDiffLines(mode, oldString, newString, content),
    [mode, oldString, newString, content]
  );

  const { added, removed } = useMemo(() => countChanges(diffLines), [diffLines]);
  const defaultExpanded = diffLines.length <= 20;
  const [expanded, setExpanded] = useState(defaultExpanded);

  const fileName = filePath.split('/').pop() || filePath;

  return (
    <div className="border-t border-zinc-200">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full flex items-center gap-2 px-3 py-1.5 text-left hover:bg-zinc-100 transition-colors cursor-pointer"
      >
        <ChevronRight
          size={12}
          className={`text-zinc-400 transition-transform duration-200 shrink-0 ${expanded ? 'rotate-90' : ''}`}
        />
        <span className="font-mono text-zinc-600 truncate">{fileName}</span>
        <span className="flex items-center gap-1.5 ml-auto shrink-0">
          {added > 0 && <span className="text-emerald-600">+{added}</span>}
          {removed > 0 && <span className="text-red-500">&minus;{removed}</span>}
        </span>
      </button>

      {expanded && (
        <div className="overflow-x-auto bg-zinc-50 max-h-80 overflow-y-auto">
          <table className="w-full font-mono text-xs leading-5">
            <tbody>
              {diffLines.map((line, i) => (
                <tr
                  key={i}
                  className={
                    line.type === 'add'
                      ? 'bg-emerald-50'
                      : 'bg-red-50'
                  }
                >
                  <td className="select-none w-5 text-right pr-2 pl-3 text-zinc-400 align-top">
                    {line.type === 'add' ? '+' : '\u2212'}
                  </td>
                  <td
                    className={`pr-3 whitespace-pre-wrap break-all ${
                      line.type === 'add'
                        ? 'text-emerald-800'
                        : 'text-red-800'
                    }`}
                  >
                    {line.text || '\u00A0'}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
