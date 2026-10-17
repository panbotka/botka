import { describe, it, expect } from 'vitest'
import { summarizeToolCall, toolSummaryParts, czechToolCount } from './toolSummary'

describe('summarizeToolCall', () => {
  it('shows the Bash command', () => {
    expect(summarizeToolCall('Bash', { command: 'make test' })).toBe('Bash · make test')
  })

  it('keeps only the first line of a multi-line command and collapses whitespace', () => {
    expect(summarizeToolCall('Bash', { command: '\n  cd   frontend\nnpm test' })).toBe('Bash · cd frontend')
  })

  it('caps very long details', () => {
    const detail = toolSummaryParts('Bash', { command: 'x'.repeat(500) }).detail
    expect(detail.length).toBe(160)
    expect(detail.endsWith('…')).toBe(true)
  })

  it('uses the file basename for file tools', () => {
    expect(summarizeToolCall('Read', { file_path: '/repo/internal/handlers/chat.go' })).toBe('Read · chat.go')
    expect(summarizeToolCall('Edit', { file_path: '/repo/src/TaskList.tsx', old_string: 'a', new_string: 'b' })).toBe('Edit · TaskList.tsx')
    expect(summarizeToolCall('Write', { file_path: 'notes.md', content: 'hi' })).toBe('Write · notes.md')
    expect(summarizeToolCall('NotebookEdit', { notebook_path: '/a/b.ipynb' })).toBe('NotebookEdit · b.ipynb')
  })

  it('shows the pattern for Grep and Glob', () => {
    expect(summarizeToolCall('Grep', { pattern: 'func main', path: '/repo' })).toBe('Grep · func main')
    expect(summarizeToolCall('Glob', { pattern: '**/*.ts' })).toBe('Glob · **/*.ts')
  })

  it('shows the hostname for WebFetch, falling back to the raw value', () => {
    expect(summarizeToolCall('WebFetch', { url: 'https://example.com/a/b?c=d' })).toBe('WebFetch · example.com')
    expect(summarizeToolCall('WebFetch', { url: 'not a url' })).toBe('WebFetch · not a url')
  })

  it('shows the query for WebSearch', () => {
    expect(summarizeToolCall('WebSearch', { query: 'vite pwa' })).toBe('WebSearch · vite pwa')
  })

  it('prefers the description for Task and Agent', () => {
    expect(summarizeToolCall('Task', { description: 'Find usages', prompt: 'long prompt' })).toBe('Task · Find usages')
    expect(summarizeToolCall('Agent', { subagent_type: 'Explore' })).toBe('Agent · Explore')
    expect(summarizeToolCall('Agent', { prompt: 'Look at\nthe code' })).toBe('Agent · Look at')
  })

  it('formats MCP tools as server: tool', () => {
    expect(summarizeToolCall('mcp__botka__create_task', { title: 'x' })).toBe('botka: create_task')
    expect(summarizeToolCall('mcp__claude_ai_Claude_Docs__batch', {})).toBe('claude_ai_Claude_Docs: batch')
  })

  it('shows only the name for unknown tools or missing input', () => {
    expect(summarizeToolCall('TodoWrite', { todos: [] })).toBe('TodoWrite')
    expect(summarizeToolCall('Bash', {})).toBe('Bash')
    expect(summarizeToolCall('Read', null)).toBe('Read')
    expect(summarizeToolCall('Bash', { command: 42 })).toBe('Bash')
  })
})

describe('czechToolCount', () => {
  it('declines the noun', () => {
    expect(czechToolCount(1)).toBe('1 nástroj')
    expect(czechToolCount(2)).toBe('2 nástroje')
    expect(czechToolCount(4)).toBe('4 nástroje')
    expect(czechToolCount(5)).toBe('5 nástrojů')
    expect(czechToolCount(0)).toBe('0 nástrojů')
  })
})
