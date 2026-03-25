import { ReactNode } from 'react';

const TASK_PATTERN = /Created task ([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/gi;

/**
 * Pre-process markdown text to convert "Created task <uuid>" into markdown links.
 */
export function linkifyTasksMarkdown(content: string): string {
  return content.replace(TASK_PATTERN, 'Created task [$1](/tasks/$1)');
}

/**
 * Convert plain text containing "Created task <uuid>" into React nodes with clickable links.
 * Used for non-markdown contexts like tool result panels.
 */
export function linkifyTasksReact(text: string): ReactNode[] {
  const parts: ReactNode[] = [];
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  const regex = new RegExp(TASK_PATTERN.source, TASK_PATTERN.flags);
  while ((match = regex.exec(text)) !== null) {
    if (match.index > lastIndex) {
      parts.push(text.slice(lastIndex, match.index));
    }
    const uuid = match[1];
    parts.push(
      <a
        key={match.index}
        href={`/tasks/${uuid}`}
        target="_blank"
        rel="noopener noreferrer"
        className="text-emerald-600 underline hover:text-emerald-500"
      >
        Created task {uuid}
      </a>
    );
    lastIndex = regex.lastIndex;
  }

  if (lastIndex < text.length) {
    parts.push(text.slice(lastIndex));
  }

  return parts;
}
