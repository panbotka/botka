# Keepalive: ping after the window resets, not before

**Status:** Approved
**Date:** 2026-07-17

## Problem

The keepalive loop exists to keep the Anthropic 5-hour rate-limit window
active, so Botka's autonomous task runner always has a live window with fresh
capacity. It does the opposite of that today.

`keepaliveLoop` (`internal/runner/keepalive.go:34`) schedules its ping at
`resets_at - KEEPALIVE_LEAD_TIME`, i.e. 15 minutes *before* the current window
expires. That ping lands inside the window that is already open and about to
close. It cannot open the next one: Anthropic's 5h windows are anchored at
first use, not sliding, so the window still resets at `resets_at` and the next
one only begins whenever some later message happens to arrive.

The consequence is a gap. With `resets_at` at 10:20, the window closes at 10:20
and — absent other traffic — nothing runs until a task or chat message opens a
fresh window at, say, 14:00, which then runs 14:00–19:00. Windows start lazily
and the day yields fewer of them than it could.

Pinging just *after* `resets_at` instead makes windows run back-to-back
(10:20 → 15:20 → 20:20 …), which is the maximum number of windows per day and
the point of the feature.

A second problem compounds the first.
`KEEPALIVE_ACTIVITY_THRESHOLD` (50m) suppresses the ping when a task started or
a chat message arrived within the last 50 minutes. Under the corrected timing
that check is not merely redundant, it is backwards: activity *before*
`resets_at` belongs to the old window and cannot open the new one. A task
running across the boundary would suppress exactly the ping that matters.

## Goals

- The 5h window is open essentially always: a new one begins ~1 minute after
  the previous one resets, without waiting for organic traffic.
- The ping is unconditional with respect to activity.
- No double ping into a single window.

## Non-goals

- Changing what the ping itself does (`claude -p "reply with pong"`).
- Changing the `UsageMonitor`, its 30s poll, or the `claude-usage` cache.
- Changing the rate-limit gates that govern task scheduling.

## Design

### Timing

Schedule the ping at `resets_at + KEEPALIVE_RESET_DELAY`, default `2m`.

The trade is asymmetric, so the default is deliberately conservative. Firing
too early is the failure that costs a whole window: a ping landing a second
before the true reset is spent on the closing window, opens nothing, and the
loop then waits ~5h for the next boundary. Firing late costs exactly the wait —
two minutes out of three hundred, 0.7% of a window. Two minutes covers clock
skew, the precision of `resets_at`, and timer latency with room left over.

The delay is an env var precisely so it can be tightened without a rebuild if
it proves over-cautious in practice.

`KEEPALIVE_LEAD_TIME` is removed rather than reinterpreted — there is no longer
anything to lead.

### Activity check

Removed entirely, along with `KEEPALIVE_ACTIVITY_THRESHOLD`, the
`recentActivity` / `mostRecentActivity` helpers, and the `activityFn` test
seam. The ping fires on every window boundary.

A ping may occasionally be redundant — a task running across the boundary opens
the new window on its own — but the cost is one `reply with pong` round-trip
per 5 hours, far below the cost of a suppressed ping leaving the window shut.

The rate-limit skip in `keepalivePing` stays. On the scheduled path it is a
no-op by construction: `UsageMonitor.IsRateLimited()`
(`internal/runner/usage.go:105`) already returns "not limited" once `now` is
past `resets_at`, so a stale 100% reading from the closing window cannot
suppress the ping. On the cold-start fallback path, where `resets_at` is
unknown, it still prevents pinging into a wall.

### Scheduling rule

The naive `resets_at + delay` recomputation double-fires, for two reasons that
stack:

1. `claude-usage` is a cron-refreshed cache (~10 min), so after a ping the
   monitor keeps reporting the *old* `resets_at` for several minutes.
2. The existing guard against that (`internal/runner/keepalive.go:90`) tests
   whether the new target falls at or before `lastTarget`. That holds only
   because the old target is `resets_at - lead`. With the target moved to
   `resets_at + delay`, the recomputed target lands *after* `lastTarget`, the
   guard does not trigger, and the loop pings again one `KEEPALIVE_RESET_DELAY`
   later — into the window it just opened.

The fix drops target-ordering in favour of a direct claim about the window we
opened. Replace the `lastTarget` state with `lastPing` (the time of the last
ping) and compute:

```go
target := maxTime(resetsAt, lastPing.Add(keepaliveWindowLength)).Add(resetDelay)
```

Our own ping at `P` opened a window, so that window resets at `P + 5h`. That is
a more reliable figure than a `resets_at` we know may be stale, and it is what
the `max` selects while the cache lags. Once the monitor catches up, the two
branches agree and the loop re-syncs onto the authoritative value.

