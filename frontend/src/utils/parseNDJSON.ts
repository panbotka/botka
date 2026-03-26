// Parsed event types from Claude Code's stream-json NDJSON output.

export type TaskOutputEvent =
  | { type: 'text'; text: string }
  | { type: 'tool_use'; name: string; input: Record<string, unknown> }
  | { type: 'result'; cost_usd: number; duration_ms: number; is_error: boolean }
  | { type: 'system_error'; message: string }

interface StreamLine {
  type: string
  subtype?: string
  message?: unknown
  cost_usd?: number
  duration_ms?: number
}

interface ContentBlock {
  type: string
  text?: string
  name?: string
  input?: Record<string, unknown>
}

interface AssistantMessage {
  role: string
  content: ContentBlock[]
}

interface SystemMessage {
  message?: string
}

function parseAssistantMessage(message: unknown): TaskOutputEvent[] {
  if (!message || typeof message !== 'object') return []
  const msg = message as AssistantMessage
  if (!Array.isArray(msg.content)) return []

  const events: TaskOutputEvent[] = []
  for (const block of msg.content) {
    if (block.type === 'text' && block.text) {
      events.push({ type: 'text', text: block.text })
    } else if (block.type === 'tool_use' && block.name) {
      events.push({
        type: 'tool_use',
        name: block.name,
        input: block.input || {},
      })
    }
  }
  return events
}

function parseSystemMessage(message: unknown): TaskOutputEvent[] {
  if (!message) return []
  if (typeof message === 'string') {
    return [{ type: 'system_error', message }]
  }
  const msg = message as SystemMessage
  if (msg.message) {
    return [{ type: 'system_error', message: msg.message }]
  }
  return []
}

// Parse a single NDJSON line into zero or more TaskOutputEvents.
function parseLine(line: string): TaskOutputEvent[] {
  if (!line.trim()) return []

  let sl: StreamLine
  try {
    sl = JSON.parse(line)
  } catch {
    // Non-JSON line — treat as raw text
    return [{ type: 'text', text: line }]
  }

  switch (sl.type) {
    case 'assistant':
      return parseAssistantMessage(sl.message)
    case 'result':
      return [{
        type: 'result',
        cost_usd: sl.cost_usd || 0,
        duration_ms: sl.duration_ms || 0,
        is_error: sl.subtype !== 'success',
      }]
    case 'system':
      return parseSystemMessage(sl.message)
    default:
      // Unknown types (content_block_delta, etc.) — skip
      return []
  }
}

// Parse raw NDJSON text (multiple lines) into TaskOutputEvents.
export function parseNDJSON(raw: string): TaskOutputEvent[] {
  const events: TaskOutputEvent[] = []
  for (const line of raw.split('\n')) {
    events.push(...parseLine(line))
  }
  return events
}

// Incrementally parse NDJSON from a stream of chunks.
// Handles partial lines across chunk boundaries.
export class NDJSONParser {
  private buffer = ''
  private _events: TaskOutputEvent[] = []

  get events(): TaskOutputEvent[] {
    return this._events
  }

  append(chunk: string): TaskOutputEvent[] {
    this.buffer += chunk
    const newEvents: TaskOutputEvent[] = []

    // Process complete lines
    const lines = this.buffer.split('\n')
    // Keep the last (possibly incomplete) line in the buffer
    this.buffer = lines.pop() || ''

    for (const line of lines) {
      const parsed = parseLine(line)
      newEvents.push(...parsed)
    }

    this._events.push(...newEvents)
    return newEvents
  }

  // Flush any remaining buffer content
  flush(): TaskOutputEvent[] {
    if (!this.buffer.trim()) return []
    const parsed = parseLine(this.buffer)
    this._events.push(...parsed)
    this.buffer = ''
    return parsed
  }
}
