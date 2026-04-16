/**
 * Lightweight helpers for parsing and describing 5-field cron expressions
 * (minute hour day-of-month month day-of-week).
 *
 * Only common patterns are converted to friendly text. For anything more
 * exotic, the helpers return the raw expression so the user can still see
 * what is scheduled.
 */

const DAY_NAMES = [
  'Sunday',
  'Monday',
  'Tuesday',
  'Wednesday',
  'Thursday',
  'Friday',
  'Saturday',
]

const MONTH_NAMES = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
]

interface ParsedField {
  any: boolean // true if the field is "*"
  step?: number // present if field is "*/N"
  values?: number[] // present if field is a comma list or single number
}

function parseField(field: string, min: number, max: number): ParsedField | null {
  if (field === '*') return { any: true }

  // Step pattern: */N
  const stepMatch = /^\*\/(\d+)$/.exec(field)
  if (stepMatch && stepMatch[1] != null) {
    const step = parseInt(stepMatch[1], 10)
    if (Number.isNaN(step) || step < 1) return null
    return { any: true, step }
  }

  // Comma-separated list (or single value)
  const parts = field.split(',')
  const values: number[] = []
  for (const part of parts) {
    if (!/^\d+$/.test(part)) return null
    const n = parseInt(part, 10)
    if (Number.isNaN(n) || n < min || n > max) return null
    values.push(n)
  }
  return { any: false, values }
}

function ordinal(n: number): string {
  const j = n % 10
  const k = n % 100
  if (k >= 11 && k <= 13) return `${n}th`
  if (j === 1) return `${n}st`
  if (j === 2) return `${n}nd`
  if (j === 3) return `${n}rd`
  return `${n}th`
}

function pad(n: number): string {
  return n.toString().padStart(2, '0')
}

/**
 * Validates a 5-field cron expression with simple pattern support
 * (number, asterisk, step, comma list). Returns true if it can be parsed.
 *
 * NOTE: Backend uses robfig/cron which supports more syntaxes (ranges,
 * predefined schedules, etc). This helper is intentionally conservative —
 * falling back to "valid" when we cannot describe the expression but the
 * backend may still accept it. We only return false for clearly malformed
 * input (wrong field count or non-numeric tokens).
 */
export function isLikelyValidCron(expr: string): boolean {
  const trimmed = expr.trim()
  if (!trimmed) return false

  // Allow predefined schedules (backend accepts them).
  if (/^@(yearly|annually|monthly|weekly|daily|midnight|hourly|every\s+\S+)$/i.test(trimmed)) {
    return true
  }

  const fields = trimmed.split(/\s+/)
  if (fields.length !== 5) return false

  const ranges: [number, number][] = [
    [0, 59], // minute
    [0, 23], // hour
    [1, 31], // day of month
    [1, 12], // month
    [0, 6], // day of week
  ]

  for (let i = 0; i < 5; i++) {
    const field = fields[i]!
    // Allow ranges and step within ranges (we don't fully parse these but
    // accept them so the user isn't blocked from valid backend syntax).
    if (/^[\d*,/-]+$/.test(field)) continue
    // Day-of-week names like MON-FRI are also valid for robfig/cron.
    if (/^[A-Za-z,/-]+$/.test(field) && (i === 3 || i === 4)) continue
    void ranges // ranges are documented for clarity above
    return false
  }

  return true
}

/**
 * Converts common cron expressions to friendly English text. For any
 * pattern this helper does not recognize, returns null and callers should
 * fall back to displaying the raw expression.
 */
export function describeCron(expr: string): string | null {
  const trimmed = expr.trim()
  if (!trimmed) return null

  // Predefined schedules
  switch (trimmed.toLowerCase()) {
    case '@yearly':
    case '@annually':
      return 'Once a year (Jan 1 at 00:00)'
    case '@monthly':
      return 'On the 1st of every month at 00:00'
    case '@weekly':
      return 'Every Sunday at 00:00'
    case '@daily':
    case '@midnight':
      return 'Every day at 00:00'
    case '@hourly':
      return 'Every hour'
  }

  const fields = trimmed.split(/\s+/)
  if (fields.length !== 5) return null

  const minute = parseField(fields[0]!, 0, 59)
  const hour = parseField(fields[1]!, 0, 23)
  const dom = parseField(fields[2]!, 1, 31)
  const month = parseField(fields[3]!, 1, 12)
  const dow = parseField(fields[4]!, 0, 6)

  if (!minute || !hour || !dom || !month || !dow) return null

  // "Every N minutes" — minute=*/N, hour=*, dom=*, month=*, dow=*
  if (
    minute.any && minute.step != null &&
    hour.any && !hour.step &&
    dom.any && !dom.step &&
    month.any && !month.step &&
    dow.any && !dow.step
  ) {
    return minute.step === 1 ? 'Every minute' : `Every ${minute.step} minutes`
  }

  // "Every N hours" — minute=0, hour is step, dom=*, month=*, dow=*
  if (
    !minute.any && minute.values && minute.values.length === 1 && minute.values[0] === 0 &&
    hour.any && hour.step != null &&
    dom.any && !dom.step &&
    month.any && !month.step &&
    dow.any && !dow.step
  ) {
    return hour.step === 1 ? 'Every hour' : `Every ${hour.step} hours`
  }

  // "Every hour" — minute=0, hour=*, dom=*, month=*, dow=*
  if (
    !minute.any && minute.values && minute.values.length === 1 && minute.values[0] === 0 &&
    hour.any && !hour.step &&
    dom.any && !dom.step &&
    month.any && !month.step &&
    dow.any && !dow.step
  ) {
    return 'Every hour (on the hour)'
  }

  // Daily/weekly/monthly at specific time — needs a single minute and a single hour.
  if (
    !minute.any && minute.values && minute.values.length === 1 &&
    !hour.any && hour.values && hour.values.length === 1
  ) {
    const m = minute.values[0]!
    const h = hour.values[0]!
    const time = `${pad(h)}:${pad(m)}`

    // Every day at HH:MM — dom=*, month=*, dow=*
    if (dom.any && !dom.step && month.any && !month.step && dow.any && !dow.step) {
      return `Every day at ${time}`
    }

    // Specific weekday(s) at HH:MM — dom=*, month=*, dow has values
    if (
      dom.any && !dom.step &&
      month.any && !month.step &&
      !dow.any && dow.values && dow.values.length > 0
    ) {
      const days = dow.values.map((d) => DAY_NAMES[d % 7]!)
      const dayList = days.length === 1
        ? `Every ${days[0]}`
        : `Every ${days.slice(0, -1).join(', ')} and ${days[days.length - 1]}`
      return `${dayList} at ${time}`
    }

    // Day of month — `0 9 1 * *` → "1st of every month at 09:00"
    if (
      !dom.any && dom.values && dom.values.length === 1 &&
      month.any && !month.step &&
      dow.any && !dow.step
    ) {
      return `${ordinal(dom.values[0]!)} of every month at ${time}`
    }

    // Specific month and day — `0 9 15 6 *` → "Every June 15th at 09:00"
    if (
      !dom.any && dom.values && dom.values.length === 1 &&
      !month.any && month.values && month.values.length === 1 &&
      dow.any && !dow.step
    ) {
      return `Every ${MONTH_NAMES[month.values[0]! - 1]} ${ordinal(dom.values[0]!)} at ${time}`
    }
  }

  return null
}

/**
 * Returns a human-readable label for the given expression. If the helper
 * cannot describe the cron, returns the raw expression so callers can
 * always show something meaningful.
 */
export function formatCronLabel(expr: string): string {
  return describeCron(expr) ?? expr
}
