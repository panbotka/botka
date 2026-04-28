import { describe, expect, it } from 'vitest'
import type { Project, Thread } from '../types'
import {
  filterChipLabel,
  hasPrefixFilters,
  matchesParsedQuery,
  matchesPrefixFilters,
  parseSearchQuery,
  removeFilter,
} from './searchQuery'

function makeThread(overrides: Partial<Thread> = {}): Thread {
  return {
    id: 1,
    title: '',
    model: 'sonnet',
    system_prompt: '',
    custom_context: '',
    pinned: false,
    archived: false,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    tags: [],
    ...overrides,
  }
}

function makeProject(overrides: Partial<Project> = {}): Project {
  return {
    id: 'p1',
    name: 'Test',
    path: '/tmp/test',
    branch_strategy: 'main',
    active: true,
    claude_md: '',
    sort_order: 0,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('parseSearchQuery', () => {
  it('returns no filters and empty free-text for empty input', () => {
    const q = parseSearchQuery('')
    expect(q.filters).toEqual([])
    expect(q.freeText).toBe('')
    expect(hasPrefixFilters(q)).toBe(false)
  })

  it('parses a simple prefix', () => {
    const q = parseSearchQuery('tag:work')
    expect(q.freeText).toBe('')
    expect(q.filters).toEqual([{ key: 'tag', value: 'work', rawText: 'tag:work' }])
    expect(hasPrefixFilters(q)).toBe(true)
  })

  it('parses combined prefixes preserving order', () => {
    const q = parseSearchQuery('proj:botka tag:work persona:mentor')
    expect(q.freeText).toBe('')
    expect(q.filters.map(f => f.key)).toEqual(['proj', 'tag', 'persona'])
    expect(q.filters.map(f => f.value)).toEqual(['botka', 'work', 'mentor'])
  })

  it('supports quoted values containing spaces', () => {
    const q = parseSearchQuery('tag:"my tag"')
    expect(q.filters).toHaveLength(1)
    expect(q.filters[0]).toEqual({ key: 'tag', value: 'my tag', rawText: 'tag:"my tag"' })
  })

  it('parses mixed free-text and prefix tokens', () => {
    const q = parseSearchQuery('hello tag:work world')
    expect(q.freeText).toBe('hello world')
    expect(q.filters).toHaveLength(1)
    expect(q.filters[0]!.key).toBe('tag')
    expect(q.filters[0]!.value).toBe('work')
  })

  it('treats unknown prefixes as free-text', () => {
    const q = parseSearchQuery('foo:bar')
    expect(q.filters).toEqual([])
    expect(q.freeText).toBe('foo:bar')
  })

  it('ignores tokens with empty values', () => {
    const q = parseSearchQuery('tag: hello')
    expect(q.filters).toEqual([])
    expect(q.freeText).toBe('hello')
  })

  it('parses keys case-insensitively but preserves value casing for substring values', () => {
    const q = parseSearchQuery('TAG:Work Persona:Bob')
    expect(q.filters.map(f => f.key)).toEqual(['tag', 'persona'])
    expect(q.filters.map(f => f.value)).toEqual(['Work', 'Bob'])
  })

  it('parses archived and pinned booleans (case-insensitive)', () => {
    const q = parseSearchQuery('archived:TRUE pinned:false')
    expect(q.filters).toEqual([
      { key: 'archived', value: 'true', rawText: 'archived:TRUE' },
      { key: 'pinned', value: 'false', rawText: 'pinned:false' },
    ])
  })

  it('drops archived/pinned tokens with non-boolean values', () => {
    const q = parseSearchQuery('archived:maybe pinned:yes')
    expect(q.filters).toEqual([])
    expect(q.freeText).toBe('')
  })
})

describe('matchesPrefixFilters', () => {
  const projectMap = new Map<string, Project>([
    ['p1', makeProject({ id: 'p1', name: 'Botka' })],
    ['p2', makeProject({ id: 'p2', name: 'Robot' })],
  ])

  it('matches proj prefix as a case-insensitive substring', () => {
    const thread = makeThread({ project_id: 'p1' })
    expect(matchesPrefixFilters(thread, parseSearchQuery('proj:bot'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(thread, parseSearchQuery('proj:BOT'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(thread, parseSearchQuery('proj:nope'), projectMap)).toBe(false)
  })

  it('rejects threads without a project when proj prefix is present', () => {
    const thread = makeThread({ project_id: undefined })
    expect(matchesPrefixFilters(thread, parseSearchQuery('proj:bot'), projectMap)).toBe(false)
  })

  it('matches tag prefix when at least one tag substring-matches', () => {
    const thread = makeThread({
      tags: [
        { id: 1, name: 'Work', color: '#000', created_at: '' },
        { id: 2, name: 'urgent', color: '#000', created_at: '' },
      ],
    })
    expect(matchesPrefixFilters(thread, parseSearchQuery('tag:wor'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(thread, parseSearchQuery('tag:URG'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(thread, parseSearchQuery('tag:nope'), projectMap)).toBe(false)
  })

  it('matches persona prefix on persona_name substring', () => {
    const thread = makeThread({ persona_name: 'Coding Mentor' })
    expect(matchesPrefixFilters(thread, parseSearchQuery('persona:ment'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(thread, parseSearchQuery('persona:guru'), projectMap)).toBe(false)
  })

  it('honours archived and pinned booleans with strict equality', () => {
    const archived = makeThread({ archived: true })
    const live = makeThread({ archived: false })
    const pinned = makeThread({ pinned: true })
    expect(matchesPrefixFilters(archived, parseSearchQuery('archived:true'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(archived, parseSearchQuery('archived:false'), projectMap)).toBe(false)
    expect(matchesPrefixFilters(live, parseSearchQuery('archived:false'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(pinned, parseSearchQuery('pinned:true'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(pinned, parseSearchQuery('pinned:false'), projectMap)).toBe(false)
  })

  it('ANDs multiple filters together', () => {
    const thread = makeThread({
      project_id: 'p1',
      tags: [{ id: 1, name: 'work', color: '#000', created_at: '' }],
    })
    expect(matchesPrefixFilters(thread, parseSearchQuery('proj:bot tag:work'), projectMap)).toBe(true)
    expect(matchesPrefixFilters(thread, parseSearchQuery('proj:bot tag:home'), projectMap)).toBe(false)
  })
})

describe('matchesParsedQuery', () => {
  const projectMap = new Map<string, Project>([
    ['p1', makeProject({ id: 'p1', name: 'Botka' })],
  ])

  it('combines prefix and free-text matching with AND semantics', () => {
    const thread = makeThread({
      title: 'Hello world',
      project_id: 'p1',
      last_message_preview: '',
    })
    expect(matchesParsedQuery(thread, parseSearchQuery('hello proj:bot'), projectMap)).toBe(true)
    expect(matchesParsedQuery(thread, parseSearchQuery('zzz proj:bot'), projectMap)).toBe(false)
  })

  it('matches free-text against last_message_preview', () => {
    const thread = makeThread({ title: 'Untitled', last_message_preview: 'A reply about cats' })
    expect(matchesParsedQuery(thread, parseSearchQuery('cats'), projectMap)).toBe(true)
    expect(matchesParsedQuery(thread, parseSearchQuery('dogs'), projectMap)).toBe(false)
  })
})

describe('removeFilter', () => {
  it('removes a quoted filter and collapses whitespace', () => {
    const q = parseSearchQuery('tag:"my tag" hello')
    const out = removeFilter('tag:"my tag" hello', q.filters[0]!)
    expect(out).toBe('hello')
  })

  it('removes a filter from the middle of the input', () => {
    const q = parseSearchQuery('hello tag:work world')
    const out = removeFilter('hello tag:work world', q.filters[0]!)
    expect(out).toBe('hello world')
  })

  it('returns the input untouched when the rawText is not present', () => {
    expect(removeFilter('hello', { key: 'tag', value: 'x', rawText: 'tag:x' })).toBe('hello')
  })
})

describe('filterChipLabel', () => {
  it('formats key and value with a colon-space', () => {
    expect(filterChipLabel({ key: 'tag', value: 'work', rawText: 'tag:work' })).toBe('tag: work')
  })
})
