import { useState } from 'react'
import { PowerOff, Loader2, AlertTriangle, CheckCircle2 } from 'lucide-react'
import { clsx } from 'clsx'
import { useBoxAutoOff } from '../hooks/useBoxAutoOff'
import type { BoxAutoOffEvaluation, BoxAutoOffEvent } from '../types'

/**
 * BLOCKER_LABELS maps the server's stable blocker keys (the constants in
 * internal/boxoff/types.go) to display text. An unknown key falls back to the
 * key itself rather than disappearing, so a new server-side condition is
 * visible here before this map catches up.
 */
const BLOCKER_LABELS: Record<string, string> = {
  disabled: 'Auto-off is switched off',
  box_offline: 'Box is not reachable',
  uptime: 'Box has not been up long enough',
  botka_tasks: 'Botka is running tasks',
  botka_box_chats: 'A chat is running on Box',
  kukatko_queue: 'Kukátko has queued jobs',
  gpu: 'GPU is busy',
  cpu: 'CPU is busy',
}

const OUTCOME_LABELS: Record<BoxAutoOffEvent['outcome'], string> = {
  shutdown: 'shut down',
  failed: 'failed',
  aborted: 'aborted — work started',
}

/** formatCountdown renders seconds as MM:SS, or H:MM:SS past an hour. */
function formatCountdown(total: number): string {
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = total % 60
  const pad = (n: number) => String(n).padStart(2, '0')
  return hours > 0 ? `${hours}:${pad(minutes)}:${pad(seconds)}` : `${pad(minutes)}:${pad(seconds)}`
}

/** formatUptime renders a duration in seconds as a compact "9h 16m". */
function formatUptime(seconds: number): string {
  const hours = Math.floor(seconds / 3600)
  const minutes = Math.floor((seconds % 3600) / 60)
  return hours > 0 ? `${hours}h ${minutes}m` : `${minutes}m`
}

/** formatRelative renders how far away a timestamp is, in whole minutes. */
function formatRelative(iso: string): string {
  const deltaMs = new Date(iso).getTime() - Date.now()
  const minutes = Math.max(0, Math.round(deltaMs / 60_000))
  return minutes < 1 ? 'in under a minute' : `in ${minutes} min`
}

/**
 * BoxAutoOffCard shows whether Box will be powered off automatically, when at
 * the earliest, and — when it will not be — exactly which condition is holding
 * it back, with the measured values behind that condition.
 *
 * The countdown is deliberately conditional: the server only supplies
 * `earliest_shutdown_at` when nothing blocks, or when the one blocker with a
 * knowable expiry (uptime) is the only one. Otherwise this renders the reason
 * instead, because a countdown that keeps resetting is worse than none.
 */