`lastPing` must be stamped **before** the ping runs, not after. `keepalivePing`
blocks on the `claude -p` subprocess for several seconds, but the window opens
when the request reaches the API, near the start of that call. Stamping
afterwards would fold the ping's own duration into every subsequent target,
putting each ping a few seconds further past the reset than intended. The error
does not accumulate across cycles either way, so this is about precision, not
correctness — but `lastPing` means "when we opened the window" and the code
should say so.

Two properties fall out for free:

- **Cold start:** `lastPing` is the zero time, so `lastPing + 5h` lands in year
  1 and `max` selects `resets_at`. No special case.
- **Stale `resets_at` after downtime:** if the box was off and `resets_at` is
  hours in the past, `max` selects `lastPing + 5h` — the window our
  catch-up ping actually opened — instead of waking early against a dead
  timestamp.

A zero `resets_at` is still handled first, before this rule, by the existing
fixed-interval fallback (`KEEPALIVE_INTERVAL`, 60m) with a zero target.

The fresh `time.Until(target)` computation is unchanged, but the clamp is not.
Today a delay below `keepaliveMinDelay` (1m) is rounded down to zero
(`internal/runner/keepalive.go:95`). That was harmless while the ping was meant
to land before the reset; now it is the expensive failure: a target 30 seconds
out would fire immediately, 30 seconds *before* `resets_at`, spending the ping
on the closing window and opening nothing.

`keepaliveMinDelay` is therefore removed and the clamp narrows to:

```go
if delay < 0 {
    delay = 0
}
```

A target in the past means the reset has already happened and pinging now is
correct; a target in the future is waited out exactly. The tight-loop
protection the threshold used to provide is now supplied by the `max`
projection, which never returns a target inside a window we already served.

### Worked trace

`R1` = current reset, `delay` = 2m, window = 5h.

| Step | `resets_at` reported | `lastPing` | target |
|---|---|---|---|
| Cold start | `R1` | zero | `R1 + 2m` |
| Ping at `T1 = R1 + 2m` | `R1` (cache lags) | `T1` | `max(R1, T1+5h) + 2m = T1 + 5h + 2m` |
| Ping at `T2 = T1 + 5h + 2m` | `R2 = T1 + 5h` (fresh) | `T2` | `max(R2, T2+5h) + 2m = T2 + 5h + 2m` |

At step 3 the true reset of the window opened at `T1` is `R2 = T1 + 5h`, and
the ping fires at `T2 = R2 + 2m` — two minutes after the reset, as intended.
Had the monitor instead already reported `R3 = T2 + 5h`, `max` returns the same
`T2 + 5h`; both branches agree.

## Changes

| File | Change |
|---|---|
| `internal/config/config.go` | `KeepaliveLeadTime` → `KeepaliveResetDelay` (`KEEPALIVE_RESET_DELAY`, default `2m`); drop `KeepaliveActivityThreshold` and its `KEEPALIVE_ACTIVITY_THRESHOLD` parse |
| `internal/runner/keepalive.go` | New scheduling rule; `lastTarget` → `lastPing`; add `maxTime` helper; narrow the clamp to `delay < 0` and delete `keepaliveMinDelay`; delete `recentActivity`, `mostRecentActivity`, and the activity branch of `keepalivePing`; update doc comments |
| `internal/runner/runner.go` | Delete the `activityFn` field (:130); update the `launchFn` comment (:135) that references it |
| `internal/runner/keepalive_test.go` | Delete the 5 activity-skip tests; rewrite the schedule tests |
| `.env.example`, `README.md`, `CLAUDE.md` | Document `KEEPALIVE_RESET_DELAY`; remove the two dropped variables |

## Testing

Unit tests only — the loop's dependencies (`resetsAtFn`, `pingFn`) are already
injectable seams, and no new I/O is introduced.

- `computeKeepaliveSchedule` targets `resets_at + delay` on cold start.
- Zero `resets_at` falls back to `KEEPALIVE_INTERVAL` with a zero target.
- A target already in the past clamps the delay to 0 (ping immediately).
- A target in the near future — inside what used to be the `keepaliveMinDelay`
  window, e.g. 30s out — is waited out and **not** fired early, since firing
  early would spend the ping on the closing window.
- **Regression, the point of the rewrite:** recomputing right after a ping,
  with the monitor still reporting the old `resets_at`, must project to
  `lastPing + 5h + delay` and must not yield a sub-window delay. This is the
  double-ping the design exists to prevent, and it is the test that would fail
  against a naive `resets_at + delay` implementation.
- The monitor reporting a fresh, post-ping `resets_at` yields the same target
  as the stale-cache branch.
- `keepalivePing` fires regardless of recent task or message activity (the
  inverse of the deleted tests).
- `keepalivePing` still skips when the runner is stopped and when
  `IsRateLimited()` reports true.

## Deployment

Build and test only. `KEEPALIVE_RESET_DELAY` has a working default, so no
`.env` change is required before restart; deploying and restarting the service
is the operator's step.
