## Problem

The Tasks page (`/tasks`) uses local React state for tab selection (`activeFilter` via `useState`). Tabs like pending, active, running etc. are not reflected in the URL, so you can't share or bookmark a direct link to a specific tab.

## Requirements

1. **Sync active tab with URL query parameter** — e.g. `/tasks?status=pending`, `/tasks?status=running`
2. **Deep links work** — navigating to `/tasks?status=pending` opens the page with the "pending" tab active
3. **Tab changes update the URL** — clicking a tab updates the query param without full page reload (use `useSearchParams` from React Router)
4. **Default behavior preserved** — if no `?status=` param, use the current smart auto-selection logic (first non-empty status: running → pending → queued → all)
5. **Invalid values handled** — if `?status=bogus`, fall back to auto-selection

## Implementation

### File: `frontend/src/pages/TasksPage.tsx`

1. Import `useSearchParams` from `react-router-dom`
2. Read initial tab from `searchParams.get('status')` if present and valid
3. When user clicks a tab, call `setSearchParams({ status: newFilter })` (or remove param for "all")
4. Keep the existing smart auto-selection as fallback when no query param is present

The valid filter values are already defined in the `FILTERS` array (~line 14): `all`, `pending`, `queued`, `running`, `done`, `failed`, `needs_review`.

## Verification

1. Run `make check`
2. Navigate to `/tasks?status=pending` — should show pending tab
3. Click a different tab — URL should update
4. Navigate to `/tasks` without param — should auto-select as before

## CRITICAL: Do NOT deploy or restart

**NEVER run `make deploy`, `make install-service`, `systemctl restart`, or any command that would restart the Botka service.** You are running inside Botka — deploying would kill your own process and leave the task stuck in "running" state forever.