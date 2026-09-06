# Box Auto-Off (automatic shutdown of the idle Box build machine)

**Status:** Approved
**Date:** 2026-09-06

## Problem

Box — the x86_64 build machine (`BOX_HOST`, woken over Wake-on-LAN) — is
started on demand by Botka (`internal/box.Waker.EnsureUp`) and by the user,
but nothing ever turns it off. Botka only offers a manual "Shutdown" button
on the Box page (`POST /api/v1/box/shutdown`). In practice the machine is
regularly left running for days — a weekend of a 12-core / 24-thread RTX 3070 desktop
idling at the wall.

The goal is not minute-level accuracy. It is to eliminate the "Box ran the
whole weekend for nothing" case, without ever cutting power to work in
progress.

## Scope

**In scope**

- A server-side setting `box_auto_off` that arms an automatic shutdown.
- A background monitor that evaluates a fixed set of idleness conditions and
  powers Box down when they hold for long enough.
- An API and a Box-page card exposing the current evaluation — including
  *why* a shutdown did not happen — plus a log of automatic shutdowns.

**Out of scope**

- Automatic wake. `box.Waker` already wakes Box when a task or chat needs it;
  auto-off deliberately does not try to be clever about waking it back.
- Per-user schedules or time windows ("never shut down on weekdays").
- Blocking on interactive SSH logins. A stale `who` entry from a forgotten
  terminal would block auto-off indefinitely, which is exactly the failure
  this feature exists to remove. CPU load and GPU utilization cover real
  interactive work.
- Notifications. The UI card and the server log are the whole report.

## Signals

Measured on the live system on 2026-09-06 before writing this spec:

| Signal | Verdict |
|---|---|
| `https://fotky.kotrzina.cz/metrics` (production Kukátko, Hetzner VPS) | Usable. Public, unauthenticated, 100 kB of Prometheus text, exposes `kukatko_jobs_queue_depth{state=…}`. Job states are `queued` and `running` (`kukatko/internal/jobs/models.go`). |
| Embeddings service on Box `:8000` | **Unusable.** `/metrics` returns 404; the FastAPI app exposes only `/health`, `/embed/*`, `/estimate/*`. |
| `nvidia-smi --query-compute-apps` | **Unusable as a busy signal.** The two idle systemd services hold 6.3 GB of VRAM permanently (photo-enhancer 3442 MiB, image-embeddings 2834 MiB), so a compute process is always present. |
| `nvidia-smi --query-gpu=utilization.gpu` | Usable, but only single-shot: combining `--query-gpu` with `-l/-c` fails with *"Option --query-gpu=utilization.gpu is not recognized"*. Sampling must be a shell loop. |
| `/proc/uptime`, `/proc/loadavg` over SSH as `panbotka` | Usable, no sudo. Idle Box measured at load1 `0.00`–`0.45` on 24 hardware threads. |

All Box-side data therefore comes from **one** SSH round-trip:

```sh
cut -d' ' -f1 /proc/uptime      # line 1: uptime seconds
cut -d' ' -f1 /proc/loadavg     # line 2: 1-minute load average
nproc                           # line 3: hardware threads (context for the UI)
for i in 1 2 3; do              # lines 4+: one GPU utilization sample per GPU
  nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits
  sleep 1
done
```

## Behavior

Every `BOX_AUTO_OFF_INTERVAL` (default 10 m) the monitor evaluates eight
conditions in order. Each produces a `Blocker` with a machine name and a
human-readable detail string.

Conditions 1 and 2 are **terminal**: if the feature is off, or Box is not
reachable, there is nothing to measure and the evaluation stops there
(saving an SSH round-trip and an HTTPS scrape). Conditions 3–8 are all
evaluated even once one of them has blocked, so the card shows the full
picture rather than whichever check happened to fail first.

| # | Name | Blocks when |
|---|---|---|
| 1 | `disabled` | `box_auto_off` is off. Recorded, then evaluation stops — no SSH, no scrape. |
| 2 | `box_offline` | The SSH probe fails. Box is already down (or unreachable); nothing to do. |
| 3 | `uptime` | `/proc/uptime` < `BOX_AUTO_OFF_MIN_UPTIME` (default 2 h). |
| 4 | `botka_tasks` | `Runner.GetStatus().ActiveTasks` is non-empty — *any* task, not only Box ones. |
| 5 | `botka_box_chats` | A chat subprocess in `claude.Registry` belongs to a thread whose project path starts with `box:`. |
| 6 | `kukatko_queue` | `kukatko_jobs_queue_depth{state="queued"} + {state="running"} > 0`, **or** the scrape did not yield a trustworthy answer. |
| 7 | `gpu` | max GPU utilization across the samples > `BOX_AUTO_OFF_GPU_THRESHOLD` (default 10 %). |
| 8 | `cpu` | load1 > `BOX_AUTO_OFF_LOAD_THRESHOLD` (default 1.0 — one fully busy thread out of 24). |

