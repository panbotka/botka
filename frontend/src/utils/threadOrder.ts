import type { Thread } from '../types';

/** Returns non-archived threads in sidebar visual order: pinned first, then regular. */
export function getSidebarThreads(threads: Thread[]): Thread[] {
  const pinned: Thread[] = [];
  const regular: Thread[] = [];
  for (const t of threads) {
    if (t.archived) continue;
    if (t.pinned) pinned.push(t);
    else regular.push(t);
  }
  return [...pinned, ...regular];
}
