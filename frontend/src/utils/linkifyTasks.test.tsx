import { describe, it, expect } from 'vitest'
import { linkifyTasksMarkdown, linkifyTasksReact } from './linkifyTasks'

const SAMPLE_UUID = '071bc0af-5639-4e4f-94be-ecbae1dc0cd0'

describe('linkifyTasksMarkdown', () => {
  it('converts "Created task <uuid>" to markdown link', () => {
    const input = `Created task ${SAMPLE_UUID}`
    const result = linkifyTasksMarkdown(input)
    expect(result).toBe(`Created task [${SAMPLE_UUID}](/tasks/${SAMPLE_UUID})`)
  })

  it('handles multiple UUIDs in one string', () => {
    const uuid2 = '0f6cbb19-6267-4f87-9c18-b2ea249a807f'
    const input = `Created task ${SAMPLE_UUID} and Created task ${uuid2}`
    const result = linkifyTasksMarkdown(input)
    expect(result).toContain(`[${SAMPLE_UUID}](/tasks/${SAMPLE_UUID})`)
    expect(result).toContain(`[${uuid2}](/tasks/${uuid2})`)
  })

  it('leaves text without task UUIDs unchanged', () => {
    const input = 'No tasks here, just text.'
    expect(linkifyTasksMarkdown(input)).toBe(input)
  })

  it('is case-insensitive', () => {
    const input = `created task ${SAMPLE_UUID}`
    const result = linkifyTasksMarkdown(input)
    expect(result).toContain(`/tasks/${SAMPLE_UUID}`)
  })
})

describe('linkifyTasksReact', () => {
  it('returns text as-is when no UUIDs present', () => {
    const parts = linkifyTasksReact('Just plain text')
    expect(parts).toEqual(['Just plain text'])
  })

  it('creates link elements for task UUIDs', () => {
    const input = `Created task ${SAMPLE_UUID}`
    const parts = linkifyTasksReact(input)
    expect(parts).toHaveLength(1)
    // The link element should be a React element (object with props)
    const link = parts[0] as React.ReactElement
    expect(link).toBeTruthy()
    expect(typeof link).toBe('object')
  })

  it('splits text around UUID into parts', () => {
    const input = `Before Created task ${SAMPLE_UUID} after`
    const parts = linkifyTasksReact(input)
    expect(parts).toHaveLength(3)
    expect(parts[0]).toBe('Before ')
    expect(parts[2]).toBe(' after')
  })
})
