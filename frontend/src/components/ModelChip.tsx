import { useEffect, useState } from 'react'
import { Check } from 'lucide-react'
import { clsx } from 'clsx'
import type { Thread } from '../types'
import { useAvailableModels } from '../hooks/useAvailableModels'
import { saveThreadModel } from '../utils/threadModel'
import BottomSheet from './BottomSheet'

interface Props {
  thread: Thread
  /** Called after a successful switch so the parent reloads its thread list. */
  onChanged: () => void
  /** Read-only users see the model but cannot change it. */
  readOnly?: boolean
}

/**
 * ModelChip is the compact model label in the mobile chat header. Tapping it
 * opens a bottom sheet with the same model list and save path as the model
 * picker in ThreadSettingsPanel; picking an entry switches the model at once.
 * The chip shows the picked model optimistically until the reloaded thread
 * catches up, and reverts if the save fails.
 */
export default function ModelChip({ thread, onChanged, readOnly = false }: Props) {
  const { models, failed } = useAvailableModels()
  const [open, setOpen] = useState(false)
  const [pending, setPending] = useState<string | null>(null)

  // The reloaded thread is now the source of truth again.
  useEffect(() => {
    setPending(null)
  }, [thread.model, thread.id])

  const current = pending ?? thread.model ?? ''
  const label = current || 'Default'

  const choose = async (model: string) => {
    setOpen(false)
    if (model === current) return
    setPending(model)
    const ok = await saveThreadModel(thread.id, model, onChanged)
    if (!ok) setPending(null)
  }

  const chipClass =
    'text-[11px] text-zinc-500 bg-zinc-100 px-2 py-0.5 rounded-md max-w-[96px] truncate'

  if (readOnly) {
    return (
      <span className={clsx(chipClass, 'flex-shrink-0')} data-testid="model-chip">
        {label}
      </span>
    )
  }

  const option = (value: string, text: string) => (
    <li key={value || 'default'}>
      <button
        type="button"
        onClick={() => choose(value)}
        aria-pressed={current === value}
        className={clsx(
          'flex w-full min-h-[48px] items-center gap-3 rounded-lg px-4 text-left text-base transition-colors cursor-pointer',
          current === value ? 'bg-emerald-50 text-emerald-700' : 'text-zinc-700 active:bg-zinc-100',
        )}
      >
        <span className="flex-1 truncate">{text}</span>
        {current === value && <Check className="h-4 w-4 flex-shrink-0" />}
      </button>
    </li>
  )

  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        aria-label={`Model: ${label}`}
        aria-haspopup="dialog"
        data-testid="model-chip"
        className="min-h-[44px] min-w-[44px] flex items-center justify-center flex-shrink-0 cursor-pointer"
      >
        <span className={chipClass}>{label}</span>
      </button>
      <BottomSheet open={open} onClose={() => setOpen(false)} ariaLabel="Model">
        <div className="px-4 pt-1 pb-2 text-[11px] font-medium uppercase tracking-wider text-zinc-400">
          Model
        </div>
        {models === null && !failed && (
          <div className="px-4 pb-4 text-sm text-zinc-400">Načítám modely…</div>
        )}
        {failed && (
          <div className="px-4 pb-4 text-sm text-zinc-500">
            Modely se nepodařilo načíst — model lze zadat v nastavení vlákna.
          </div>
        )}
        {models !== null && !failed && (
          <ul className="p-2 pt-0">
            {option('', 'Default (gateway decides)')}
            {models.map((m) => option(m, m))}
          </ul>
        )}
      </BottomSheet>
    </>
  )
}