**Streak.** A clean evaluation increments a counter; any blocker resets it to
zero. Shutdown fires at `BOX_AUTO_OFF_IDLE_CHECKS` (default 3) — 30 minutes
of continuous quiet at the default interval. This is what bridges the gap
between two batches of Kukátko jobs, and what makes a single unlucky GPU
sample harmless.

**Countdown.** The evaluation carries `earliest_shutdown_at`, the soonest
moment a shutdown could happen if nothing changes. It is computed by a pure
function alongside the blocker list, so the UI only has to tick a clock:

- **No blockers.** Ticks land at `next_check_at + k·interval`, and with a
  streak of `s` the run needs `required − s` more clean ticks:
  `earliest = next_check_at + (required − s − 1)·interval`. At `s = 2` of 3
  that is the next tick itself.
- **`uptime` is the only blocker.** This one blocker has a known expiry, so
  the countdown stays meaningful: take the first tick at or after
  `now + (min_uptime − uptime)`, then add `(required − 1)·interval`. A Box
  booted five minutes ago therefore shows a real ~2 h 30 min countdown
  instead of a blank.
- **Anything else blocks** — disabled, offline, tasks, chats, Kukátko, CPU,
  GPU — `earliest_shutdown_at` is null. Those blockers have no predictable
  expiry, and a countdown that keeps resetting is worse than none. The UI
  shows the blocking reason instead.

**Re-check before firing.** Conditions 4 and 5 are re-evaluated immediately
before the shutdown command is issued. The evaluation involves an SSH probe
and an HTTPS scrape and takes seconds, during which the runner can claim a
task; these two checks are local and cheap, so there is no reason to act on
stale readings. A blocker found here aborts the shutdown and resets the
streak.

**Failure is always fail-safe.** Every unknown blocks: an SSH probe that
errors or returns unparsable output, a scrape that fails, a `nvidia-smi`
that is missing. The one thing the monitor never does is shut down on
incomplete information.

**Kukátko scrape trustworthiness.** A 200 response is not enough. The parser
requires at least one `kukatko_jobs_queue_depth{` sample to be present in the
body before it believes a `queued+running` sum of zero. Without that guard, a
renamed metric, a captive-portal HTML page, or an app that has not registered
its collectors would all read as "queue empty" and license a shutdown. The
HTTP status code and — on failure — the transport error or the first 200
bytes of the body are kept in the evaluation for the UI, per the requirement
that the reason be visible with the response.

## Components

### `internal/boxoff` (new)

Split so each file has one job and the decision logic is testable with no
network, no database and no Box:

- `evaluate.go` — two pure functions.
  `Evaluate(now time.Time, in Inputs, cfg Thresholds) Evaluation` takes
  already-gathered readings and returns the blocker list.
  `EarliestShutdownAt(now time.Time, ev Evaluation, streak int, nextCheck time.Time, cfg Thresholds) *time.Time`
  turns that into the countdown described above. No I/O in either.
- `probe.go` — `SSHProbe` runs the command above via a `CommandRunner` and
  parses stdout into `BoxReadings{UptimeSeconds, Load1, Cores, GPUSamples}`.
  Parsing is a separate exported function so malformed output is testable
  directly. Multiple GPUs are handled by taking the max over every sample
  line.
- `kukatko.go` — `MetricsClient.Fetch(ctx) (KukatkoReadings, error)`: HTTP GET
  with a 10 s timeout, minimal Prometheus text parsing of the one metric
  family, plus the `status_code` / `error` / `body_excerpt` fields the UI
  needs.
- `activity.go` — `AppActivity` adapts `*runner.Runner`, `claude.Registry`
  and the database to the `ActivitySource` interface
  (`RunningTasks() []string`, `BoxChatThreads(ctx) ([]string, error)`). The
  Box-chat query joins the registry's thread IDs against
  `threads ⨝ projects` with `projects.path LIKE 'box:%'`, and is skipped
  entirely when the registry is empty.
