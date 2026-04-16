import { describe, it, expect } from 'vitest'
import { describeCron, formatCronLabel, isLikelyValidCron } from './cronExpression'

describe('describeCron', () => {
  it('describes daily at 09:00', () => {
    expect(describeCron('0 9 * * *')).toBe('Every day at 09:00')
  })

  it('describes weekly on Monday at 09:00', () => {
    expect(describeCron('0 9 * * 1')).toBe('Every Monday at 09:00')
  })

  it('describes "Every 30 minutes"', () => {
    expect(describeCron('*/30 * * * *')).toBe('Every 30 minutes')
  })

  it('describes "Every minute"', () => {
    expect(describeCron('*/1 * * * *')).toBe('Every minute')
  })

  it('describes 1st of every month at 00:00', () => {
    expect(describeCron('0 0 1 * *')).toBe('1st of every month at 00:00')
  })

  it('describes a day of the month with ordinal', () => {
    expect(describeCron('0 9 22 * *')).toBe('22nd of every month at 09:00')
  })

  it('describes multiple weekdays', () => {
    expect(describeCron('30 8 * * 1,3,5')).toBe(
      'Every Monday, Wednesday and Friday at 08:30',
    )
  })

  it('describes hourly', () => {
    expect(describeCron('0 * * * *')).toBe('Every hour (on the hour)')
  })

  it('describes every N hours', () => {
    expect(describeCron('0 */4 * * *')).toBe('Every 4 hours')
  })

  it('describes specific month and day', () => {
    expect(describeCron('0 9 15 6 *')).toBe('Every June 15th at 09:00')
  })

  it('describes @daily predefined schedule', () => {
    expect(describeCron('@daily')).toBe('Every day at 00:00')
  })

  it('returns null for complex patterns', () => {
    // Range syntax we don't expand
    expect(describeCron('0 9-17 * * 1-5')).toBeNull()
  })

  it('returns null for invalid expressions', () => {
    expect(describeCron('not a cron')).toBeNull()
    expect(describeCron('')).toBeNull()
  })
})

describe('formatCronLabel', () => {
  it('falls back to the raw expression when complex', () => {
    expect(formatCronLabel('0 9-17 * * 1-5')).toBe('0 9-17 * * 1-5')
  })

  it('returns description when known', () => {
    expect(formatCronLabel('0 9 * * *')).toBe('Every day at 09:00')
  })
})

describe('isLikelyValidCron', () => {
  it('accepts well-formed expressions', () => {
    expect(isLikelyValidCron('0 9 * * *')).toBe(true)
    expect(isLikelyValidCron('*/30 * * * *')).toBe(true)
    expect(isLikelyValidCron('0 9 * * 1-5')).toBe(true)
    expect(isLikelyValidCron('0 9 * * MON-FRI')).toBe(true)
    expect(isLikelyValidCron('@daily')).toBe(true)
  })

  it('rejects malformed expressions', () => {
    expect(isLikelyValidCron('')).toBe(false)
    expect(isLikelyValidCron('hello world')).toBe(false)
    expect(isLikelyValidCron('0 9 * *')).toBe(false) // 4 fields
    expect(isLikelyValidCron('0 9 * * * *')).toBe(false) // 6 fields
  })
})