export default function BoxAutoOffCard() {
  const { status, loading, error, countdown, setEnabled } = useBoxAutoOff()
  const [saving, setSaving] = useState(false)
  const [saveError, setSaveError] = useState<string | null>(null)

  const handleToggle = async (on: boolean) => {
    setSaving(true)
    setSaveError(null)
    try {
      await setEnabled(on)
    } catch (err) {
      setSaveError(err instanceof Error ? err.message : 'Failed to save the setting')
    } finally {
      setSaving(false)
    }
  }

  const evaluation = status?.last_evaluation ?? null
  const blockers = evaluation?.blockers ?? []
  const primary = blockers[0] ?? null

  return (
    <div className="rounded-lg border border-zinc-200 bg-white dark:bg-zinc-100 p-5 space-y-4">
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <PowerOff className="h-4 w-4 text-zinc-500" />
          <h2 className="text-sm font-semibold uppercase tracking-wider text-zinc-500">
            Auto-off
          </h2>
        </div>
        <label className="flex items-center gap-2 text-sm text-zinc-700">
          {saving && <Loader2 className="h-3.5 w-3.5 animate-spin text-zinc-400" />}
          <input
            type="checkbox"
            checked={status?.enabled ?? false}
            disabled={loading || saving || !status}
            onChange={(e) => void handleToggle(e.target.checked)}
            className="h-4 w-4 rounded border-zinc-300 accent-emerald-600 disabled:cursor-not-allowed"
          />
          Enabled
        </label>
      </div>

      {/* A failed toggle is a direct response to a user action, so it is an
          alert. A failed background poll is not urgent — role="status" keeps
          it polite, and keeps this card from competing with the page's own
          action alerts. */}
      {saveError && (
        <div role="alert" className="rounded-md bg-red-50 px-3 py-2 text-sm text-red-700">
          {saveError}
        </div>
      )}
      {!saveError && error && (
        <div role="status" className="rounded-md bg-amber-50 px-3 py-2 text-sm text-amber-800">
          {error}
        </div>
      )}

      {status && (
        <>
          <div>
            {countdown !== null ? (
              <>
                <div className="flex items-baseline gap-2">
                  <span className="text-2xl font-semibold tabular-nums text-zinc-900">
                    {formatCountdown(countdown)}
                  </span>
                  <span className="text-sm text-zinc-500">until shutdown</span>
                </div>
                <p className="mt-0.5 text-xs text-zinc-500">if nothing changes</p>
              </>
            ) : (
              <div className="flex items-start gap-2">
                <AlertTriangle className="mt-0.5 h-4 w-4 flex-shrink-0 text-amber-500" />
                <div>
                  <p className="text-sm font-medium text-zinc-900">
                    {primary ? (BLOCKER_LABELS[primary.name] ?? primary.name) : 'No check has run yet'}
                  </p>
                  {primary && <p className="text-xs text-zinc-500">{primary.detail}</p>}
                </div>
              </div>
            )}
          </div>

          <div className="flex flex-wrap gap-x-4 gap-y-1 text-xs text-zinc-500">
            <span>
              {status.streak} / {status.required_checks} quiet checks
            </span>
            {status.next_check_at && <span>next check {formatRelative(status.next_check_at)}</span>}
          </div>

          {blockers.length > 1 && (
            <ul className="space-y-1">
              {blockers.slice(1).map((b) => (
                <li key={b.name} className="flex items-start gap-2 text-xs">
                  <AlertTriangle className="mt-0.5 h-3 w-3 flex-shrink-0 text-amber-500" />
                  <span className="text-zinc-700">
                    {BLOCKER_LABELS[b.name] ?? b.name}
                    <span className="text-zinc-500"> — {b.detail}</span>
                  </span>
                </li>
              ))}
            </ul>
          )}

          <Readings evaluation={evaluation} />

          {status.events.length > 0 && (
            <div>
              <h3 className="mb-1 text-xs font-semibold uppercase tracking-wider text-zinc-400">
                Recent attempts
              </h3>
              <ul className="space-y-0.5 text-xs text-zinc-600">
                {status.events.map((ev) => (
                  <li key={ev.id} className="flex items-center gap-2">
                    <span className="tabular-nums text-zinc-500">
                      {new Date(ev.occurred_at).toLocaleString()}
                    </span>
                    <span
                      className={clsx(
                        ev.outcome === 'shutdown' && 'text-emerald-700',
                        ev.outcome === 'failed' && 'text-red-700',
                        ev.outcome === 'aborted' && 'text-amber-700',
                      )}
                    >
                      {OUTCOME_LABELS[ev.outcome]}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
        </>
      )}
    </div>
  )
}

/**
 * Readings shows the measured values behind the decision. The Kukátko line
 * always carries the HTTP status and, on a failed scrape, the error and a slice
 * of the response — the reason has to be visible together with the response
 * that produced it, not summarized away.
 */
function Readings({ evaluation }: { evaluation: BoxAutoOffEvaluation | null }) {
  if (!evaluation?.box && !evaluation?.kukatko) return null

  const { box, kukatko } = evaluation

  return (
    <div className="space-y-1 border-t border-zinc-100 pt-3 text-xs text-zinc-600">
      {box && (
        <p>
          up {formatUptime(box.uptime_seconds)} · load {box.load1.toFixed(2)} ({box.threads}{' '}
          threads) · GPU {box.gpu_max_percent}%
        </p>
      )}
      {kukatko && (
        <p className="flex flex-wrap items-center gap-x-1.5">
          {kukatko.error || kukatko.body_excerpt ? (
            <AlertTriangle className="h-3 w-3 flex-shrink-0 text-amber-500" />
          ) : (
            <CheckCircle2 className="h-3 w-3 flex-shrink-0 text-emerald-500" />
          )}
          <span>
            Kukátko HTTP {kukatko.status_code} · {kukatko.queued} queued, {kukatko.running} running
          </span>
          {kukatko.error && <span className="text-red-700">{kukatko.error}</span>}
          {kukatko.body_excerpt && (
            <span className="font-mono text-zinc-500 break-all">{kukatko.body_excerpt}</span>
          )}
        </p>
      )}
    </div>
  )
}