- `monitor.go` — the loop: ticker, setting read, streak, re-check, shutdown,
  in-memory ring buffer of the last 20 evaluations, event persistence. Owns
  a `sync.RWMutex` guarding the ring and the streak so the HTTP handler can
  read them.

The monitor takes its collaborators as interfaces (`Prober`,
`KukatkoSource`, `ActivitySource`, `Shutdowner`), so its tests drive the
whole lifecycle with fakes.

### `internal/box` (extended)

`BoxHandler.Shutdown` currently owns the non-obvious part of powering Box
down: `sudo shutdown now` kills sshd mid-session, so ssh exits non-zero on
success, and `isExpectedShutdownDisconnect` separates that from a real
auth/connection failure. The monitor needs exactly the same behavior.
Duplicating a 40-line heuristic whose whole point is "do not report success
when it failed" is the wrong move, so it moves down into `internal/box`:

```go
func Shutdown(ctx context.Context, run RunFunc, sshTarget string) error
```

with `isExpectedShutdownDisconnect` and its two marker tables coming along.
`handlers.BoxHandler.Shutdown` becomes a thin caller that maps the error to
HTTP 500, keeping its current target (`user@BOX_HOST`); the monitor calls the
same function with the `Waker`'s target. The existing tests in
`internal/handlers/box_test.go` that cover the disconnect heuristic move to
`internal/box` with it.

### Persistence

- **Setting.** `box_auto_off` is a row in `app_settings` (`"true"`/`"false"`,
  default off when absent). No migration — `app_settings` is a key/value
  table that already exists.
- **Events.** Migration `042_box_auto_off_events` creates:

  ```sql
  CREATE TABLE box_auto_off_events (
      id          BIGSERIAL PRIMARY KEY,
      occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
      outcome     TEXT        NOT NULL,   -- 'shutdown' | 'failed' | 'aborted'
      detail      JSONB       NOT NULL    -- readings + blockers at the moment of the attempt
  );
  CREATE INDEX idx_box_auto_off_events_occurred_at ON box_auto_off_events (occurred_at DESC);
  ```

  Only actual shutdown *attempts* are written — a handful per week, not the
  144 evaluations a day. `aborted` is the re-check catching late work,
  `failed` is SSH refusing.
- **Live state** (streak, last 20 evaluations) is in memory only. It is
  transient by nature and refills within one interval after a restart; the
  first tick runs 30 s after startup rather than a full interval later, so
  the card is not empty for 10 minutes.

### REST API

`GET /api/v1/box/auto-off` (registered in `RegisterBoxRoutes`):

```json
{"data": {
  "enabled": true,
  "streak": 1,
  "required_checks": 3,
  "interval_seconds": 600,
  "next_check_at": "2026-09-06T20:10:00Z",
  "earliest_shutdown_at": null,
  "last_evaluation": {
    "checked_at": "2026-09-06T20:00:00Z",
    "idle": false,
    "blockers": [{"name": "kukatko_queue", "detail": "12 queued, 1 running"}],
    "box": {"uptime_seconds": 33390, "load1": 0.0, "cores": 24, "gpu_max_percent": 0},
    "kukatko": {"status_code": 200, "queued": 12, "running": 1, "error": null, "body_excerpt": null}
  },
  "recent": [ /* up to 20 evaluations, newest first */ ],
  "events": [ /* up to 20 rows from box_auto_off_events, newest first */ ]
}}
```

The toggle rides the existing settings endpoint rather than growing a second
write path: `PUT /api/v1/settings` gains `box_auto_off *bool`, and
`SettingsHandler.Get` returns it as a real boolean (the same special-casing
`max_workers` already gets for integers). `SettingsHandler.SetOnChange` is
already wired in `main.go`; the callback tells the monitor to re-read the
setting immediately, so arming it does not wait for the next tick.

### Frontend

One card on `BoxPage`, below the existing service list, next to the manual
Shutdown button it complements:

- Header with the toggle (optimistic switch, `updateServerSettings`).
- When enabled and a countdown exists: the headline is
  "Vypnutí za 27:14", ticking once a second off `earliest_shutdown_at`,
  with "pokud zůstane klid" beneath it so nobody reads it as a promise.
  When `earliest_shutdown_at` is null the same slot names the blocker
  instead ("Blokuje: fronta Kukátka — 12 queued").
- "Další kontrola za 7 min" and the streak as `2 / 3 klidných kontrol`.
- The condition list, each row ✓/✗ with its detail: uptime, load1 with core
  count, GPU max %, Kukátko queue (or its HTTP status and error text), and
  the Botka-side task/chat blockers.
