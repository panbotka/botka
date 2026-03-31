import type { Message, Thread } from '../types';
import { formatDateTime } from './dateFormat';

export type ExportFormat = 'md' | 'json';

function formatDate(dateStr: string): string {
  return formatDateTime(dateStr);
}

export function exportAsMarkdown(messages: Message[], thread?: Thread): string {
  const lines: string[] = [];
  if (thread?.title) lines.push(`# ${thread.title}\n`);
  for (const msg of messages) {
    const label = msg.role === 'user' ? '**User:**' : '**Assistant:**';
    lines.push(`${label}\n`);
    lines.push(msg.content);
    lines.push('');
  }
  lines.push('---\n');
  const meta: string[] = [];
  if (thread?.model) meta.push(`Model: ${thread.model}`);
  if (thread?.created_at) meta.push(`Date: ${formatDate(thread.created_at)}`);
  meta.push(`Messages: ${messages.length}`);
  lines.push(`*${meta.join(' · ')}*`);
  lines.push('');
  return lines.join('\n');
}

export function exportAsJSON(messages: Message[], thread?: Thread): string {
  const data = {
    thread: thread
      ? {
          title: thread.title,
          model: thread.model,
          created_at: thread.created_at,
          updated_at: thread.updated_at,
        }
      : null,
    messages: messages.map((m) => ({
      role: m.role,
      content: m.content,
      created_at: m.created_at,
    })),
  };
  return JSON.stringify(data, null, 2);
}

function sanitizeFilename(name: string): string {
  return name.replace(/[^a-zA-Z0-9_\- ]/g, '').trim() || 'chat';
}

export function downloadExport(
  messages: Message[],
  format: ExportFormat,
  thread?: Thread,
): void {
  const filename = sanitizeFilename(thread?.title || 'chat');
  let content: string;
  let mimeType: string;
  let ext: string;

  if (format === 'json') {
    content = exportAsJSON(messages, thread);
    mimeType = 'application/json';
    ext = 'json';
  } else {
    content = exportAsMarkdown(messages, thread);
    mimeType = 'text/markdown';
    ext = 'md';
  }

  const blob = new Blob([content], { type: mimeType });
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `${filename}.${ext}`;
  a.click();
  URL.revokeObjectURL(url);
}
