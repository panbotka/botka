import { describe, it, expect, vi } from 'vitest'
import { SSESessionManager } from './SSEContext'
import type { StreamChunk } from '../api/client'

function chunkStream(chunks: StreamChunk[]) {
  return async function* (): AsyncGenerator<StreamChunk> {
    for (const chunk of chunks) yield chunk
  }
}

// A stream that emits some content and then the body read drops with a raw
// browser network error — the "Load failed" TypeError that surfaces when an
// iOS PWA is backgrounded mid-turn.
function droppingStream(content = 'Partial ') {
  return async function* (): AsyncGenerator<StreamChunk> {
    yield { content }
    throw new TypeError('Load failed')
  }
}

// Sum of the backoff delays (1000 + 2000 + 4000) plus slack, so advancing fake
// timers by this covers every reconnect attempt.
const ALL_BACKOFFS_MS = 10_000

describe('SSESessionManager', () => {
  it('coalesces a burst of content tokens into a single subscriber notification', async () => {
    const manager = new SSESessionManager()
    manager.startSession(1)

    let notifications = 0
    manager.subscribe(1, () => { notifications++ })

    const tokens: StreamChunk[] = Array.from({ length: 40 }, (_, i) => ({ content: `t${i} ` }))
    await manager.runStream(1, chunkStream(tokens))

    // 40 tokens arrive within one frame. Rendering each one is what stalls the
    // main thread on mobile; the manager should collapse them and then flush
    // once at completion.
    expect(notifications).toBeLessThanOrEqual(2)

    const state = manager.getSessionState(1)
    expect(state?.content).toBe(tokens.map((c) => c.content).join('').trim())
    expect(state?.isComplete).toBe(true)
  })

  it('notifies immediately for structural events', async () => {
    const manager = new SSESessionManager()
    manager.startSession(1)

    let notifications = 0
    manager.subscribe(1, () => { notifications++ })

    await manager.runStream(1, chunkStream([
      { tool_use: { id: 'a', name: 'Read', input: {} } },
      { tool_use: { id: 'b', name: 'Edit', input: {} } },
      { title: 'A thread' },
    ]))

    // Two tool calls + a title + the completion flush.
    expect(notifications).toBe(4)
    expect(manager.getSessionState(1)?.toolCalls).toHaveLength(2)
  })

  it('drops a pending coalesced frame when the session is aborted', async () => {
    const manager = new SSESessionManager()
    manager.startSession(1)

    let notifications = 0
    manager.subscribe(1, () => { notifications++ })

    await manager.runStream(1, chunkStream([{ content: 'partial' }]))
    const afterStream = notifications

    manager.abortSession(1)
    await new Promise((r) => requestAnimationFrame(() => r(null)))

    expect(notifications).toBe(afterStream)
    expect(manager.getSessionState(1)).toBeNull()
  })
})

describe('SSESessionManager reconnect recovery', () => {
  it('recovers from a mid-stream transport drop by resubscribing, not by showing an error', async () => {
    vi.useFakeTimers()
    try {
      const manager = new SSESessionManager()
      manager.startSession(1)

      // Resubscribe replays the full buffer and finishes the turn.
      const retryStream = chunkStream([{ content: 'Partial answer.' }, { done: true }])

      const promise = manager.runStream(1, droppingStream(), {
        retryStreamFn: () => retryStream(),
        recoverConnectionErrors: true,
      })

      await vi.advanceTimersByTimeAsync(ALL_BACKOFFS_MS)
      await promise

      const state = manager.getSessionState(1)!
      // The raw "Load failed" TypeError must never become a visible error.
      expect(state.streamError).toBeNull()
      expect(state.streamErrorRaw).toBeNull()
      // The resubscribed content replaced the partial buffer.
      expect(state.content).toBe('Partial answer.')
      expect(state.reconnecting).toBeNull()
      expect(state.isComplete).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })

  it('stays silent when every reconnect attempt is a transport drop, so the DB reload can take over', async () => {
    vi.useFakeTimers()
    try {
      const manager = new SSESessionManager()
      manager.startSession(1)

      const promise = manager.runStream(1, droppingStream(), {
        retryStreamFn: () => droppingStream()(),
        recoverConnectionErrors: true,
      })

      await vi.advanceTimersByTimeAsync(ALL_BACKOFFS_MS)
      await promise

      const state = manager.getSessionState(1)!
      // Exhausted transport-drop retries must not surface a dead-end error block
      // (neither the raw string nor "Server unavailable") — the completion
      // effect reloads the finished answer from the DB instead.
      expect(state.streamError).toBeNull()
      expect(state.streamErrorRaw).toBeNull()
      expect(state.reconnecting).toBeNull()
      expect(state.isComplete).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })

  it('still surfaces a genuine backend error on the reconnect path', async () => {
    const manager = new SSESessionManager()
    manager.startSession(1)

    const errorStream = chunkStream([
      { error: 'model overloaded', error_raw: '{"type":"overloaded"}' },
    ])

    await manager.runStream(1, () => errorStream(), {
      retryStreamFn: () => errorStream(),
      recoverConnectionErrors: true,
    })

    const state = manager.getSessionState(1)!
    expect(state.streamError).toBe('model overloaded')
    expect(state.streamErrorRaw).toBe('{"type":"overloaded"}')
  })

  it('never renders a raw transport error string on the send path', async () => {
    const manager = new SSESessionManager()
    manager.startSession(1)

    // Send path: a retry fn is present but recoverConnectionErrors is not set,
    // so a raw transport drop is not retried — but it must still be sanitized.
    await manager.runStream(1, droppingStream(), {
      retryStreamFn: () => droppingStream()(),
    })

    const state = manager.getSessionState(1)!
    expect(state.streamError).toBe('Server unavailable')
    expect(state.streamError).not.toBe('Load failed')
    expect(state.streamErrorRaw).toBeNull()
  })
})
