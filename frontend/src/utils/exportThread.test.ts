import { describe, it, expect } from 'vitest'
import { exportAsMarkdown, exportAsJSON } from './exportThread'
import type { Message, Thread } from '../types'

function makeMessage(overrides: Partial<Message> = {}): Message {
  return {
    id: 1,
    thread_id: 1,
    role: 'user',
    content: 'Hello',
    created_at: '2024-01-01T12:00:00Z',
    ...overrides,
  }
}

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: 1,
    title: 'Test Thread',
    model: 'sonnet',
    system_prompt: '',
    pinned: false,
    archived: false,
    created_at: '2024-06-15T10:30:00Z',
    updated_at: '2024-06-15T11:00:00Z',
    ...overrides,
  }
}

describe('exportAsMarkdown', () => {
  it('includes thread title as heading', () => {
    const md = exportAsMarkdown([], makeThread({ title: 'My Chat' }))
    expect(md).toContain('# My Chat')
  })

  it('labels user and assistant messages correctly', () => {
    const messages = [
      makeMessage({ role: 'user', content: 'Hi there' }),
      makeMessage({ role: 'assistant', content: 'Hello!' }),
    ]
    const md = exportAsMarkdown(messages)
    expect(md).toContain('**User:**')
    expect(md).toContain('Hi there')
    expect(md).toContain('**Assistant:**')
    expect(md).toContain('Hello!')
  })

  it('includes message count in metadata', () => {
    const messages = [makeMessage(), makeMessage({ id: 2, content: 'World' })]
    const md = exportAsMarkdown(messages)
    expect(md).toContain('Messages: 2')
  })

  it('includes model in metadata when thread provided', () => {
    const md = exportAsMarkdown([], makeThread({ model: 'opus' }))
    expect(md).toContain('Model: opus')
  })

  it('works without thread', () => {
    const md = exportAsMarkdown([makeMessage()])
    expect(md).toContain('**User:**')
    expect(md).toContain('Messages: 1')
  })

  it('includes separator', () => {
    const md = exportAsMarkdown([])
    expect(md).toContain('---')
  })
})

describe('exportAsJSON', () => {
  it('returns valid JSON', () => {
    const json = exportAsJSON([makeMessage()])
    expect(() => JSON.parse(json)).not.toThrow()
  })

  it('includes thread metadata when provided', () => {
    const thread = makeThread({ title: 'My Thread', model: 'haiku' })
    const json = exportAsJSON([], thread)
    const parsed = JSON.parse(json)
    expect(parsed.thread.title).toBe('My Thread')
    expect(parsed.thread.model).toBe('haiku')
  })

  it('sets thread to null when not provided', () => {
    const json = exportAsJSON([makeMessage()])
    const parsed = JSON.parse(json)
    expect(parsed.thread).toBeNull()
  })

  it('maps messages to role, content, created_at', () => {
    const messages = [
      makeMessage({ role: 'user', content: 'Hi', created_at: '2024-01-01T00:00:00Z' }),
    ]
    const json = exportAsJSON(messages)
    const parsed = JSON.parse(json)
    expect(parsed.messages).toHaveLength(1)
    expect(parsed.messages[0]).toEqual({
      role: 'user',
      content: 'Hi',
      created_at: '2024-01-01T00:00:00Z',
    })
  })

  it('strips extra fields from messages', () => {
    const messages = [
      makeMessage({ id: 42, thread_id: 5, role: 'assistant', content: 'Test' }),
    ]
    const json = exportAsJSON(messages)
    const parsed = JSON.parse(json)
    expect(parsed.messages[0]).not.toHaveProperty('id')
    expect(parsed.messages[0]).not.toHaveProperty('thread_id')
  })
})
