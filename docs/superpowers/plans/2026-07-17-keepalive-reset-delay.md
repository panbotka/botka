# Keepalive Reset Delay Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fire the keepalive ping 2 minutes *after* the 5h window resets instead of 15 minutes before it, so a new window opens immediately and windows run back-to-back.

**Architecture:** Two independent changes to `internal/runner/keepalive.go` and its config. First, delete the activity-threshold check so the ping is unconditional. Second, retarget the schedule from `resets_at - lead` to `resets_at + delay` and replace the `lastTarget` ordering guard — which only worked for a *negative* offset — with `max(resets_at, lastPing + 5h)`, a projection off the window our own ping opened.

**Tech Stack:** Go 1.25, stdlib `testing`, golangci-lint v2.

**Spec:** `docs/superpowers/specs/2026-07-17-keepalive-reset-delay-design.md`

## Global Constraints

- **Never deploy or restart the service.** No `make deploy`, `make install-service`, `systemctl restart botka`, `systemctl stop botka`. A task agent runs *inside* Botka and would kill its own process. Build and test only.
- **Never run a second botka process** (`make run`, `go run ./cmd/server`) — two schedulers on one database run tasks concurrently.
- `make check` must pass before every commit (fmt + vet + lint + test + frontend type-check).
- Go tests run with the race detector (`make test`).
- Exact config values from the spec: `KEEPALIVE_RESET_DELAY` default `2m`. `KEEPALIVE_LEAD_TIME` (was `15m`) and `KEEPALIVE_ACTIVITY_THRESHOLD` (was `50m`) are both removed.
- The 5h window constant `keepaliveWindowLength = 5 * time.Hour` already exists in `internal/runner/keepalive.go:20` — reuse it, do not redefine.
- Doc comments are mandatory on every exported and unexported function (project Go style).

---

### Task 1: Make the keepalive ping unconditional

Delete the `KEEPALIVE_ACTIVITY_THRESHOLD` check and everything that fed it.

**Note on TDD:** this task is a pure deletion, so there is no red-green cycle to run — there is no seam that distinguishes "ping ignores activity" from "the activity code no longer exists". The proof is that `TestKeepalivePing_RunsWhenRunning` and `TestKeepalivePing_RunsWhenPaused` still pass with the activity code gone: they assert the ping fires, and nothing can now suppress it but the two intentional guards (stopped, rate-limited). Do not invent a mock to fake a cycle here.

**Files:**
- Modify: `internal/runner/keepalive.go:128-217` (activity branch of `keepalivePing`, `recentActivity`, `mostRecentActivity`)
- Modify: `internal/runner/runner.go:130` (`activityFn` field), `internal/runner/runner.go:134-135` (comment referencing it)
- Modify: `internal/config/config.go:44` (field), `:98-101` (parse), `:175` (assignment)
- Modify: `internal/runner/keepalive_test.go:161-261` (delete 5 tests)
- Modify: `.env.example:59`, `README.md:122`, `CLAUDE.md:219`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `keepalivePing()` retains its signature `func (r *Runner) keepalivePing()` and its two remaining guards (stopped, rate-limited). `Runner` no longer has an `activityFn` field. `config.Config` no longer has a `KeepaliveActivityThreshold` field.

- [ ] **Step 1: Delete the activity branch from `keepalivePing`**

In `internal/runner/keepalive.go`, replace the whole `keepalivePing` function (currently lines 128-171, doc comment included) with:

```go
// keepalivePing runs a minimal Claude Code session unless the runner is stopped
// or already rate limited. The ping is unconditional with respect to activity:
// it is scheduled to fire just after the 5h window resets, and a task or chat
// message from before that reset belongs to the closing window and cannot open
// the next one, so there is nothing for prior activity to make redundant.
func (r *Runner) keepalivePing() {
	r.mu.RLock()
	state := r.state
	r.mu.RUnlock()

	if state == models.StateStopped {
		slog.Debug("keepalive skipped: runner is stopped")
		return
	}

	if r.usageMon != nil {
		if limited, reason := r.usageMon.IsRateLimited(); limited {
			slog.Info("keepalive skipped: rate limited", "reason", reason)
			return
		}
	}

	if err := r.doPing(); err != nil {
		slog.Warn("keepalive ping failed", "error", err)
		return
	}
	slog.Info("keepalive ping completed")
}
```

