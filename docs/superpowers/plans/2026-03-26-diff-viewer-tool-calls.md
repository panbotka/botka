# Diff Viewer for File Edit Tool Calls

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a readable diff view for Edit and Write tool calls instead of raw JSON input.

**Architecture:** Create a `DiffView` component that renders unified-diff-style output from Edit's old_string/new_string and Write's content. Integrate it into the existing `ToolCallPanel` component, replacing the raw JSON display for these two tools. The component handles line splitting, diff coloring, line counts for the summary, and auto-collapse for large diffs.

**Tech Stack:** React 19, TypeScript, Tailwind CSS 4, Lucide icons

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `frontend/src/components/DiffView.tsx` | Create | Renders unified diff view with red/green lines, line numbers, file path header |
| `frontend/src/components/ToolCallPanel.tsx` | Modify | Use DiffView for Edit/Write tools instead of raw JSON in expanded content |

---

### Task 1: Create the DiffView component

**Files:**
- Create: `frontend/src/components/DiffView.tsx`

- [ ] **Step 1: Create DiffView component**

Create the component that renders a diff view. It accepts two modes: "edit" (old_string → new_string) and "write" (all additions).

```tsx
// frontend/src/components/DiffView.tsx
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
  type: 'add' | 'remove' | 'context';
  text: string;
}

function buildDiffLines(mode: 'edit' | 'write', oldString?: string, newString?: string, content?: string): DiffLine[] {
  if (mode === 'write') {
    return (content || '').split('\n').map(line => ({ type: 'add', text: line }));
  }

  // Edit mode: show old lines as removals, new lines as additions
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
          {removed > 0 && <span className="text-red-500">−{removed}</span>}
        </span>
      </button>

      {expanded && (
        <div className="overflow-x-auto bg-zinc-50">
          <table className="w-full font-mono text-xs leading-5">
            <tbody>
              {diffLines.map((line, i) => (
                <tr
                  key={i}
                  className={
                    line.type === 'add'
                      ? 'bg-emerald-50 dark:bg-emerald-950/30'
                      : line.type === 'remove'
                        ? 'bg-red-50 dark:bg-red-950/30'
                        : ''
                  }
                >
                  <td className="select-none w-5 text-right pr-2 pl-3 text-zinc-400 align-top">
                    {line.type === 'add' ? '+' : line.type === 'remove' ? '−' : ' '}
                  </td>
                  <td
                    className={`pr-3 whitespace-pre-wrap break-all ${
                      line.type === 'add'
                        ? 'text-emerald-800 dark:text-emerald-300'
                        : line.type === 'remove'
                          ? 'text-red-800 dark:text-red-300'
                          : 'text-zinc-700'
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

export { countChanges, buildDiffLines };
export type { DiffLine };
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors related to DiffView.tsx

- [ ] **Step 3: Commit**

```bash
git add frontend/src/components/DiffView.tsx
git commit -m "feat: add DiffView component for unified diff rendering"
```

---

### Task 2: Integrate DiffView into ToolCallPanel

**Files:**
- Modify: `frontend/src/components/ToolCallPanel.tsx`

- [ ] **Step 1: Import DiffView and update the expanded content**

In `ToolCallPanel.tsx`, import DiffView at the top:

```tsx
import DiffView from './DiffView';
```

- [ ] **Step 2: Replace the expanded Input section for Edit/Write tools**

Replace the expanded content section (the `<div className="overflow-hidden">` block, lines 107-131) to conditionally render DiffView for Edit/Write tools, and keep the existing raw display for all other tools.

The logic:
- If `name === 'Edit'`: render `<DiffView mode="edit" filePath={input.file_path} oldString={input.old_string} newString={input.new_string} />`
- If `name === 'Write'`: render `<DiffView mode="write" filePath={input.file_path} content={input.content} />`
- Otherwise: render existing raw JSON input/output display

The result/output section should still appear below the diff for Edit/Write tools.

Updated expanded content block:

```tsx
<div className="overflow-hidden">
  <div className="border-t border-zinc-200">
    {(name === 'Edit' || name === 'Write') && input.file_path ? (
      <>
        <DiffView
          filePath={String(input.file_path)}
          mode={name === 'Edit' ? 'edit' : 'write'}
          oldString={name === 'Edit' ? String(input.old_string || '') : undefined}
          newString={name === 'Edit' ? String(input.new_string || '') : undefined}
          content={name === 'Write' ? String(input.content || '') : undefined}
        />
      </>
    ) : (
      <div className="px-3 py-2 bg-zinc-100/50">
        <div className="text-zinc-400 mb-1">Input</div>
        <pre className="text-zinc-700 whitespace-pre-wrap break-all font-mono leading-relaxed max-h-40 overflow-y-auto">
          {name === 'Bash' ? String(input.command || '') : JSON.stringify(input, null, 2)}
        </pre>
      </div>
    )}

    {result != null && (
      <div className="px-3 py-2 border-t border-zinc-200">
        <div className={`mb-1 ${isError ? 'text-red-500' : 'text-zinc-400'}`}>
          {isError ? 'Error' : 'Output'}
        </div>
        <pre className={`whitespace-pre-wrap break-all font-mono leading-relaxed max-h-60 overflow-y-auto ${
          isError ? 'text-red-600' : 'text-zinc-700'
        }`}>
          {(() => {
            const text = result.length > 2000 ? result.slice(0, 2000) + '\n... (truncated)' : result;
            return name === 'mcp__botka__create_task' ? linkifyTasksReact(text) : text;
          })()}
        </pre>
      </div>
    )}
  </div>
</div>
```

- [ ] **Step 3: Update the label for Edit to include line counts**

In `getToolLabel`, enhance the Edit case to show +/- counts when old_string and new_string are present:

```tsx
case 'Edit': {
  const fileName = String(input.file_path || '').split('/').pop() || 'file';
  const oldLines = String(input.old_string || '').split('\n').length;
  const newLines = String(input.new_string || '').split('\n').length;
  if (input.old_string || input.new_string) {
    return `${fileName} (+${newLines} −${oldLines})`;
  }
  return fileName;
}
case 'Write': {
  const fileName = String(input.file_path || '').split('/').pop() || 'file';
  const lines = String(input.content || '').split('\n').length;
  return `${fileName} (+${lines})`;
}
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /home/pi/projects/botka/frontend && npx tsc --noEmit 2>&1 | head -20`
Expected: No errors

- [ ] **Step 5: Build the frontend**

Run: `cd /home/pi/projects/botka/frontend && npx vite build 2>&1 | tail -10`
Expected: Build succeeds

- [ ] **Step 6: Run backend tests**

Run: `cd /home/pi/projects/botka && make test 2>&1 | tail -20`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
git add frontend/src/components/ToolCallPanel.tsx
git commit -m "feat: integrate diff viewer for Edit/Write tool calls in chat"
```
