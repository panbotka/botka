export interface ThreadColor {
  key: string
  label: string
  lightBg: string
  darkBg: string
  swatch: string
}

export const THREAD_COLORS: ThreadColor[] = [
  { key: '', label: 'None', lightBg: 'transparent', darkBg: 'transparent', swatch: 'transparent' },
  { key: 'red', label: 'Red', lightBg: '#fef2f2', darkBg: 'rgba(239,68,68,0.08)', swatch: '#ef4444' },
  { key: 'orange', label: 'Orange', lightBg: '#fff7ed', darkBg: 'rgba(249,115,22,0.08)', swatch: '#f97316' },
  { key: 'amber', label: 'Amber', lightBg: '#fffbeb', darkBg: 'rgba(245,158,11,0.08)', swatch: '#f59e0b' },
  { key: 'yellow', label: 'Yellow', lightBg: '#fefce8', darkBg: 'rgba(234,179,8,0.08)', swatch: '#eab308' },
  { key: 'lime', label: 'Lime', lightBg: '#f7fee7', darkBg: 'rgba(132,204,22,0.08)', swatch: '#84cc16' },
  { key: 'green', label: 'Green', lightBg: '#f0fdf4', darkBg: 'rgba(34,197,94,0.08)', swatch: '#22c55e' },
  { key: 'emerald', label: 'Emerald', lightBg: '#ecfdf5', darkBg: 'rgba(16,185,129,0.08)', swatch: '#10b981' },
  { key: 'teal', label: 'Teal', lightBg: '#f0fdfa', darkBg: 'rgba(20,184,166,0.08)', swatch: '#14b8a6' },
  { key: 'cyan', label: 'Cyan', lightBg: '#ecfeff', darkBg: 'rgba(6,182,212,0.08)', swatch: '#06b6d4' },
  { key: 'sky', label: 'Sky', lightBg: '#f0f9ff', darkBg: 'rgba(14,165,233,0.08)', swatch: '#0ea5e9' },
  { key: 'blue', label: 'Blue', lightBg: '#eff6ff', darkBg: 'rgba(59,130,246,0.08)', swatch: '#3b82f6' },
  { key: 'indigo', label: 'Indigo', lightBg: '#eef2ff', darkBg: 'rgba(99,102,241,0.08)', swatch: '#6366f1' },
  { key: 'violet', label: 'Violet', lightBg: '#f5f3ff', darkBg: 'rgba(139,92,246,0.08)', swatch: '#8b5cf6' },
  { key: 'purple', label: 'Purple', lightBg: '#faf5ff', darkBg: 'rgba(168,85,247,0.08)', swatch: '#a855f7' },
  { key: 'rose', label: 'Rose', lightBg: '#fff1f2', darkBg: 'rgba(244,63,94,0.08)', swatch: '#f43f5e' },
]

export function getThreadBackground(colorKey: string | undefined, theme: string): string {
  if (!colorKey) return 'transparent'
  const color = THREAD_COLORS.find(c => c.key === colorKey)
  if (!color) return 'transparent'
  return theme === 'light' ? color.lightBg : color.darkBg
}
