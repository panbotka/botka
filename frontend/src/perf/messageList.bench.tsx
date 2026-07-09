/**
 * Chat message-list render benchmark.
 *
 * Not part of `make check` — vitest's default include only picks up
 * *.test.tsx / *.spec.tsx. Run it explicitly:
 *
 *   npx vitest run --include='src/perf/*.bench.tsx'
 *
 * It reproduces, in jsdom, the two costs that dominate the mobile chat view:
 *
 *   1. Mounting a long thread (N message bubbles, each markdown-parsed).
 *   2. A streaming response: each SSE token bumps ChatView state, which
 *      re-renders the whole `messages.map(...)`.
 *
 * jsdom has no layout/paint, so the numbers are pure scripting cost
 * (react-markdown parse + React reconciliation). That is exactly the axis
 * a mid-thread mobile stall lives on, and it is the axis memoization and a
 * bounded render window move.
 */
import { useState } from 'react';
import { render, act, cleanup } from '@testing-library/react';
import { test, afterEach } from 'vitest';
import MessageBubble from '../components/MessageBubble';
import type { Message } from '../types';

const MESSAGE_COUNT = 200;
const STREAM_TOKENS = 60;

function makeMessages(n: number): Message[] {
  return Array.from({ length: n }, (_, i) => ({
    id: i + 1,
    thread_id: 1,
    role: i % 2 === 0 ? ('user' as const) : ('assistant' as const),
    content:
      i % 2 === 0
        ? `Question number ${i} about the deployment pipeline.`
        : [
            `## Answer ${i}`,
            '',
            'Here is a **bold** point and a [link](https://example.com).',
            '',
            '- first item',
            '- second item',
            '- third item',
            '',
            '| col | val |',
            '| --- | --- |',
            '| a   | 1   |',
            '',
            'Inline `code` plus a paragraph long enough to force a real parse of',
            'several block nodes rather than a trivial single-text-node tree.',
          ].join('\n'),
    created_at: '2026-07-09T10:00:00Z',
  }));
}

const MESSAGES = makeMessages(MESSAGE_COUNT);

// ChatView hands every bubble the same callback identity; mirror that here or
// the benchmark measures a bug it doesn't have.
const noop = () => {};

/** Mirrors ChatView's message list plus the streaming bubble. */
function Harness({ window: windowSize, onReady }: { window: number; onReady: (setToken: (s: string) => void) => void }) {
  const [streaming, setStreaming] = useState('');
  onReady(setStreaming);
  const visible = MESSAGES.slice(Math.max(0, MESSAGES.length - windowSize));
  return (
    <div>
      {visible.map((msg) => (
        <MessageBubble
          key={msg.id}
          message={msg}
          isLastAssistant={msg.id === MESSAGE_COUNT - 1}
          isPending={false}
          onImageClick={noop}
        />
      ))}
      {streaming && (
        <MessageBubble
          message={{
            id: -1,
            thread_id: 1,
            role: 'assistant',
            content: streaming,
            created_at: '2026-07-09T10:00:00Z',
          }}
          isStreaming
        />
      )}
    </div>
  );
}

afterEach(cleanup);

function measure(label: string, windowSize: number) {
  let setToken!: (s: string) => void;

  const mountStart = performance.now();
  render(<Harness window={windowSize} onReady={(fn) => { setToken = fn; }} />);
  const mountMs = performance.now() - mountStart;

  let content = '';
  const streamStart = performance.now();
  for (let i = 0; i < STREAM_TOKENS; i++) {
    content += `token${i} `;
    act(() => setToken(content));
  }
  const streamMs = performance.now() - streamStart;

  process.stdout.write(
    `\n  ${label} (rendered=${Math.min(windowSize, MESSAGE_COUNT)}/${MESSAGE_COUNT}, tokens=${STREAM_TOKENS})\n` +
      `  mount:            ${mountMs.toFixed(0)} ms\n` +
      `  stream total:     ${streamMs.toFixed(0)} ms\n` +
      `  stream per token: ${(streamMs / STREAM_TOKENS).toFixed(1)} ms\n`,
  );
}

test('benchmark: full thread rendered', () => {
  measure('full list', MESSAGE_COUNT);
});

test('benchmark: bounded render window', () => {
  measure('windowed', 50);
});
