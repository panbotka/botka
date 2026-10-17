import { useState } from 'react';
import { ChevronDown, Loader2, Wrench } from 'lucide-react';
import ToolCallPanel from './ToolCallPanel';
import { czechToolCount, toolSummaryParts } from '../utils/toolSummary';

/** The common shape of a live (SSE) and a persisted tool call. */
export interface ToolActivityCall {
  /** Stable id for live calls; persisted calls fall back to their index. */
  id?: string;
  name: string;
  input: Record<string, unknown>;
  result?: string;
  isError?: boolean;
}

interface Props {
  toolCalls: ToolActivityCall[];
  /** True while the reply is still streaming; false for a saved message. */
  isStreaming?: boolean;
}

/**
 * One compact row summarising a turn's tool calls, shared by the streaming
 * bubble and the saved message so the row keeps its place and height when the
 * reply completes. While streaming it names the latest still-running tool
 * (with a spinner) or, when the model is writing text, the last tool muted.
 * Tapping toggles the full list of individually collapsed ToolCallPanels.
 */
export default function ToolActivity({ toolCalls, isStreaming }: Props) {
  const [expanded, setExpanded] = useState(false);
  if (toolCalls.length === 0) return null;

  let running: ToolActivityCall | undefined;
  if (isStreaming) {
    for (let i = toolCalls.length - 1; i >= 0; i--) {
      if (toolCalls[i]!.result == null) {
        running = toolCalls[i];
        break;
      }
    }
  }
  const shown = running ?? toolCalls[toolCalls.length - 1]!;
  const { label, detail } = toolSummaryParts(shown.name, shown.input);

  return (
    <div className="mb-2 min-w-0 max-w-full" data-testid="tool-activity">
      <button
        type="button"
        onClick={() => setExpanded((e) => !e)}
        aria-expanded={expanded}
        className="flex items-center gap-1.5 h-6 max-w-full min-w-0 text-xs text-zinc-400 hover:text-zinc-600 transition-colors cursor-pointer"
      >
        {running ? (
          <Loader2 size={12} className="shrink-0 animate-spin text-blue-500" aria-label="Probíhá" />
        ) : (
          <Wrench size={12} className="shrink-0" />
        )}
        {isStreaming ? (
          <>
            <span className={`shrink-0 font-medium ${running ? 'text-zinc-600' : ''}`}>{label}</span>
            {detail && (
              <span className={`truncate min-w-0 ${running ? 'text-zinc-500' : ''}`}>· {detail}</span>
            )}
            <span className="shrink-0 whitespace-nowrap">· {czechToolCount(toolCalls.length)}</span>
          </>
        ) : (
          <span className="truncate min-w-0">
            {toolCalls.length} tool {toolCalls.length === 1 ? 'call' : 'calls'}
          </span>
        )}
        <ChevronDown
          size={12}
          className={`shrink-0 transition-transform duration-200 ${expanded ? 'rotate-180' : ''}`}
        />
      </button>
      <div
        className={`grid transition-[grid-template-rows] duration-200 ease-in-out ${
          expanded ? 'grid-rows-[1fr]' : 'grid-rows-[0fr]'
        }`}
      >
        <div className="overflow-hidden">
          <div className="mt-1">
            {toolCalls.map((tc, i) => (
              <ToolCallPanel
                key={tc.id ?? i}
                name={tc.name}
                input={tc.input}
                result={tc.result}
                isError={tc.isError}
                isStreaming={isStreaming && tc.result == null}
              />
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
