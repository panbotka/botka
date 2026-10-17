import { describe, it, expect } from 'vitest'
import { formatCompactDateTime, formatFullDateTime } from './dateFormat'

// Local-time constructors so the assertions hold in any test timezone.
const now = new Date(2026, 9, 17, 18, 0, 0)

describe('formatCompactDateTime', () => {
  it('shows only the time for today', () => {
    expect(formatCompactDateTime(new Date(2026, 9, 17, 14, 32), now)).toBe('14:32')
    expect(formatCompactDateTime(new Date(2026, 9, 17, 0, 5), now)).toBe('00:05')
  })

  it('shows day.month. and time within the current year', () => {
    expect(formatCompactDateTime(new Date(2026, 9, 16, 14, 32), now)).toBe('16.10. 14:32')
    expect(formatCompactDateTime(new Date(2026, 0, 5, 9, 7), now)).toBe('5.1. 09:07')
  })

  it('includes the year for older timestamps', () => {
    expect(formatCompactDateTime(new Date(2025, 9, 17, 14, 32), now)).toBe('17.10.2025 14:32')
  })

  it('accepts ISO strings', () => {
    const iso = new Date(2026, 9, 17, 14, 32).toISOString()
    expect(formatCompactDateTime(iso, now)).toBe('14:32')
  })
})

describe('formatFullDateTime', () => {
  it('includes the full date and seconds', () => {
    expect(formatFullDateTime(new Date(2026, 9, 17, 14, 32, 5))).toBe('17.10.2026 14:32:05')
  })
})
