import { describe, it, expect } from 'vitest'
import { render } from '@testing-library/react'
import * as fs from 'node:fs'
import * as path from 'node:path'
import * as url from 'node:url'
import MarkdownContent from './MarkdownContent'

const HERE = path.dirname(url.fileURLToPath(import.meta.url))
const INDEX_CSS = fs.readFileSync(path.join(HERE, '..', 'index.css'), 'utf-8')

describe('MarkdownContent', () => {
  it('wraps content in a .markdown-content container', () => {
    const { container } = render(<MarkdownContent content="hello" />)
    const wrapper = container.querySelector('.markdown-content')
    expect(wrapper).not.toBeNull()
  })

  it('renders a 300-char URL without horizontal overflow', () => {
    const longUrl = 'https://example.com/' + 'a'.repeat(280)
    expect(longUrl.length).toBeGreaterThanOrEqual(300)

    const { container } = render(<MarkdownContent content={longUrl} />)

    const wrapper = container.querySelector('.markdown-content')
    expect(wrapper).not.toBeNull()

    // The long URL must actually appear in the rendered DOM.
    expect(container.textContent).toContain(longUrl)

    // The rendered anchor (if produced by remark-gfm autolink) must sit inside
    // the .markdown-content wrapper — the CSS rule below ensures wrapping.
    const anchor = container.querySelector('a')
    if (anchor) {
      expect(wrapper!.contains(anchor)).toBe(true)
    }

    // jsdom does not perform layout, so we assert the CSS contract that
    // prevents overflow: `.markdown-content` and its text-carrying descendants
    // must declare `overflow-wrap: anywhere`.
    expect(INDEX_CSS).toMatch(
      /\.markdown-content\s*\{[^}]*overflow-wrap:\s*anywhere/,
    )
    expect(INDEX_CSS).toMatch(
      /\.markdown-content\s+p[\s\S]*?overflow-wrap:\s*anywhere/,
    )
  })
})
