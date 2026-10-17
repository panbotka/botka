import { describe, it, expect } from 'vitest'
import { resolvePrismLanguage } from './prismLanguages'

describe('resolvePrismLanguage', () => {
  it.each([
    ['go', 'go'],
    ['golang', 'go'],
    ['tsx', 'tsx'],
    ['ts', 'typescript'],
    ['js', 'javascript'],
    ['sh', 'bash'],
    ['zsh', 'bash'],
    ['shell', 'bash'],
    ['console', 'bash'],
    ['yml', 'yaml'],
    ['py', 'python'],
    ['html', 'markup'],
    ['dockerfile', 'docker'],
    ['Makefile', 'makefile'],
    ['toml', 'toml'],
    ['nginx', 'nginx'],
  ])('maps %s to %s', (input, expected) => {
    expect(resolvePrismLanguage(input)).toBe(expected)
  })

  it('falls back to plain text for unknown languages', () => {
    expect(resolvePrismLanguage('brainfuck')).toBe('text')
    expect(resolvePrismLanguage('')).toBe('text')
  })
})
