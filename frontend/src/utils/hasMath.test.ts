import { describe, it, expect } from 'vitest'
import { hasMath } from './hasMath'

describe('hasMath', () => {
  it.each([
    'Display: $$x^2$$',
    'Inline $a + b$ math',
    'Paren \\(x\\) delimiters',
    'Bracket \\[x\\] delimiters',
    'Inline across\nlines $a +\nb$',
  ])('detects math in %j', (content) => {
    expect(hasMath(content)).toBe(true)
  })

  it.each([
    'plain text',
    'costs $5 only',
    '```go\nfmt.Println("hi")\n```',
    '',
  ])('reports no math in %j', (content) => {
    expect(hasMath(content)).toBe(false)
  })
})
