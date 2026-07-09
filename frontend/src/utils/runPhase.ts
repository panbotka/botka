import type { RunPhase, TaskStatus } from '../types'

// Czech labels for the executor phases, shown next to a running task's badge.
const runPhaseLabels: Record<RunPhase, string> = {
  preparing: 'příprava',
  agent: 'agent',
  verifying: 'ověřování',
  publishing: 'publikování',
  summarizing: 'shrnutí',
}

// runPhaseLabel returns the Czech label for a phase, or null when there is
// nothing to show — either the task is not running, the backend sent no phase,
// or it sent one this frontend does not know yet.
export function runPhaseLabel(
  status: TaskStatus,
  phase: RunPhase | null | undefined,
): string | null {
  if (status !== 'running' || !phase) return null
  return runPhaseLabels[phase] ?? null
}