- [ ] **Step 2: Delete `recentActivity` and `mostRecentActivity`**

In `internal/runner/keepalive.go`, delete both functions entirely (currently lines 173-217, doc comments included). They span from the comment `// recentActivity returns the most recent activity timestamp...` down to the closing brace before `// doPing executes the ping.`

- [ ] **Step 3: Drop the now-unused imports**

Those two functions were the only users of `database/sql`, `errors`, and `gorm.io/gorm` in this file. The import block at `internal/runner/keepalive.go:3-14` becomes:

```go
import (
	"context"
	"log/slog"
	"os/exec"
	"time"

	"botka/internal/models"
)
```

- [ ] **Step 4: Delete the `activityFn` seam from `Runner`**

In `internal/runner/runner.go`, delete line 130 (`activityFn func() (time.Time, error) ...`) and fix the `launchFn` comment below it, which names the seam being removed:

```go
	// launchFn, when non-nil, replaces launchTask. Test-only seam (like pingFn /
	// resetsAtFn) so force-run tests can assert launch behavior without spawning
	// a real Claude subprocess.
	launchFn func(task *models.Task, execution *models.TaskExecution) bool
```

- [ ] **Step 5: Drop `KeepaliveActivityThreshold` from config**

In `internal/config/config.go`, delete the struct field at line 44 (`KeepaliveActivityThreshold time.Duration`), the parse block at lines 98-101:

```go
	keepaliveActivityThreshold, err := time.ParseDuration(getEnv("KEEPALIVE_ACTIVITY_THRESHOLD", "50m"))
	if err != nil {
		return nil, fmt.Errorf("parsing KEEPALIVE_ACTIVITY_THRESHOLD: %w", err)
	}
```

and the assignment at line 175 (`KeepaliveActivityThreshold: keepaliveActivityThreshold,`).

- [ ] **Step 6: Delete the five obsolete tests**

In `internal/runner/keepalive_test.go`, delete these functions in full (lines 161-261):

- `TestKeepalivePing_SkipsOnRecentTaskActivity`
- `TestKeepalivePing_SkipsOnRecentMessageActivity`
- `TestKeepalivePing_RunsWhenActivityIsOld`
- `TestKeepalivePing_RunsWhenActivityCheckErrors`
- `TestKeepalivePing_RunsWhenTablesEmpty`

The first two assert the behavior this task deliberately reverses; the last three assert fail-open paths of code that no longer exists.

If `errors` is now unused in the test file's import block, drop it — `go build` will say so.

- [ ] **Step 7: Run the keepalive tests**

Run: `go test ./internal/runner/ -run 'TestKeepalivePing|TestKeepaliveLoop|TestDoPing' -race -v`
Expected: PASS. `TestKeepalivePing_RunsWhenRunning` and `TestKeepalivePing_RunsWhenPaused` passing is the evidence that the ping is now unconditional — nothing suppresses it but the stopped and rate-limited guards, each of which still has its own passing test (`TestKeepalivePing_SkipsWhenStopped`, `TestKeepalivePing_SkipsWhenRateLimited`).

- [ ] **Step 8: Remove the variable from the docs**

Delete the `KEEPALIVE_ACTIVITY_THRESHOLD` row from the env var table in `README.md` (line 122) and `CLAUDE.md` (line 219).

In `.env.example`, delete line 59 (`KEEPALIVE_ACTIVITY_THRESHOLD=50m`) and any comment line above it that describes the threshold.

- [ ] **Step 9: Run the full gate**

Run: `make check`
Expected: PASS. Watch for `unused` or `unparam` lint findings pointing at leftovers of the deleted code.

- [ ] **Step 10: Commit**

