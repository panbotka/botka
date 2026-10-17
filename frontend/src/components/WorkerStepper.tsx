import { Minus, Plus } from 'lucide-react'
import { clsx } from 'clsx'

interface WorkerStepperProps {
  value: number
  canDecrement: boolean
  canIncrement: boolean
  onDecrement: () => void
  onIncrement: () => void
}

const buttonClass = clsx(
  // 44px tap target on mobile, compact on desktop.
  'inline-flex h-11 w-11 items-center justify-center rounded-md text-zinc-600 transition-colors sm:h-8 sm:w-8',
  'hover:bg-zinc-100 hover:text-zinc-900 disabled:cursor-not-allowed disabled:opacity-40 disabled:hover:bg-transparent',
)

/** Compact `− [N] +` control for the runner's concurrent worker count. */
export default function WorkerStepper({
  value,
  canDecrement,
  canIncrement,
  onDecrement,
  onIncrement,
}: WorkerStepperProps) {
  return (
    <div
      role="group"
      aria-label="Max workers"
      className="inline-flex items-center rounded-lg border border-zinc-200"
    >
      <button
        type="button"
        onClick={onDecrement}
        disabled={!canDecrement}
        aria-label="Decrease workers"
        className={buttonClass}
      >
        <Minus className="h-4 w-4" />
      </button>
      <span
        aria-live="polite"
        data-testid="worker-count"
        className="min-w-[1.75rem] text-center text-sm font-semibold tabular-nums text-zinc-900"
      >
        {value}
      </span>
      <button
        type="button"
        onClick={onIncrement}
        disabled={!canIncrement}
        aria-label="Increase workers"
        className={buttonClass}
      >
        <Plus className="h-4 w-4" />
      </button>
    </div>
  )
}
