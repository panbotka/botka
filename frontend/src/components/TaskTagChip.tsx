import { clsx } from 'clsx'
import type { TaskTag } from '../types'

interface TaskTagChipProps {
  tag: TaskTag
  size?: 'xs' | 'sm'
  onRemove?: () => void
  onClick?: () => void
  selected?: boolean
  className?: string
  title?: string
}

// hexToRGB converts a #RRGGBB color to its R/G/B integers.
function hexToRGB(hex: string): { r: number; g: number; b: number } | null {
  const m = /^#?([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex.trim())
  if (!m) return null
  return {
    r: parseInt(m[1]!, 16),
    g: parseInt(m[2]!, 16),
    b: parseInt(m[3]!, 16),
  }
}

// readableTextColor picks between near-black and near-white text based on the
// perceived luminance of the given background color, matching WCAG guidance.
function readableTextColor(bg: string): string {
  const rgb = hexToRGB(bg)
  if (!rgb) return '#27272a'
  const luminance = (0.299 * rgb.r + 0.587 * rgb.g + 0.114 * rgb.b) / 255
  return luminance > 0.6 ? '#27272a' : '#ffffff'
}

export function TaskTagChip({
  tag,
  size = 'sm',
  onRemove,
  onClick,
  selected,
  className,
  title,
}: TaskTagChipProps) {
  const bg = tag.color
  const fg = readableTextColor(bg)
  const ring = selected ? '0 0 0 1.5px rgba(24, 24, 27, 0.85)' : 'none'
  const Tag = onClick ? 'button' : 'span'

  return (
    <Tag
      type={onClick ? 'button' : undefined}
      onClick={onClick}
      title={title ?? tag.name}
      className={clsx(
        'inline-flex items-center gap-1 rounded-full font-medium',
        size === 'xs' ? 'px-1.5 py-0.5 text-[10px]' : 'px-2 py-0.5 text-xs',
        onClick && 'cursor-pointer hover:opacity-90 transition-opacity',
        className,
      )}
      style={{ backgroundColor: bg, color: fg, boxShadow: ring }}
    >
      {tag.name}
      {onRemove && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation()
            onRemove()
          }}
          className="-mr-0.5 ml-0.5 rounded-full hover:bg-black/10 px-1 leading-none"
          aria-label={`Remove ${tag.name}`}
        >
          ×
        </button>
      )}
    </Tag>
  )
}
