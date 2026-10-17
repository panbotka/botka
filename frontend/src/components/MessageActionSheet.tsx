import type { ReactNode } from 'react'
import { clsx } from 'clsx'
import { Copy, Pencil, RefreshCw, GitFork, EyeOff, Eye } from 'lucide-react'
import BottomSheet from './BottomSheet'

interface Props {
  open: boolean
  onClose: () => void
  role: 'user' | 'assistant' | 'system'
  isLastAssistant: boolean
  isHidden?: boolean
  onCopy: () => void
  onEdit?: () => void
  onRegenerate?: () => void
  onFork?: () => void
  onHide?: () => void
}

function Row({ icon, label, onClick }: { icon: ReactNode; label: string; onClick: () => void }) {
  return (
    <li>
      <button
        type="button"
        onClick={onClick}
        className={clsx(
          'flex w-full min-h-[48px] items-center gap-3 rounded-lg px-4 text-left text-base font-medium',
          'text-zinc-700 active:bg-zinc-100 transition-colors cursor-pointer',
        )}
      >
        <span className="flex h-5 w-5 flex-shrink-0 items-center justify-center">{icon}</span>
        <span className="flex-1 truncate">{label}</span>
      </button>
    </li>
  )
}

/**
 * MessageActionSheet is the mobile replacement for the hover-only
 * MessageActions row: a long-press on a message bubble opens it with exactly
 * the actions that message supports. Every action closes the sheet first.
 */
export default function MessageActionSheet({
  open,
  onClose,
  role,
  isLastAssistant,
  isHidden,
  onCopy,
  onEdit,
  onRegenerate,
  onFork,
  onHide,
}: Props) {
  const run = (action: () => void) => () => {
    onClose()
    action()
  }

  return (
    <BottomSheet open={open} onClose={onClose} ariaLabel="Akce zprávy">
      <ul className="p-2">
        <Row icon={<Copy className="h-5 w-5" />} label="Kopírovat" onClick={run(onCopy)} />
        {role === 'user' && onEdit && (
          <Row icon={<Pencil className="h-5 w-5" />} label="Upravit" onClick={run(onEdit)} />
        )}
        {role === 'assistant' && isLastAssistant && onRegenerate && (
          <Row icon={<RefreshCw className="h-5 w-5" />} label="Regenerovat" onClick={run(onRegenerate)} />
        )}
        {onFork && <Row icon={<GitFork className="h-5 w-5" />} label="Forknout odsud" onClick={run(onFork)} />}
        {onHide && (
          <Row
            icon={isHidden ? <Eye className="h-5 w-5" /> : <EyeOff className="h-5 w-5" />}
            label={isHidden ? 'Zobrazit' : 'Skrýt'}
            onClick={run(onHide)}
          />
        )}
      </ul>
    </BottomSheet>
  )
}
