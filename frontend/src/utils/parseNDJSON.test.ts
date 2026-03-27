import { describe, it, expect } from 'vitest'
import { parseNDJSON, NDJSONParser } from './parseNDJSON'

describe('parseNDJSON', () => {
  it('parses assistant text messages', () => {
    const line = JSON.stringify({
      type: 'assistant',
      message: { role: 'assistant', content: [{ type: 'text', text: 'Hello world' }] },
    })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'text', text: 'Hello world' }])
  })

  it('parses assistant tool_use messages', () => {
    const line = JSON.stringify({
      type: 'assistant',
      message: {
        role: 'assistant',
        content: [{ type: 'tool_use', name: 'Bash', input: { command: 'ls' } }],
      },
    })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'tool_use', name: 'Bash', input: { command: 'ls' } }])
  })

  it('parses result events', () => {
    const line = JSON.stringify({
      type: 'result',
      subtype: 'success',
      cost_usd: 0.05,
      duration_ms: 1200,
    })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'result', cost_usd: 0.05, duration_ms: 1200, is_error: false }])
  })

  it('parses result events with error subtype', () => {
    const line = JSON.stringify({
      type: 'result',
      subtype: 'error',
      cost_usd: 0.01,
      duration_ms: 500,
    })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'result', cost_usd: 0.01, duration_ms: 500, is_error: true }])
  })

  it('parses system error messages (string)', () => {
    const line = JSON.stringify({ type: 'system', message: 'Something went wrong' })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'system_error', message: 'Something went wrong' }])
  })

  it('parses system error messages (object)', () => {
    const line = JSON.stringify({ type: 'system', message: { message: 'Rate limited' } })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'system_error', message: 'Rate limited' }])
  })

  it('ignores unknown event types', () => {
    const line = JSON.stringify({ type: 'content_block_delta' })
    const events = parseNDJSON(line)
    expect(events).toEqual([])
  })

  it('treats non-JSON lines as raw text', () => {
    const events = parseNDJSON('raw output text')
    expect(events).toEqual([{ type: 'text', text: 'raw output text' }])
  })

  it('skips empty lines', () => {
    const events = parseNDJSON('\n\n')
    expect(events).toEqual([])
  })

  it('parses multiple lines', () => {
    const lines = [
      JSON.stringify({ type: 'assistant', message: { content: [{ type: 'text', text: 'Line 1' }] } }),
      JSON.stringify({ type: 'result', subtype: 'success', cost_usd: 0.1, duration_ms: 2000 }),
    ].join('\n')
    const events = parseNDJSON(lines)
    expect(events).toHaveLength(2)
    expect(events[0]).toEqual({ type: 'text', text: 'Line 1' })
    expect(events[1]).toEqual({ type: 'result', cost_usd: 0.1, duration_ms: 2000, is_error: false })
  })

  it('handles assistant message with empty content', () => {
    const line = JSON.stringify({ type: 'assistant', message: { content: [] } })
    expect(parseNDJSON(line)).toEqual([])
  })

  it('handles assistant message with null message', () => {
    const line = JSON.stringify({ type: 'assistant', message: null })
    expect(parseNDJSON(line)).toEqual([])
  })

  it('defaults cost_usd and duration_ms to 0 when missing', () => {
    const line = JSON.stringify({ type: 'result', subtype: 'success' })
    const events = parseNDJSON(line)
    expect(events).toEqual([{ type: 'result', cost_usd: 0, duration_ms: 0, is_error: false }])
  })
})

describe('NDJSONParser', () => {
  it('incrementally parses complete lines', () => {
    const parser = new NDJSONParser()
    const events1 = parser.append(
      JSON.stringify({ type: 'assistant', message: { content: [{ type: 'text', text: 'hi' }] } }) + '\n',
    )
    expect(events1).toEqual([{ type: 'text', text: 'hi' }])
    expect(parser.events).toEqual([{ type: 'text', text: 'hi' }])
  })

  it('buffers partial lines across chunks', () => {
    const parser = new NDJSONParser()
    const fullLine = JSON.stringify({
      type: 'assistant',
      message: { content: [{ type: 'text', text: 'hello' }] },
    })
    const half1 = fullLine.slice(0, 20)
    const half2 = fullLine.slice(20) + '\n'

    const events1 = parser.append(half1)
    expect(events1).toEqual([])

    const events2 = parser.append(half2)
    expect(events2).toEqual([{ type: 'text', text: 'hello' }])
  })

  it('flushes remaining buffer', () => {
    const parser = new NDJSONParser()
    parser.append('raw text without newline')
    const flushed = parser.flush()
    expect(flushed).toEqual([{ type: 'text', text: 'raw text without newline' }])
  })

  it('flush returns empty array for empty buffer', () => {
    const parser = new NDJSONParser()
    expect(parser.flush()).toEqual([])
  })

  it('accumulates events across multiple appends', () => {
    const parser = new NDJSONParser()
    parser.append(JSON.stringify({ type: 'assistant', message: { content: [{ type: 'text', text: 'a' }] } }) + '\n')
    parser.append(JSON.stringify({ type: 'assistant', message: { content: [{ type: 'text', text: 'b' }] } }) + '\n')
    expect(parser.events).toHaveLength(2)
  })
})
