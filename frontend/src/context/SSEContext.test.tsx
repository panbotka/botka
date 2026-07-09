import { describe, it, expect } from 'vitest'
import { SSESessionManager } from './SSEContext'
import type { StreamChunk } from '../api/client'

function chunkStream(chunks: StreamChunk[]) {
  return async function* (): AsyncGenerator<StreamChunk> {
    for (const chunk of chunks) yield chunk
  }
}

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
