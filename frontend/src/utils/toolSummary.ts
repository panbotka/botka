/**
 * One-line summaries of a tool call, used by the compact tool activity row in
 * chat (`ToolActivity`). Kept pure so the formatting is unit-testable.
 */

/** Hard cap on the detail length; the row truncates visually with CSS anyway. */
const MAX_DETAIL = 160;

export interface ToolSummaryParts {
  /** Display name of the tool (`Bash`, or `server: tool` for MCP tools). */
  label: string;
  /** Short description of the input; empty when there is nothing useful to show. */
  detail: string;
}

function str(v: unknown): string {
  return typeof v === 'string' ? v : '';
}

/** First non-empty line with whitespace collapsed, capped at MAX_DETAIL chars. */
function oneLine(s: string): string {
  const line = s.split('\n').map((l) => l.trim()).find((l) => l !== '') ?? '';
  const collapsed = line.replace(/\s+/g, ' ');
  return collapsed.length > MAX_DETAIL ? collapsed.slice(0, MAX_DETAIL - 1) + '…' : collapsed;
}

function basename(path: string): string {
  return path.replace(/\/+$/, '').split('/').pop() ?? '';
}

function hostname(raw: string): string {
  try {
    return new URL(raw).hostname || raw;
  } catch {
    return raw;
  }
}

/**
 * Splits a tool call into a display label and a short input detail.
 * MCP tools (`mcp__server__tool`) become `server: tool` with no detail; unknown
 * tools show their name only.
 */
export function toolSummaryParts(name: string, input: Record<string, unknown> | null | undefined): ToolSummaryParts {
  const inp = input ?? {};
  if (name.startsWith('mcp__')) {
    const rest = name.slice('mcp__'.length);
    const sep = rest.indexOf('__');
    if (sep > 0 && sep < rest.length - 2) {
      return { label: `${rest.slice(0, sep)}: ${rest.slice(sep + 2)}`, detail: '' };
    }
    return { label: rest || name, detail: '' };
  }

  let detail = '';
  switch (name) {
    case 'Bash':
      detail = str(inp.command) || str(inp.description);
      break;
    case 'Read':
    case 'Write':
    case 'Edit':
    case 'MultiEdit':
      detail = basename(str(inp.file_path));
      break;
    case 'NotebookEdit':
      detail = basename(str(inp.notebook_path) || str(inp.file_path));
      break;
    case 'Grep':
    case 'Glob':
      detail = str(inp.pattern);
      break;
    case 'WebFetch':
      detail = hostname(str(inp.url));
      break;
    case 'WebSearch':
      detail = str(inp.query);
      break;
    case 'Task':
    case 'Agent':
      detail = str(inp.description) || str(inp.subagent_type) || str(inp.prompt);
      break;
    case 'Skill':
      detail = str(inp.skill) || str(inp.command);
      break;
    default:
      detail = '';
  }
  return { label: name, detail: oneLine(detail) };
}

/** `Bash · make test`, `Edit · TaskList.tsx`, `botka: create_task`, or just the name. */
export function summarizeToolCall(name: string, input: Record<string, unknown> | null | undefined): string {
  const { label, detail } = toolSummaryParts(name, input);
  return detail ? `${label} · ${detail}` : label;
}

/** Czech count of tools: `1 nástroj`, `3 nástroje`, `7 nástrojů`. */
export function czechToolCount(n: number): string {
  if (n === 1) return '1 nástroj';
  if (n >= 2 && n <= 4) return `${n} nástroje`;
  return `${n} nástrojů`;
}
