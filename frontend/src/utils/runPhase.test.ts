import { describe, it, expect } from 'vitest'
import { runPhaseLabel } from './runPhase'
import type { RunPhase } from '../types'

describe('runPhaseLabel', () => {
  it('labels every phase in Czech', () => {
    const cases: [RunPhase, string][] = [
      ['preparing', 'příprava'],
      ['agent', 'agent'],
      ['verifying', 'ověřování'],
      ['publishing', 'publikování'],
      ['summarizing', 'shrnutí'],
    ]
    for (const [phase, label] of cases) {
      expect(runPhaseLabel('running', phase)).toBe(label)
    }
  })

  it('renders nothing when the task carries no phase', () => {
    expect(runPhaseLabel('running', null)).toBeNull()
    expect(runPhaseLabel('running', undefined)).toBeNull()
  })

  it('renders nothing for a task that is not running', () => {
    expect(runPhaseLabel('done', 'publishing')).toBeNull()
    expect(runPhaseLabel('failed', 'summarizing')).toBeNull()
    expect(runPhaseLabel('queued', 'preparing')).toBeNull()
  })

  it('renders nothing for a phase this frontend does not know', () => {
    expect(runPhaseLabel('running', 'teleporting' as RunPhase)).toBeNull()
  })
})
