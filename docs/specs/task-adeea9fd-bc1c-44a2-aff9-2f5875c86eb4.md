# Fix False Positive Orphaned Task Detection

Tasks that are actively running with a valid in-memory executor sometimes appear as "orphaned" on the dashboard homepage. The orphaned detection works by comparing DB tasks with status='running' against the in-memory executors map, but false positives occur.

## Symptoms

- A task is running normally (visible in `get_runner_status` as a tracked active task)
- But the dashboard homepage shows it with amber "orphaned" styling and AlertTriangle icon
- The orphaned warning banner may also appear

## Relevant Code

- **Backend detection:** `internal/runner/runner.go` — `GetStatus()` (lines ~288-320) and `loadOrphanedRunningTasks()` (lines ~325-355)
- **Frontend display:** `frontend/src/pages/DashboardPage.tsx` (lines ~240-274, 323-356) and `frontend/src/components/RunnerStatus.tsx` (lines ~29-64)
- **Tests:** `internal/runner/runner_test.go` (lines ~483-562)

## Requirements

1. Investigate the root cause of false positives. Likely candidates:
   - Race condition: the status endpoint is called between task DB status update and executor map insertion
   - Timing: `loadOrphanedRunningTasks()` queries the DB while an executor is being set up but not yet in the map
   - The executor map is keyed by ProjectID but orphan check uses TaskID — verify there's no mismatch edge case

2. Fix the root cause so that a task with an active executor is never reported as orphaned.

3. Consider adding a grace period: a task that transitioned to 'running' within the last N seconds (e.g. 30s) should not be considered orphaned, since the executor may still be initializing.

4. Add or update tests to cover the false positive scenario.

5. Verify the fix doesn't break legitimate orphaned task detection (tasks that are truly stuck as 'running' in the DB with no executor after a process restart).