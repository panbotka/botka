# Worker count on the Dashboard

The Dashboard is where the user manages the task runner, but the number of concurrent workers (`max_workers`) can currently only be changed in Settings (`frontend/src/pages/SettingsPage.tsx`, around the max workers field). Bring it into the runner controls and make the runner controls the first thing on the Dashboard.

## Requirements

- In `RunnerControls` on `frontend/src/pages/DashboardPage.tsx`, next to the "X/Y active" indicator, add a compact stepper: `−` [Y] `+` that changes `max_workers`.
- Use the same backend endpoint/setting that the Settings page uses to save max workers — do not add a new persistence mechanism. Check that the running runner picks up the new value without a restart (it does if Settings already works that way; if Settings requires a restart, keep the same semantics and say so in a small hint).
- Bounds: minimum 1, maximum the same upper bound the Settings page / backend validation enforces (if none exists, cap at 5). Buttons disable at the bounds.
- Optimistic update with rollback + an error toast/message if the save fails. Debounce rapid clicks so only the final value is saved (~400ms).
- Lowering the count below the number of currently running tasks does not kill anything; the indicator just shows e.g. "3/2 active" until tasks finish. Show this hint once in muted text when it happens.
- Buttons at least 44px tap target on mobile.
- Move the RunnerControls card to the very top of the Dashboard (above the API usage meters). Keep everything else on the Dashboard in its current order.
- The Settings page field stays and both stay in sync (reading the same value).
- `frontend/src/components/RunnerStatus.tsx` appears unused (nothing imports it) — delete it if that is confirmed.

## Implementation Notes
Add a Vitest test for the stepper (bounds, debounce, rollback on error). `make check` must pass.