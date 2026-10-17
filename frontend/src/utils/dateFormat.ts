/**
 * Shared date/time formatting utilities.
 * Standard format: DD/MM/YYYY HH:MM
 */

export function formatDate(date: Date | string): string {
  const d = date instanceof Date ? date : new Date(date);
  const day = d.getDate().toString().padStart(2, '0');
  const month = (d.getMonth() + 1).toString().padStart(2, '0');
  const year = d.getFullYear();
  return `${day}/${month}/${year}`;
}

export function formatTime(date: Date | string): string {
  const d = date instanceof Date ? date : new Date(date);
  const hours = d.getHours().toString().padStart(2, '0');
  const minutes = d.getMinutes().toString().padStart(2, '0');
  return `${hours}:${minutes}`;
}

export function formatDateTime(date: Date | string): string {
  return `${formatDate(date)} ${formatTime(date)}`;
}

function isSameDay(a: Date, b: Date): boolean {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

/**
 * Compact Czech-style timestamp that always keeps the time visible, shortening
 * only the date part relative to `now`:
 *   today            → `14:32`
 *   this year        → `17.10. 14:32`
 *   an earlier year  → `17.10.2025 14:32`
 */
export function formatCompactDateTime(date: Date | string, now: Date = new Date()): string {
  const d = date instanceof Date ? date : new Date(date);
  const time = formatTime(d);
  if (isSameDay(d, now)) return time;
  const dayMonth = `${d.getDate()}.${d.getMonth() + 1}.`;
  if (d.getFullYear() === now.getFullYear()) return `${dayMonth} ${time}`;
  return `${dayMonth}${d.getFullYear()} ${time}`;
}

/** Full timestamp with seconds (`17.10.2026 14:32:05`), meant for tooltips. */
export function formatFullDateTime(date: Date | string): string {
  const d = date instanceof Date ? date : new Date(date);
  const seconds = d.getSeconds().toString().padStart(2, '0');
  return `${d.getDate()}.${d.getMonth() + 1}.${d.getFullYear()} ${formatTime(d)}:${seconds}`;
}
