import type { Project, Thread } from '../types'

const PREFIX_KEYS = ['proj', 'tag', 'persona', 'archived', 'pinned'] as const

export type PrefixKey = (typeof PREFIX_KEYS)[number]

const PREFIX_KEY_SET: Set<string> = new Set(PREFIX_KEYS)

export interface ParsedFilter {
  /** Lowercase prefix key, e.g. `tag`. */
  key: PrefixKey
  /** Filter value. For archived/pinned this is the literal string `true` or `false`. */
  value: string
  /**
   * Original substring of the input that produced this filter, including any
   * quotes. Used to remove the chip from the search input.
   */
  rawText: string
}

export interface ParsedQuery {
  /** Joined free-text portion of the query, trimmed. Empty string if none. */
  freeText: string
  /** Active prefix filters in the order they appeared. */
  filters: ParsedFilter[]
}

interface RawToken {
  /** Original substring including any quote characters. */
  raw: string
  /** Token content with surrounding/internal quote characters stripped. */
  content: string
}

function tokenize(input: string): RawToken[] {
  const tokens: RawToken[] = []
  const len = input.length
  let i = 0
  while (i < len) {
    if (/\s/.test(input.charAt(i))) {
      i++
      continue
    }
    const start = i
    let content = ''
    let inQuotes = false
    while (i < len && (inQuotes || !/\s/.test(input.charAt(i)))) {
      const ch = input.charAt(i)
      if (ch === '"') {
        inQuotes = !inQuotes
      } else {
        content += ch
      }
      i++
    }
    tokens.push({ raw: input.slice(start, i), content })
  }
  return tokens
}

/** Parse a search input into prefix filters and a free-text remainder. */
export function parseSearchQuery(input: string): ParsedQuery {
  const filters: ParsedFilter[] = []
  const freeParts: string[] = []
  for (const token of tokenize(input)) {
    const colonIdx = token.content.indexOf(':')
    if (colonIdx === -1) {
      freeParts.push(token.content)
      continue
    }
    const key = token.content.slice(0, colonIdx).toLowerCase()
    const value = token.content.slice(colonIdx + 1)
    if (!PREFIX_KEY_SET.has(key)) {
      // Unknown prefix — treat the whole token as free-text per spec.
      freeParts.push(token.content)
      continue
    }
    if (!value) {
      // Empty value — drop the token entirely (do not surface as free-text).
      continue
    }
    if (key === 'archived' || key === 'pinned') {
      const lower = value.toLowerCase()
      if (lower !== 'true' && lower !== 'false') continue
      filters.push({ key, value: lower, rawText: token.raw })
      continue
    }
    filters.push({ key: key as PrefixKey, value, rawText: token.raw })
  }
  return { freeText: freeParts.join(' ').trim(), filters }
}

export function hasPrefixFilters(query: ParsedQuery): boolean {
  return query.filters.length > 0
}

/**
 * True when `thread` matches every prefix filter in `query`. Free-text is NOT
 * considered here — the caller runs the global search API for that part and
 * then uses this helper to post-filter the API thread results client-side.
 */
export function matchesPrefixFilters(
  thread: Thread,
  query: ParsedQuery,
  projectMap: Map<string, Project>,
): boolean {
  for (const f of query.filters) {
    if (f.key === 'proj') {
      const projectName = thread.project_id ? projectMap.get(thread.project_id)?.name : undefined
      if (!projectName) return false
      if (!projectName.toLowerCase().includes(f.value.toLowerCase())) return false
    } else if (f.key === 'tag') {
      const tags = thread.tags || []
      const lower = f.value.toLowerCase()
      if (!tags.some(t => t.name.toLowerCase().includes(lower))) return false
    } else if (f.key === 'persona') {
      const personaName = thread.persona_name
      if (!personaName) return false
      if (!personaName.toLowerCase().includes(f.value.toLowerCase())) return false
    } else if (f.key === 'archived') {
      if (thread.archived !== (f.value === 'true')) return false
    } else if (f.key === 'pinned') {
      if (thread.pinned !== (f.value === 'true')) return false
    }
  }
  return true
}

/**
 * True when `thread` matches both the prefix filters AND the free-text portion
 * (case-insensitive substring match on `title` and `last_message_preview`).
 * Used by the prefix-only local filter path.
 */
export function matchesParsedQuery(
  thread: Thread,
  query: ParsedQuery,
  projectMap: Map<string, Project>,
): boolean {
  if (!matchesPrefixFilters(thread, query, projectMap)) return false
  if (query.freeText) {
    const lower = query.freeText.toLowerCase()
    const title = (thread.title || '').toLowerCase()
    const preview = (thread.last_message_preview || '').toLowerCase()
    if (!title.includes(lower) && !preview.includes(lower)) return false
  }
  return true
}

/** Remove a filter token from the search input string. */
export function removeFilter(input: string, filter: ParsedFilter): string {
  const idx = input.indexOf(filter.rawText)
  if (idx === -1) return input
  const before = input.slice(0, idx)
  const after = input.slice(idx + filter.rawText.length)
  return (before + ' ' + after).replace(/\s+/g, ' ').trim()
}

/** Display label for a filter (used as chip text). */
export function filterChipLabel(filter: ParsedFilter): string {
  return `${filter.key}: ${filter.value}`
}