```bash
git add internal/runner/keepalive.go internal/runner/runner.go internal/runner/keepalive_test.go internal/config/config.go .env.example README.md CLAUDE.md
git commit -m "$(cat <<'EOF'
feat(runner): make the keepalive ping unconditional

KEEPALIVE_ACTIVITY_THRESHOLD suppressed the ping when a task or chat
message landed in the last 50 minutes, on the theory that real traffic
had already kept the window alive. That theory does not survive the ping
moving to just after resets_at: activity before the reset belongs to the
closing window and cannot open the next one, so a task running across the
boundary would suppress exactly the ping that matters.

Drop the check, the DB queries behind it, and the activityFn seam.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

### Task 2: Schedule the ping after the reset

Retarget the schedule and replace the projection guard.

**Files:**
- Modify: `internal/config/config.go:45` (field), `:103-106` (parse), `:176` (assignment)
- Modify: `internal/runner/keepalive.go:16-25` (constants), `:27-61` (`keepaliveLoop`), `:63-99` (`computeKeepaliveSchedule`)
- Modify: `internal/runner/keepalive_test.go:373-465`, `:467-486`, `:488-591`
- Modify: `.env.example:54,61`, `README.md:123`, `CLAUDE.md:220`

**Interfaces:**
- Consumes: `keepalivePing()` from Task 1 (unconditional, same signature).
- Produces: `config.Config.KeepaliveResetDelay time.Duration`. `computeKeepaliveSchedule(resetDelay, fallback time.Duration, lastPing time.Time) (time.Time, time.Duration)` — note the third parameter is now the time of the last **ping**, not the last target. `maxTime(a, b time.Time) time.Time`.

**Ordering note:** the config rename lands in Step 6, not first. `computeKeepaliveSchedule` takes the delay as a parameter, so its tests can go red and green while `keepaliveLoop` still reads the old `KeepaliveLeadTime` field. Renaming the field first would only break the build and turn the red step into a compile error, which proves nothing.

- [ ] **Step 1: Write the failing tests**

In `internal/runner/keepalive_test.go`, replace the six schedule tests (`TestComputeKeepaliveSchedule_FutureResetsAt` through `TestComputeKeepaliveSchedule_UsesFreshResetsAtAfterWindowAdvanced`, lines 373-486) with these six. Two are renamed because their premise inverted: `NearTargetPingsImmediately` becomes `NearTargetWaitsInsteadOfFiringEarly`, and `AdvancesToNextWindowAfterPing` becomes `ProjectsNextWindowWhileMonitorIsStale`.

```go
func TestComputeKeepaliveSchedule_FutureResetsAt(t *testing.T) {
	t.Parallel()

	resetsAt := time.Now().Add(45 * time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	target, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	wantTarget := resetsAt.Add(2 * time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target %v, got %v", wantTarget, target)
	}
	// Delay should be ~47min: 45min until the reset, plus the 2min delay after it.
	if delay < 46*time.Minute || delay > 48*time.Minute {
		t.Errorf("expected delay around 47m, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_PastTargetPingsImmediately(t *testing.T) {
	t.Parallel()

	// The window reset 5 minutes ago and we have not pinged for it — the target
	// is in the past, so open a window now.
	resetsAt := time.Now().Add(-5 * time.Minute)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	_, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	if delay != 0 {
		t.Errorf("expected immediate ping (delay=0), got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_NearTargetWaitsInsteadOfFiringEarly(t *testing.T) {
	t.Parallel()

	// The reset is 30s away, so the target is 2m30s away. The old
	// keepaliveMinDelay clamp would round any sub-minute delay down to zero;
	// under the new timing that fires the ping BEFORE the reset, spending it on
	// the closing window and opening nothing. The delay must be waited out.
	resetsAt := time.Now().Add(30 * time.Second)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	_, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	if delay < 2*time.Minute {
		t.Errorf("expected the delay to be waited out (>=2m), got %v — a ping this early lands before the reset", delay)
	}
}

func TestComputeKeepaliveSchedule_ZeroResetsAtUsesFallback(t *testing.T) {
	t.Parallel()

	r := &Runner{
		resetsAtFn: func() time.Time { return time.Time{} },
	}

	target, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, time.Time{})

	if !target.IsZero() {
		t.Errorf("expected zero target in fallback mode, got %v", target)
	}
	if delay != 60*time.Minute {
		t.Errorf("expected fallback delay 60m, got %v", delay)
	}
}

func TestComputeKeepaliveSchedule_ProjectsNextWindowWhileMonitorIsStale(t *testing.T) {
	t.Parallel()

	// We just pinged 2m after the reset, opening a new window. claude-usage is a
	// cron-refreshed cache, so the monitor keeps reporting the OLD resets_at for
	// several minutes. Recomputing must project to the window our own ping
	// opened (lastPing + 5h) instead of re-firing into it 2m later.
	resetsAt := time.Now().Add(-2 * time.Minute)
	lastPing := time.Now()
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	target, delay := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, lastPing)

	wantTarget := lastPing.Add(5*time.Hour + 2*time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target projected to the next window %v, got %v", wantTarget, target)
	}
	if delay < 5*time.Hour {
		t.Errorf("expected a ~5h delay, got %v — this is the double ping the projection exists to prevent", delay)
	}
}

func TestComputeKeepaliveSchedule_FreshResetsAtAgreesWithProjection(t *testing.T) {
	t.Parallel()

	// Same moment as the stale-cache case, except the monitor has caught up and
	// reports the reset of the window our ping opened. Both branches of the max
	// must land on the same target.
	lastPing := time.Now()
	resetsAt := lastPing.Add(5 * time.Hour)
	r := &Runner{
		resetsAtFn: func() time.Time { return resetsAt },
	}

	target, _ := r.computeKeepaliveSchedule(2*time.Minute, 60*time.Minute, lastPing)

	wantTarget := resetsAt.Add(2 * time.Minute)
	if !target.Equal(wantTarget) {
		t.Errorf("expected target %v, got %v", wantTarget, target)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/runner/ -race -run TestComputeKeepaliveSchedule`

Expected: FAIL — four of the six, with assertion errors, not a compile error. The old code reads the first parameter as a lead time and subtracts it:

- `FutureResetsAt`: old code targets `resets_at - 2m`; test wants `resets_at + 2m`.
- `NearTargetWaitsInsteadOfFiringEarly`: old code targets `resets_at - 2m`, already past, clamped to delay 0; test wants >= 2m.
- `ProjectsNextWindowWhileMonitorIsStale`: old guard projects to `lastTarget + 5h`; test wants `lastPing + 5h + 2m` — short by the delay.
- `FreshResetsAtAgreesWithProjection`: old code targets `resets_at - 2m`.

`PastTargetPingsImmediately` and `ZeroResetsAtUsesFallback` pass already — they assert behavior the rewrite preserves. That is expected, not a problem.

- [ ] **Step 3: Drop `keepaliveMinDelay`**

In `internal/runner/keepalive.go`, the const block at lines 16-25 becomes:

```go
const (
	keepaliveTimeout = 2 * time.Minute
	// keepaliveWindowLength is the assumed Anthropic 5-hour rate-limit window
	// length. Used to project the next ping time off the window our own ping
	// just opened, while the usage monitor's cache still reports the old one.
	keepaliveWindowLength = 5 * time.Hour
)
```

- [ ] **Step 4: Rewrite `computeKeepaliveSchedule`**

Replace the function and its doc comment (lines 63-99) with:

```go
// computeKeepaliveSchedule returns the target time of the next ping and the
// delay until then. The ping fires resetDelay after the 5h window resets, so
// the new window opens immediately rather than waiting for organic traffic.
//
// Behavior:
//   - If the reset time is unknown (zero), fall back to the fixed interval and
//     return a zero target — this is the cold-start path when the usage monitor
//     hasn't polled yet.
//   - Otherwise target the later of resetsAt and lastPing+5h, plus resetDelay.
//     Our own ping at lastPing opened a window that resets 5h later, which is a
//     more reliable figure than a resetsAt we know may be stale by up to the
//     claude-usage cache interval. Taking the later of the two keeps the loop
//     from firing a second time into the window it just opened, and re-syncs
//     onto the authoritative resetsAt once the monitor catches up. On cold start
//     lastPing is the zero time, so lastPing+5h lands in year 1 and resetsAt
//     wins — no special case needed.
//   - Only a target in the past collapses to a zero delay. A target in the near
//     future must be waited out: firing before resetsAt spends the ping on the
//     closing window and opens nothing, costing a full 5h window, whereas
//     firing late costs only the wait.
//
// Always computes the deadline freshly from time.Now() and the target to avoid
// timer drift across iterations.
func (r *Runner) computeKeepaliveSchedule(resetDelay, fallback time.Duration, lastPing time.Time) (time.Time, time.Duration) {
	resetsAt := r.currentResetsAt()
	if resetsAt.IsZero() {
		return time.Time{}, fallback
	}

	target := maxTime(resetsAt, lastPing.Add(keepaliveWindowLength)).Add(resetDelay)

	delay := time.Until(target)
	if delay < 0 {
		delay = 0
	}
	return target, delay
}

// maxTime returns the later of a and b.
func maxTime(a, b time.Time) time.Time {
	if b.After(a) {
		return b
	}
	return a
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/runner/ -race -run TestComputeKeepaliveSchedule -v`
Expected: PASS, all six. The package still compiles at this point: `keepaliveLoop` passes `r.config.KeepaliveLeadTime` into the `resetDelay` parameter, which is semantically wrong but type-correct. Step 6 fixes that.

- [ ] **Step 6: Rename the config field**

Steps 6 and 7 must land together — the tree does not compile between them, because `keepaliveLoop` reads the field being renamed.

In `internal/config/config.go`, change the struct field at line 45 from `KeepaliveLeadTime` to:

```go
	KeepaliveResetDelay        time.Duration
```

Replace the parse block at lines 103-106:

```go
	keepaliveResetDelay, err := time.ParseDuration(getEnv("KEEPALIVE_RESET_DELAY", "2m"))
	if err != nil {
		return nil, fmt.Errorf("parsing KEEPALIVE_RESET_DELAY: %w", err)
	}
```

and the assignment at line 176:

```go
		KeepaliveResetDelay:        keepaliveResetDelay,
```

- [ ] **Step 7: Rewrite `keepaliveLoop`**

Replace the function and its doc comment (lines 27-61) with:

```go
// keepaliveLoop periodically runs a minimal Claude Code session to keep the
// Anthropic API 5h rate limit window active. Runs in a dedicated goroutine
// alongside the scheduler loop and does not consume worker slots. Pings are
// scheduled to fire KEEPALIVE_RESET_DELAY after the current window resets, so
// the next window opens back-to-back with the one that just closed instead of
// waiting for organic traffic. When no usage data is available yet, falls back
// to the fixed-interval behavior driven by KEEPALIVE_INTERVAL.
func (r *Runner) keepaliveLoop(stopCh <-chan struct{}) {
	defer r.wg.Done()

	resetDelay := r.config.KeepaliveResetDelay
	fallback := r.config.KeepaliveInterval

	slog.Info("keepalive loop started", "reset_delay", resetDelay, "fallback_interval", fallback)

	var lastPing time.Time
	target, delay := r.computeKeepaliveSchedule(resetDelay, fallback, lastPing)
	logKeepaliveSchedule(target, delay)
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-stopCh:
			slog.Info("keepalive loop stopped")
			return
		case <-timer.C:
			// Stamp before the ping, not after: the window opens when the request
			// reaches the API, near the start of keepalivePing, which then blocks
			// on the subprocess for several seconds. Stamping afterwards would
			// fold that duration into every subsequent target.
			lastPing = time.Now()
			r.keepalivePing()
			target, delay = r.computeKeepaliveSchedule(resetDelay, fallback, lastPing)
			logKeepaliveSchedule(target, delay)
			timer.Reset(delay)
		}
	}
}
```

- [ ] **Step 8: Fix the loop tests' config field**

Three loop tests still set the removed `KeepaliveLeadTime` field and will not compile. In `internal/runner/keepalive_test.go`, in `TestKeepaliveLoop_FallbackWhenResetsAtZero` (line ~494), `TestKeepaliveLoop_PingsImmediatelyWhenTargetInPast` (line ~526), and `TestKeepaliveLoop_StopsCleanlyWhileWaitingOnLongDelay` (line ~558), change:

```go
		KeepaliveLeadTime: 15 * time.Minute,
```

to:

```go
		KeepaliveResetDelay: 2 * time.Minute,
```

In `TestKeepaliveLoop_PingsImmediatelyWhenTargetInPast`, also update the stale comment, since the target is now past because `resetsAt` itself is past rather than because the lead time pushed it there:

```go
	// Fixed past resetsAt — the target (resetsAt + 2m) is already behind us, so
	// the first ping is immediate. After that ping the loop projects to the
	// window it just opened (lastPing + 5h), so we must not see a flood.
```

- [ ] **Step 9: Run the whole runner suite**

Run: `go test ./internal/runner/ -race`
Expected: PASS (~all runner tests). `TestKeepaliveLoop_PingsImmediatelyWhenTargetInPast` asserting exactly 1 ping is the loop-level guard against the double ping.

- [ ] **Step 10: Update the docs**

In `.env.example`, replace the keepalive comment at line 54 and the `KEEPALIVE_LEAD_TIME=15m` line at 61:

```
# How long after the 5h window's resets_at to fire the keepalive ping. The ping
# opens the next window, so it must land AFTER the reset — firing early spends
# it on the closing window and costs a full 5h window.
KEEPALIVE_RESET_DELAY=2m
```

In `README.md`, replace the `KEEPALIVE_LEAD_TIME` row (line 123):

```
| `KEEPALIVE_RESET_DELAY` | `2m` | How long after the current 5h window's `resets_at` to fire the keepalive ping, opening the next window immediately |
```

In `CLAUDE.md`, replace the `KEEPALIVE_LEAD_TIME` row (line 220):

```
| `KEEPALIVE_RESET_DELAY` | `2m` | How long after the current 5h window's `resets_at` to fire the keepalive ping. Anthropic's 5h windows are anchored at first use, so the ping must land *after* the reset to open the next window; the loop then projects the following ping off `lastPing + 5h` while the `claude-usage` cache still reports the old `resets_at` (Go duration) |
```

- [ ] **Step 11: Run the full gate**

Run: `make check`
Expected: PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/runner/keepalive.go internal/runner/keepalive_test.go internal/config/config.go .env.example README.md CLAUDE.md
git commit -m "$(cat <<'EOF'
fix(runner): ping keepalive after the 5h window resets, not before

The ping fired at resets_at - 15m, inside the window that was already
closing. Anthropic's 5h windows are anchored at first use rather than
sliding, so that ping opened nothing: the window still reset on schedule
and the next one began only when organic traffic arrived. The feature has
never done what it was built to do.

Fire at resets_at + KEEPALIVE_RESET_DELAY (2m) instead, so windows run
back-to-back. Two guards had to change with it:

  - The lastTarget ordering guard only suppressed a repeat ping because
    the old target sat before resets_at. With the target after it, the
    guard never triggered and the loop pinged again a delay later, into
    the window it had just opened. Project off max(resets_at, lastPing+5h)
    instead — our own ping is a better clock than a cache that lags by up
    to 10 minutes, and the two agree once it catches up.
  - keepaliveMinDelay rounded any sub-minute delay to zero, which now
    means firing before the reset. Only a past target collapses to zero.

Co-Authored-By: Claude Opus 4.8 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Verification

After both tasks, the runner is not exercised end-to-end by this plan: the real ping fires once per 5 hours against the live API, and the service must not be restarted from inside a task agent. The unit tests are the verification, and `TestKeepaliveLoop_PingsImmediatelyWhenTargetInPast` covers the loop wiring with a real timer and goroutine.

Report to the user that a deploy + `systemctl restart botka` is needed for the change to take effect, and that `KEEPALIVE_RESET_DELAY` has a working default so no `.env` edit is required first. The old `KEEPALIVE_LEAD_TIME` / `KEEPALIVE_ACTIVITY_THRESHOLD` entries, if present in the live `.env`, become inert — `config.Load` ignores unknown keys, so a stale `.env` will not fail startup.