- A collapsed log of the last automatic shutdowns (time + outcome).

Data comes from a `useBoxAutoOff` hook polling the endpoint on the same
cadence the page already polls Box status; no SSE.

## Configuration

| Variable | Default | Description |
|---|---|---|
| `BOX_AUTO_OFF_INTERVAL` | `10m` | Evaluation interval |
| `BOX_AUTO_OFF_MIN_UPTIME` | `2h` | Minimum Box uptime before it may be shut down |
| `BOX_AUTO_OFF_IDLE_CHECKS` | `3` | Consecutive clean evaluations required |
| `BOX_AUTO_OFF_GPU_THRESHOLD` | `10` | GPU utilization %, above which Box counts as busy |
| `BOX_AUTO_OFF_LOAD_THRESHOLD` | `1.0` | 1-minute load average, above which Box counts as busy |
| `KUKATKO_METRICS_URL` | `https://fotky.kotrzina.cz/metrics` | Production Kukátko metrics endpoint |

The master switch is the `box_auto_off` setting, not an env var: it is a
thing the user flips, not a deployment parameter.

## Files Touched

**New**

- `internal/boxoff/{evaluate,probe,kukatko,activity,monitor}.go` + tests
- `migrations/042_box_auto_off_events.{up,down}.sql`
- `internal/models/box_auto_off_event.go`
- `frontend/src/components/BoxAutoOffCard.tsx` + test
- `frontend/src/hooks/useBoxAutoOff.ts`

**Modified**

- `internal/box/box.go` — gains `Shutdown` + the disconnect heuristic
- `internal/handlers/box.go` — `Shutdown` delegates; new `AutoOff` endpoint
- `internal/handlers/settings.go` — `box_auto_off` read/write
- `internal/config/config.go` — the six new variables
- `cmd/server/main.go` — construct and start the monitor, stop it on shutdown
- `frontend/src/api/client.ts`, `frontend/src/types/index.ts`,
  `frontend/src/pages/BoxPage.tsx`
- `CLAUDE.md` — env var table and an "Important Patterns" entry

## Testing

- **`Evaluate`** — table-driven over every blocker and every combination
  boundary: uptime just under/over 2 h, load and GPU at the threshold,
  queue of exactly zero, unknown readings. Pure function, no fixtures.
- **`EarliestShutdownAt`** — clean evaluation at streak 0/1/2 of 3; the
  uptime-only blocker on a freshly booted Box (countdown crosses the 2 h
  mark and adds two intervals); every other blocker returning nil; a
  disabled monitor returning nil.
- **Probe parsing** — the happy 6-line output; multi-GPU output; truncated
  output; `nvidia-smi` error text on stdout; non-numeric fields. Each must
  produce an error rather than a zero reading.
- **Kukátko parsing** — real metric lines lifted from the live endpoint; a
  body with `state="done"` only (→ 0, trusted); a body with no
  `kukatko_jobs_queue_depth` family at all (→ error, not zero); an HTML error
  page with status 200; a 502.
- **Monitor lifecycle** — fakes for all four collaborators: streak reaching
  the threshold fires exactly one shutdown; a blocker mid-streak resets it;
  the disabled setting skips probing entirely; a shutdown error is recorded
  as `failed` and does not reset the world; the pre-shutdown re-check aborts
  and records `aborted`.
- **`internal/box.Shutdown`** — the existing disconnect-heuristic cases,
  moved.
- **Handlers** — `GET /box/auto-off` shape; `PUT /settings` accepting and
  echoing `box_auto_off`.
- **Frontend** — the card renders blockers and the Kukátko error detail;
  toggling calls the settings API.

`make check` must pass before the branch is proposed for merge.

## Risks

- **A permanently loaded Box never auto-offs.** If some daemon keeps load1
  above 1.0 or the GPU above 10 %, the feature silently does nothing. It is
  not actually silent: the card names the blocking condition with its
  measured value on every evaluation, and both thresholds are env-tunable.
- **Shutting down under work Botka cannot see.** A manual `ssh box` build is
  visible as CPU or GPU load, and the 30-minute streak means a shutdown needs
  half an hour of genuine quiet. A compile that idles for 30 minutes mid-way
  is not a scenario worth defending against; Box wakes on demand.
- **Racing `box.Waker`.** A task claimed moments after shutdown will call
  `EnsureUp`, which sends Wake-on-LAN and waits up to 60 s. The result is a
  slower start, not a failure.
