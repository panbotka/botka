# Box Auto-Off Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Shut the Box build machine down automatically once it has been up for two hours with no Botka tasks, no Box chat sessions, an empty Kukátko job queue, and idle CPU and GPU.

**Architecture:** A new `internal/boxoff` package owns a ticker goroutine that gathers three readings (one SSH probe of Box, one HTTPS scrape of Kukátko, one local look at Botka's own work), feeds them to a pure `Evaluate` function, and shuts Box down after `IdleChecks` consecutive clean evaluations. The existing shutdown-over-SSH logic moves from `internal/handlers/box.go` down into `internal/box` so the HTTP handler and the monitor share it. State the UI needs is exposed at `GET /api/v1/box/auto-off`; the on/off switch is an `app_settings` row written through the existing settings endpoint.

**Tech Stack:** Go 1.25 (stdlib `net/http`, `os/exec`, `log/slog`), Gin, GORM + golang-migrate on PostgreSQL 17, React 19 + TypeScript + Tailwind 4, Vitest.

**Spec:** `docs/superpowers/specs/2026-09-06-box-auto-off-design.md`

## Global Constraints

- **`make check` must pass before every commit.** It runs `fmt` + `vet` + `golangci-lint` + `go test -race` + frontend type-check. Do not commit code that fails it.
- **Never deploy.** `make deploy`, `make install-service`, `systemctl restart botka`, `systemctl stop botka` kill the process this may be running inside. Build and test only.
- **Never start a second botka process** (`make run`, `go run ./cmd/server`) while the systemd service is running.
- **Fail-safe is the rule.** Every unknown reading — SSH error, unparsable output, failed scrape, failed DB query — must produce a *blocker*, never a shutdown.
- **Linters are strict** (errcheck, gocritic, revive, staticcheck, misspell, bodyclose, unconvert, whitespace, predeclared). Every exported symbol needs a doc comment starting with its name. Every `resp.Body` needs a closed body. Every returned error is checked.
- **Blocker details are language-neutral facts** ("12 queued, 1 running", "load1 3.20 > 1.00"). Czech labels live in the frontend, keyed off `Blocker.Name`.
- Default values, verbatim: interval `10m`, min uptime `2h`, idle checks `3`, GPU threshold `10`, load threshold `1.0`, metrics URL `https://fotky.kotrzina.cz/metrics`.

---

## File Structure

**New Go files**

| File | Responsibility |
|---|---|
| `internal/boxoff/types.go` | Shared value types: `Blocker`, `BoxReadings`, `KukatkoReadings`, `Inputs`, `Thresholds`, `Evaluation`, blocker name constants |
| `internal/boxoff/evaluate.go` | Pure decision logic: `Evaluate`, `EarliestShutdownAt` |
| `internal/boxoff/probe.go` | `SSHProbe` + `ParseProbeOutput` — the one SSH round-trip |
| `internal/boxoff/kukatko.go` | `MetricsClient` + `ParseQueueDepth` — the HTTPS scrape |
| `internal/boxoff/activity.go` | `AppActivity` — Botka's own running tasks and Box chats |
| `internal/boxoff/monitor.go` | The loop: ticker, streak, re-check, shutdown, ring buffer, event writes |
| `internal/models/box_auto_off_event.go` | `BoxAutoOffEvent` GORM model |
| `migrations/042_box_auto_off_events.{up,down}.sql` | The events table |

**Modified Go files**

| File | Change |
|---|---|
| `internal/box/box.go` | Gains `Shutdown` + the disconnect heuristic moved from handlers |
| `internal/handlers/box.go` | `Shutdown` delegates to `box.Shutdown`; new `AutoOff` endpoint |
| `internal/handlers/settings.go` | `box_auto_off` read + write |
| `internal/config/config.go` | Six new fields |
| `cmd/server/main.go` | Construct, start and stop the monitor; wire the settings callback |

**Frontend**

| File | Change |
|---|---|
| `frontend/src/types/index.ts` | `BoxAutoOffStatus` and friends; `ServerSettings.box_auto_off` |
| `frontend/src/api/client.ts` | `fetchBoxAutoOff` |
| `frontend/src/hooks/useBoxAutoOff.ts` | Poll + live countdown tick |
| `frontend/src/components/BoxAutoOffCard.tsx` | The card |
| `frontend/src/pages/BoxPage.tsx` | Render the card |

---

### Task 1: Move the shutdown heuristic into `internal/box`

The `sudo shutdown now` path is subtle: powering off kills sshd mid-session, so `ssh` exits non-zero *on success*, and `isExpectedShutdownDisconnect` is what separates that from a real auth or connection failure. The monitor needs identical behavior, and duplicating a heuristic whose whole purpose is "never report success when it failed" is the wrong move. Move it down; leave the HTTP handler a thin caller.

**Files:**
- Modify: `internal/box/box.go`
- Modify: `internal/handlers/box.go` (delete lines covering `sshTransportExitCode`, `shutdownDisconnectMarkers`, `sshFailureMarkers`, `exitCoder`, `isExpectedShutdownDisconnect`, `shutdownErrorMessage`; rewrite `Shutdown`)
- Modify: `internal/handlers/box_test.go` (delete `TestIsExpectedShutdownDisconnect_NilError`)
- Test: `internal/box/shutdown_test.go` (new)

**Interfaces:**
- Consumes: nothing.
- Produces: `box.RunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)`; `box.Shutdown(ctx context.Context, run RunFunc, sshTarget string) error`. Task 8 calls `Shutdown`; Task 11 wires the target.

- [ ] **Step 1: Write the failing test**

Create `internal/box/shutdown_test.go`:

```go
package box

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeExitError carries a specific process exit status, like *exec.ExitError.
type fakeExitError struct{ code int }

func (e *fakeExitError) Error() string { return "exit status" }
func (e *fakeExitError) ExitCode() int { return e.code }

func TestShutdown(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		err     error
		wantErr bool
	}{
		{name: "clean exit", output: "", err: nil, wantErr: false},
		{name: "disconnect is success", output: "Connection to box closed by remote host.", err: &fakeExitError{code: 255}, wantErr: false},
		{name: "reset by peer is success", output: "client_loop: send disconnect: Connection reset by peer", err: &fakeExitError{code: 255}, wantErr: false},
		{name: "permission denied fails", output: "Permission denied (publickey).", err: &fakeExitError{code: 255}, wantErr: true},
		{name: "sudo password fails", output: "sudo: a password is required", err: &fakeExitError{code: 255}, wantErr: true},
		{name: "relayed exit status fails", output: "Connection closed by remote host", err: &fakeExitError{code: 1}, wantErr: true},
		{name: "unrecognized output fails", output: "something odd happened", err: &fakeExitError{code: 255}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotArgs []string
			run := func(_ context.Context, name string, args ...string) ([]byte, error) {
				gotArgs = append([]string{name}, args...)
				return []byte(tt.output), tt.err
			}

			err := Shutdown(context.Background(), run, "panbotka@box")

			if tt.wantErr && err == nil {
				t.Fatalf("Shutdown() = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Shutdown() = %v, want nil", err)
			}
			if tt.wantErr && tt.output != "" && !strings.Contains(err.Error(), tt.output) {
				t.Errorf("error %q does not carry the ssh output %q", err, tt.output)
			}
			joined := strings.Join(gotArgs, " ")
			for _, want := range []string{"ssh", "panbotka@box", "sudo", "shutdown", "now"} {
				if !strings.Contains(joined, want) {
					t.Errorf("command %q missing %q", joined, want)
				}
			}
		})
	}
}

func TestShutdown_TimeoutIsAlwaysFailure(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("Connection closed by remote host"), errors.New("signal: killed")
	}

	if err := Shutdown(ctx, run, "panbotka@box"); err == nil {
		t.Fatal("Shutdown() with a cancelled context = nil, want error")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/box/ -run TestShutdown -v`
Expected: FAIL — `undefined: Shutdown`.

- [ ] **Step 3: Move the implementation**

Append to `internal/box/box.go` (imports gain `os/exec` is already there; add `errors` and `strings` if missing). Copy the four markers/constants and `isExpectedShutdownDisconnect` **verbatim** from `internal/handlers/box.go` (including their doc comments — they explain the exit-code reasoning and must survive the move), then add:

```go
// RunFunc runs a command and returns its combined output. It matches the
// signature of the command runners used by the box handler and the auto-off
// monitor, so both can pass their own (mockable) implementation.
type RunFunc func(ctx context.Context, name string, args ...string) ([]byte, error)

// Shutdown powers the Box off over SSH.
//
// A failing ssh invocation is reported as an error unless the failure is the
// connection teardown caused by the machine actually powering off — see
// isExpectedShutdownDisconnect. A cancelled or timed-out context is always a
// failure: the command never disconnected cleanly, so nothing can be inferred
// from its output.
func Shutdown(ctx context.Context, run RunFunc, sshTarget string) error {
	output, err := run(ctx, "ssh", "-o", "StrictHostKeyChecking=no", "-o", "ConnectTimeout=10", sshTarget, "sudo", "shutdown", "now")
	if err == nil {
		return nil
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		return shutdownError(fmt.Errorf("%w: %w", ctxErr, err), output)
	}
	if !isExpectedShutdownDisconnect(err, string(output)) {
		return shutdownError(err, output)
	}
	return nil
}

// shutdownError builds an error carrying both the cause and whatever SSH
// printed, so callers can show why the shutdown failed.
func shutdownError(err error, output []byte) error {
	if out := strings.TrimSpace(string(output)); out != "" {
		return fmt.Errorf("shutdown failed: %w: %s", err, out)
	}
	return fmt.Errorf("shutdown failed: %w", err)
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/box/ -run TestShutdown -v`
Expected: PASS.

- [ ] **Step 5: Rewrite the handler to delegate**

In `internal/handlers/box.go`, delete `sshTransportExitCode`, `shutdownDisconnectMarkers`, `sshFailureMarkers`, `exitCoder`, `isExpectedShutdownDisconnect` and `shutdownErrorMessage`, and replace the body of `Shutdown` with:

```go
// Shutdown sends a shutdown command to the box via SSH. The SSH-level details
// (a poweroff tears the session down, so ssh legitimately fails on success)
// live in internal/box, shared with the auto-off monitor.
func (h *BoxHandler) Shutdown(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 30*time.Second)
	defer cancel()

	if err := box.Shutdown(ctx, h.runner.Run, fmt.Sprintf("%s@%s", h.sshUser, h.host)); err != nil {
		respondError(c, http.StatusInternalServerError, err.Error())
		return
	}

	respondOK(c, gin.H{"message": "shutdown command sent"})
}
```

Add `"botka/internal/box"` to the imports; drop `"errors"` and `"strings"` if they become unused (`go vet` will say so). Delete `TestIsExpectedShutdownDisconnect_NilError` from `internal/handlers/box_test.go` — its subject moved.

- [ ] **Step 6: Verify nothing regressed**

Run: `go build ./... && go test ./internal/box/ ./internal/handlers/ -run 'Box|Shutdown' -v`
Expected: PASS, including the pre-existing `TestBoxHandler_Shutdown_*` tests, which must keep passing unchanged — they are the proof the move preserved behavior.

- [ ] **Step 7: Commit**

```bash
git add internal/box/box.go internal/box/shutdown_test.go internal/handlers/box.go internal/handlers/box_test.go
git commit -m "refactor(box): move shutdown-over-SSH into internal/box"
```

---

### Task 2: Decision logic (`types.go`, `evaluate.go`)

The heart of the feature, and the only part that decides to cut power. Pure functions, no I/O, so every branch is testable directly.

**Files:**
- Create: `internal/boxoff/types.go`, `internal/boxoff/evaluate.go`
- Test: `internal/boxoff/evaluate_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: every type below. Tasks 3, 4, 7, 8 and 10 all build on them.

- [ ] **Step 1: Write the types**

Create `internal/boxoff/types.go`:

```go
// Package boxoff decides when the remote Box build machine may be powered
// off, and does it. Box is woken on demand but nothing ever turns it off, so
// it regularly idles for days.
//
// The decision is deliberately conservative: a shutdown needs several
// consecutive clean evaluations, and every unknown reading — a failed SSH
// probe, an unreachable metrics endpoint, a database error — counts as a
// reason to stay on. Powering off a machine someone is using is far worse
// than leaving it running one more interval.
package boxoff

import "time"

// Blocker names. These are stable machine keys; the frontend maps them to
// Czech labels, so they must not be reworded to suit a UI.
const (
	// BlockerDisabled means the box_auto_off setting is off.
	BlockerDisabled = "disabled"
	// BlockerBoxOffline means the SSH probe did not reach Box.
	BlockerBoxOffline = "box_offline"
	// BlockerUptime means Box has not been up long enough yet.
	BlockerUptime = "uptime"
	// BlockerBotkaTasks means the task runner has work in flight.
	BlockerBotkaTasks = "botka_tasks"
	// BlockerBotkaBoxChats means a chat session is running on Box.
	BlockerBotkaBoxChats = "botka_box_chats"
	// BlockerKukatkoQueue means Kukátko has queued or running jobs — or its
	// queue could not be read, which counts the same way.
	BlockerKukatkoQueue = "kukatko_queue"
	// BlockerGPU means GPU utilization is above the threshold.
	BlockerGPU = "gpu"
	// BlockerCPU means the load average is above the threshold.
	BlockerCPU = "cpu"
)

// Blocker is one reason a shutdown did not happen. Name is a stable key from
// the constants above; Detail carries the measured facts in a
// language-neutral form ("12 queued, 1 running").
type Blocker struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
}

// BoxReadings holds the values sampled from Box in one SSH round-trip.
type BoxReadings struct {
	UptimeSeconds float64 `json:"uptime_seconds"`
	Load1         float64 `json:"load1"`
	Threads       int     `json:"threads"`
	GPUMaxPercent int     `json:"gpu_max_percent"`
}

// KukatkoReadings is the outcome of one Kukátko metrics scrape. It is filled
// in even when the scrape failed, because the failure detail (status code,
// transport error, a slice of an unexpected body) is exactly what the UI
// needs to explain why no shutdown happened.
type KukatkoReadings struct {
	StatusCode  int    `json:"status_code"`
	Queued      int    `json:"queued"`
	Running     int    `json:"running"`
	Error       string `json:"error,omitempty"`
	BodyExcerpt string `json:"body_excerpt,omitempty"`
}

// Inputs bundles every reading Evaluate needs. Gathering them is the
// monitor's job; deciding on them is Evaluate's.
type Inputs struct {
	// Enabled is the box_auto_off setting.
	Enabled bool

	// BoxOnline reports whether the SSH probe succeeded; Box and ProbeError
	// hold its result.
	BoxOnline  bool
	Box        BoxReadings
	ProbeError string

	// KukatkoOK reports whether the scrape produced a trustworthy queue
	// reading. Kukatko is populated either way.
	KukatkoOK bool
	Kukatko   KukatkoReadings

	// RunningTasks and BoxChats are human-readable labels of Botka's own work;
	// empty means idle. ActivityError is set when that could not be
	// determined, which blocks.
	RunningTasks  []string
	BoxChats      []string
	ActivityError string
}

// Thresholds are the tunables behind the decision, all env-configurable.
type Thresholds struct {
	Interval   time.Duration
	MinUptime  time.Duration
	IdleChecks int
	GPUPercent int
	Load1      float64
}

// Evaluation is one pass over the conditions: the blockers found, and the
// readings behind them for the UI. Idle is true exactly when Blockers is
// empty.
type Evaluation struct {
	CheckedAt time.Time        `json:"checked_at"`
	Idle      bool             `json:"idle"`
	Blockers  []Blocker        `json:"blockers"`
	Box       *BoxReadings     `json:"box"`
	Kukatko   *KukatkoReadings `json:"kukatko"`
}
```

- [ ] **Step 2: Write the failing tests**

Create `internal/boxoff/evaluate_test.go`:

```go
package boxoff

import (
	"testing"
	"time"
)

func testThresholds() Thresholds {
	return Thresholds{
		Interval:   10 * time.Minute,
		MinUptime:  2 * time.Hour,
		IdleChecks: 3,
		GPUPercent: 10,
		Load1:      1.0,
	}
}

// idleInputs is a fully clean set of readings: enabled, Box up for 9 hours,
// idle CPU and GPU, empty Kukátko queue, no Botka work.
func idleInputs() Inputs {
	return Inputs{
		Enabled:   true,
		BoxOnline: true,
		Box:       BoxReadings{UptimeSeconds: 33390, Load1: 0.01, Threads: 24, GPUMaxPercent: 0},
		KukatkoOK: true,
		Kukatko:   KukatkoReadings{StatusCode: 200},
	}
}

func blockerNames(ev Evaluation) []string {
	names := make([]string, 0, len(ev.Blockers))
	for _, b := range ev.Blockers {
		names = append(names, b.Name)
	}
	return names
}

func hasBlocker(ev Evaluation, name string) bool {
	for _, b := range ev.Blockers {
		if b.Name == name {
			return true
		}
	}
	return false
}

func TestEvaluate_IdleHasNoBlockers(t *testing.T) {
	ev := Evaluate(time.Now(), idleInputs(), testThresholds())

	if !ev.Idle {
		t.Fatalf("Idle = false, blockers %v", blockerNames(ev))
	}
	if ev.Box == nil || ev.Kukatko == nil {
		t.Error("readings must be attached for the UI")
	}
}

func TestEvaluate_TerminalBlockers(t *testing.T) {
	t.Run("disabled short-circuits", func(t *testing.T) {
		in := idleInputs()
		in.Enabled = false

		ev := Evaluate(time.Now(), in, testThresholds())

		if got := blockerNames(ev); len(got) != 1 || got[0] != BlockerDisabled {
			t.Fatalf("blockers = %v, want exactly [%s]", got, BlockerDisabled)
		}
		if ev.Box != nil {
			t.Error("a disabled monitor must not report Box readings it never took")
		}
	})

	t.Run("offline short-circuits", func(t *testing.T) {
		in := idleInputs()
		in.BoxOnline = false
		in.ProbeError = "connection refused"

		ev := Evaluate(time.Now(), in, testThresholds())

		if got := blockerNames(ev); len(got) != 1 || got[0] != BlockerBoxOffline {
			t.Fatalf("blockers = %v, want exactly [%s]", got, BlockerBoxOffline)
		}
		if ev.Blockers[0].Detail == "" {
			t.Error("offline blocker must carry the probe error")
		}
	})
}

func TestEvaluate_Blockers(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Inputs)
		want   string
	}{
		{
			name:   "uptime below minimum",
			mutate: func(in *Inputs) { in.Box.UptimeSeconds = 3600 },
			want:   BlockerUptime,
		},
		{
			name:   "running task",
			mutate: func(in *Inputs) { in.RunningTasks = []string{"fix the parser"} },
			want:   BlockerBotkaTasks,
		},
		{
			name:   "box chat session",
			mutate: func(in *Inputs) { in.BoxChats = []string{"render the video"} },
			want:   BlockerBotkaBoxChats,
		},
		{
			name:   "activity lookup failed",
			mutate: func(in *Inputs) { in.ActivityError = "database is closed" },
			want:   BlockerBotkaBoxChats,
		},
		{
			name:   "kukatko queue not empty",
			mutate: func(in *Inputs) { in.Kukatko.Queued = 12; in.Kukatko.Running = 1 },
			want:   BlockerKukatkoQueue,
		},
		{
			name: "kukatko unreadable",
			mutate: func(in *Inputs) {
				in.KukatkoOK = false
				in.Kukatko = KukatkoReadings{StatusCode: 502, Error: "bad gateway"}
			},
			want: BlockerKukatkoQueue,
		},
		{
			name:   "gpu busy",
			mutate: func(in *Inputs) { in.Box.GPUMaxPercent = 47 },
			want:   BlockerGPU,
		},
		{
			name:   "cpu busy",
			mutate: func(in *Inputs) { in.Box.Load1 = 3.2 },
			want:   BlockerCPU,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := idleInputs()
			tt.mutate(&in)

			ev := Evaluate(time.Now(), in, testThresholds())

			if ev.Idle {
				t.Fatal("Idle = true, want blocked")
			}
			if !hasBlocker(ev, tt.want) {
				t.Fatalf("blockers = %v, want %s among them", blockerNames(ev), tt.want)
			}
			for _, b := range ev.Blockers {
				if b.Detail == "" {
					t.Errorf("blocker %s has no detail", b.Name)
				}
			}
		})
	}
}

func TestEvaluate_ThresholdsAreInclusive(t *testing.T) {
	// Exactly at a threshold is idle; only strictly above blocks. The same
	// applies to uptime: exactly MinUptime is enough.
	in := idleInputs()
	in.Box.GPUMaxPercent = 10
	in.Box.Load1 = 1.0
	in.Box.UptimeSeconds = 7200

	ev := Evaluate(time.Now(), in, testThresholds())

	if !ev.Idle {
		t.Fatalf("Idle = false at exact thresholds, blockers %v", blockerNames(ev))
	}
}

func TestEvaluate_CollectsEveryNonTerminalBlocker(t *testing.T) {
	in := idleInputs()
	in.Box.Load1 = 5
	in.Box.GPUMaxPercent = 90
	in.RunningTasks = []string{"a task"}

	ev := Evaluate(time.Now(), in, testThresholds())

	if len(ev.Blockers) != 3 {
		t.Fatalf("blockers = %v, want all three reported", blockerNames(ev))
	}
}

func TestEarliestShutdownAt(t *testing.T) {
	cfg := testThresholds()
	now := time.Date(2026, 9, 6, 20, 0, 0, 0, time.UTC)
	nextCheck := now.Add(7 * time.Minute)
	idle := Evaluate(now, idleInputs(), cfg)

	t.Run("clean at streak 0 needs three ticks", func(t *testing.T) {
		got := EarliestShutdownAt(now, idle, 0, nextCheck, cfg)
		want := nextCheck.Add(20 * time.Minute)
		if got == nil || !got.Equal(want) {
			t.Fatalf("= %v, want %v", got, want)
		}
	})

	t.Run("clean at streak 2 fires on the next tick", func(t *testing.T) {
		got := EarliestShutdownAt(now, idle, 2, nextCheck, cfg)
		if got == nil || !got.Equal(nextCheck) {
			t.Fatalf("= %v, want %v", got, nextCheck)
		}
	})

	t.Run("uptime-only blocker still counts down", func(t *testing.T) {
		in := idleInputs()
		in.Box.UptimeSeconds = 300 // 5 minutes up, 115 to go
		ev := Evaluate(now, in, cfg)

		got := EarliestShutdownAt(now, ev, 0, nextCheck, cfg)

		// Ready at now+115m; ticks are at now+7m, +17m, ... The first tick at
		// or after now+115m is now+117m; then two more intervals.
		want := now.Add(117 * time.Minute).Add(20 * time.Minute)
		if got == nil || !got.Equal(want) {
			t.Fatalf("= %v, want %v", got, want)
		}
	})

	t.Run("any other blocker has no countdown", func(t *testing.T) {
		in := idleInputs()
		in.Box.Load1 = 4
		ev := Evaluate(now, in, cfg)

		if got := EarliestShutdownAt(now, ev, 0, nextCheck, cfg); got != nil {
			t.Fatalf("= %v, want nil", got)
		}
	})

	t.Run("uptime plus another blocker has no countdown", func(t *testing.T) {
		in := idleInputs()
		in.Box.UptimeSeconds = 300
		in.Box.Load1 = 4
		ev := Evaluate(now, in, cfg)

		if got := EarliestShutdownAt(now, ev, 0, nextCheck, cfg); got != nil {
			t.Fatalf("= %v, want nil", got)
		}
	})

	t.Run("disabled has no countdown", func(t *testing.T) {
		in := idleInputs()
		in.Enabled = false
		ev := Evaluate(now, in, cfg)

		if got := EarliestShutdownAt(now, ev, 0, nextCheck, cfg); got != nil {
			t.Fatalf("= %v, want nil", got)
		}
	})
}
```

- [ ] **Step 3: Run the tests to verify they fail**

Run: `go test ./internal/boxoff/ -v`
Expected: FAIL — `undefined: Evaluate`, `undefined: EarliestShutdownAt`.

- [ ] **Step 4: Write the implementation**

Create `internal/boxoff/evaluate.go`:

```go
package boxoff

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// Evaluate applies every condition to one set of readings and returns the
// blockers found. It performs no I/O — gathering the readings is the
// monitor's job.
//
// Two conditions are terminal. A disabled monitor and an unreachable Box both
// mean there is nothing to measure, so evaluation stops with that single
// blocker. Every other condition is evaluated even once one has blocked, so
// the UI can show the whole picture rather than whichever check ran first.
func Evaluate(now time.Time, in Inputs, cfg Thresholds) Evaluation {
	ev := Evaluation{CheckedAt: now}

	if !in.Enabled {
		ev.Blockers = []Blocker{{Name: BlockerDisabled, Detail: "auto-off is switched off"}}
		return ev
	}

	if !in.BoxOnline {
		detail := "box is not reachable over SSH"
		if in.ProbeError != "" {
			detail = in.ProbeError
		}
		ev.Blockers = []Blocker{{Name: BlockerBoxOffline, Detail: detail}}
		return ev
	}

	box := in.Box
	ev.Box = &box
	kukatko := in.Kukatko
	ev.Kukatko = &kukatko

	uptime := time.Duration(in.Box.UptimeSeconds * float64(time.Second))
	if uptime < cfg.MinUptime {
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerUptime,
			Detail: fmt.Sprintf("up %s, minimum %s", uptime.Round(time.Minute), cfg.MinUptime),
		})
	}

	if len(in.RunningTasks) > 0 {
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerBotkaTasks,
			Detail: fmt.Sprintf("%d running: %s", len(in.RunningTasks), strings.Join(in.RunningTasks, ", ")),
		})
	}

	switch {
	case in.ActivityError != "":
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerBotkaBoxChats,
			Detail: "could not be determined: " + in.ActivityError,
		})
	case len(in.BoxChats) > 0:
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerBotkaBoxChats,
			Detail: fmt.Sprintf("%d on box: %s", len(in.BoxChats), strings.Join(in.BoxChats, ", ")),
		})
	}

	switch {
	case !in.KukatkoOK:
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerKukatkoQueue,
			Detail: "queue could not be read: " + kukatkoFailureDetail(in.Kukatko),
		})
	case in.Kukatko.Queued+in.Kukatko.Running > 0:
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerKukatkoQueue,
			Detail: fmt.Sprintf("%d queued, %d running", in.Kukatko.Queued, in.Kukatko.Running),
		})
	}

	if in.Box.GPUMaxPercent > cfg.GPUPercent {
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerGPU,
			Detail: fmt.Sprintf("%d%% > %d%%", in.Box.GPUMaxPercent, cfg.GPUPercent),
		})
	}

	if in.Box.Load1 > cfg.Load1 {
		ev.Blockers = append(ev.Blockers, Blocker{
			Name:   BlockerCPU,
			Detail: fmt.Sprintf("load1 %.2f > %.2f (%d threads)", in.Box.Load1, cfg.Load1, in.Box.Threads),
		})
	}

	ev.Idle = len(ev.Blockers) == 0
	return ev
}

// kukatkoFailureDetail summarizes why a scrape could not be trusted.
func kukatkoFailureDetail(k KukatkoReadings) string {
	parts := make([]string, 0, 3)
	if k.StatusCode != 0 {
		parts = append(parts, fmt.Sprintf("HTTP %d", k.StatusCode))
	}
	if k.Error != "" {
		parts = append(parts, k.Error)
	}
	if k.BodyExcerpt != "" {
		parts = append(parts, k.BodyExcerpt)
	}
	if len(parts) == 0 {
		return "no detail"
	}
	return strings.Join(parts, ": ")
}

// EarliestShutdownAt returns the soonest moment a shutdown could happen if
// nothing changes, or nil when no honest estimate exists.
//
// With no blockers the run needs IdleChecks-streak more clean ticks, all
// Interval apart starting at nextCheck. When uptime is the *only* blocker the
// countdown survives, because that blocker has a known expiry: wait for Box to
// reach MinUptime, then run the full streak from the first tick after that.
// Every other blocker — a task, a queue, load — has no predictable end, and a
// countdown that keeps resetting is worse than none.
func EarliestShutdownAt(now time.Time, ev Evaluation, streak int, nextCheck time.Time, cfg Thresholds) *time.Time {
	if cfg.IdleChecks <= 0 || cfg.Interval <= 0 {
		return nil
	}

	if len(ev.Blockers) == 0 {
		remaining := cfg.IdleChecks - streak - 1
		if remaining < 0 {
			remaining = 0
		}
		t := nextCheck.Add(time.Duration(remaining) * cfg.Interval)
		return &t
	}

	if len(ev.Blockers) != 1 || ev.Blockers[0].Name != BlockerUptime || ev.Box == nil {
		return nil
	}

	wait := cfg.MinUptime - time.Duration(ev.Box.UptimeSeconds*float64(time.Second))
	if wait < 0 {
		wait = 0
	}
	t := firstTickAtOrAfter(nextCheck, cfg.Interval, now.Add(wait)).
		Add(time.Duration(cfg.IdleChecks-1) * cfg.Interval)
	return &t
}

// firstTickAtOrAfter returns the first tick of the schedule that starts at
// first and repeats every interval, landing at or after target.
func firstTickAtOrAfter(first time.Time, interval time.Duration, target time.Time) time.Time {
	if !target.After(first) {
		return first
	}
	steps := math.Ceil(float64(target.Sub(first)) / float64(interval))
	return first.Add(time.Duration(steps) * interval)
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/boxoff/ -v`
Expected: PASS, all subtests.

- [ ] **Step 6: Commit**

```bash
git add internal/boxoff/types.go internal/boxoff/evaluate.go internal/boxoff/evaluate_test.go
git commit -m "feat(boxoff): idleness evaluation and shutdown countdown"
```

---

### Task 3: SSH probe (`probe.go`)

One SSH round-trip returns everything Box-side. Parsing is separate from running so malformed output is testable without a network.

**Files:**
- Create: `internal/boxoff/probe.go`
- Test: `internal/boxoff/probe_test.go`

**Interfaces:**
- Consumes: `BoxReadings` (Task 2).
- Produces: `CommandRunner` interface; `NewSSHProbe(run box.RunFunc, sshTarget string) *SSHProbe`; `(*SSHProbe).Probe(ctx) (BoxReadings, error)`; `ParseProbeOutput(string) (BoxReadings, error)`. Task 8 consumes `Probe`.

- [ ] **Step 1: Write the failing tests**

Create `internal/boxoff/probe_test.go`:

```go
package boxoff

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestParseProbeOutput_Valid(t *testing.T) {
	out := "33390.91\n0.45\n24\n0\n7\n3\n"

	got, err := ParseProbeOutput(out)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error = %v", err)
	}

	if got.UptimeSeconds != 33390.91 {
		t.Errorf("UptimeSeconds = %v, want 33390.91", got.UptimeSeconds)
	}
	if got.Load1 != 0.45 {
		t.Errorf("Load1 = %v, want 0.45", got.Load1)
	}
	if got.Threads != 24 {
		t.Errorf("Threads = %v, want 24", got.Threads)
	}
	if got.GPUMaxPercent != 7 {
		t.Errorf("GPUMaxPercent = %v, want 7 (the max sample)", got.GPUMaxPercent)
	}
}

func TestParseProbeOutput_MultipleGPUs(t *testing.T) {
	// Two GPUs means two lines per sample; the max across all of them wins.
	out := "100.0\n0.10\n24\n0\n12\n0\n3\n0\n1\n"

	got, err := ParseProbeOutput(out)
	if err != nil {
		t.Fatalf("ParseProbeOutput() error = %v", err)
	}
	if got.GPUMaxPercent != 12 {
		t.Errorf("GPUMaxPercent = %d, want 12", got.GPUMaxPercent)
	}
}

func TestParseProbeOutput_Rejected(t *testing.T) {
	tests := []struct {
		name string
		out  string
	}{
		{name: "empty", out: ""},
		{name: "truncated before gpu samples", out: "33390.91\n0.45\n24\n"},
		{name: "nvidia-smi error instead of a sample", out: "33390.91\n0.45\n24\nNVIDIA-SMI has failed because it couldn't communicate with the driver\n"},
		{name: "non-numeric uptime", out: "up 9 hours\n0.45\n24\n0\n"},
		{name: "non-numeric load", out: "33390.91\nnope\n24\n0\n"},
		{name: "non-numeric thread count", out: "33390.91\n0.45\nmany\n0\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseProbeOutput(tt.out); err == nil {
				t.Fatal("ParseProbeOutput() = nil error, want a rejection — an unparsable probe must never read as idle")
			}
		})
	}
}

func TestSSHProbe_RunsOneCommand(t *testing.T) {
	var calls int
	var gotArgs []string
	run := func(_ context.Context, name string, args ...string) ([]byte, error) {
		calls++
		gotArgs = append([]string{name}, args...)
		return []byte("33390.91\n0.01\n24\n0\n0\n0\n"), nil
	}

	p := NewSSHProbe(run, "panbotka@box")
	got, err := p.Probe(context.Background())
	if err != nil {
		t.Fatalf("Probe() error = %v", err)
	}

	if calls != 1 {
		t.Errorf("ran %d commands, want exactly 1 round-trip", calls)
	}
	if got.Threads != 24 {
		t.Errorf("Threads = %d, want 24", got.Threads)
	}
	joined := strings.Join(gotArgs, " ")
	for _, want := range []string{"ssh", "BatchMode=yes", "panbotka@box", "/proc/uptime", "/proc/loadavg", "nvidia-smi"} {
		if !strings.Contains(joined, want) {
			t.Errorf("command %q missing %q", joined, want)
		}
	}
}

func TestSSHProbe_ErrorCarriesOutput(t *testing.T) {
	run := func(_ context.Context, _ string, _ ...string) ([]byte, error) {
		return []byte("ssh: connect to host box port 22: No route to host"), errors.New("exit status 255")
	}

	_, err := NewSSHProbe(run, "panbotka@box").Probe(context.Background())
	if err == nil {
		t.Fatal("Probe() = nil error, want failure")
	}
	if !strings.Contains(err.Error(), "No route to host") {
		t.Errorf("error %q does not carry the ssh output", err)
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/boxoff/ -run Probe -v`
Expected: FAIL — `undefined: ParseProbeOutput`, `undefined: NewSSHProbe`.

- [ ] **Step 3: Write the implementation**

Create `internal/boxoff/probe.go`:

```go
package boxoff

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"botka/internal/box"
)

// probeTimeout bounds the SSH probe. The remote script sleeps twice between
// three GPU samples, so a healthy run takes ~3 s plus the SSH handshake.
const probeTimeout = 25 * time.Second

// probeScript is what runs on Box. It is one command so a probe costs a single
// SSH round-trip, and it prints fixed-position lines rather than anything that
// needs real parsing:
//
//	line 1     uptime in seconds
//	line 2     1-minute load average
//	line 3     hardware thread count
//	lines 4..n GPU utilization, one line per GPU per sample
//
// GPU utilization is sampled in a shell loop because nvidia-smi refuses
// --query-gpu together with its own -l/-c loop flags ("Option
// --query-gpu=utilization.gpu is not recognized"). Utilization is the only
// usable busy signal: the idle photo-enhancer and image-embeddings services
// hold several GB of VRAM permanently, so the presence of a compute process
// says nothing.
const probeScript = `cut -d' ' -f1 /proc/uptime; ` +
	`cut -d' ' -f1 /proc/loadavg; ` +
	`nproc; ` +
	`for i in 1 2 3; do nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits; sleep 1; done`

// SSHProbe samples Box's uptime, load and GPU utilization over SSH.
type SSHProbe struct {
	run     box.RunFunc
	target  string
	timeout time.Duration
}

// NewSSHProbe returns a probe that runs commands with run against sshTarget
// ("user@host").
func NewSSHProbe(run box.RunFunc, sshTarget string) *SSHProbe {
	return &SSHProbe{run: run, target: sshTarget, timeout: probeTimeout}
}

// Probe runs the probe script on Box and parses its output. Any failure —
// unreachable host, a shell error, unexpected output — is returned as an
// error; callers must treat that as "unknown", never as "idle".
func (p *SSHProbe) Probe(ctx context.Context) (BoxReadings, error) {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	out, err := p.run(ctx, "ssh",
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=5",
		"-o", "StrictHostKeyChecking=no",
		p.target,
		probeScript,
	)
	if err != nil {
		return BoxReadings{}, fmt.Errorf("box probe: %w: %s", err, strings.TrimSpace(string(out)))
	}

	return ParseProbeOutput(string(out))
}

// ParseProbeOutput turns the probe script's stdout into readings. It is strict
// on purpose: a missing or unparsable field returns an error rather than a
// zero value, because a zero reading looks exactly like an idle machine.
func ParseProbeOutput(out string) (BoxReadings, error) {
	lines := make([]string, 0, 8)
	for _, line := range strings.Split(out, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) < 4 {
		return BoxReadings{}, fmt.Errorf("probe output has %d non-empty lines, want at least 4: %q", len(lines), out)
	}

	uptime, err := strconv.ParseFloat(lines[0], 64)
	if err != nil {
		return BoxReadings{}, fmt.Errorf("uptime %q: %w", lines[0], err)
	}
	load1, err := strconv.ParseFloat(lines[1], 64)
	if err != nil {
		return BoxReadings{}, fmt.Errorf("load average %q: %w", lines[1], err)
	}
	threads, err := strconv.Atoi(lines[2])
	if err != nil {
		return BoxReadings{}, fmt.Errorf("thread count %q: %w", lines[2], err)
	}

	gpuMax := 0
	for _, sample := range lines[3:] {
		v, err := strconv.Atoi(sample)
		if err != nil {
			return BoxReadings{}, fmt.Errorf("gpu sample %q: %w", sample, err)
		}
		if v > gpuMax {
			gpuMax = v
		}
	}

	return BoxReadings{
		UptimeSeconds: uptime,
		Load1:         load1,
		Threads:       threads,
		GPUMaxPercent: gpuMax,
	}, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/boxoff/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/boxoff/probe.go internal/boxoff/probe_test.go
git commit -m "feat(boxoff): SSH probe for Box uptime, load and GPU utilization"
```

---

### Task 4: Kukátko metrics client (`kukatko.go`)

**Files:**
- Create: `internal/boxoff/kukatko.go`
- Test: `internal/boxoff/kukatko_test.go`

**Interfaces:**
- Consumes: `KukatkoReadings` (Task 2).
- Produces: `NewMetricsClient(url string) *MetricsClient`; `(*MetricsClient).Fetch(ctx) (KukatkoReadings, error)`; `ParseQueueDepth(body string) (queued, running int, err error)`. Task 8 consumes `Fetch`.

- [ ] **Step 1: Write the failing tests**

Create `internal/boxoff/kukatko_test.go`:

```go
package boxoff

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// realSample is trimmed from the live https://fotky.kotrzina.cz/metrics
// response, including the neighbouring families whose names share a prefix
// with the one we read.
const realSample = `# HELP kukatko_jobs_queue_depth Number of jobs in the queue, partitioned by state.
# TYPE kukatko_jobs_queue_depth gauge
kukatko_jobs_queue_depth{state="done"} 155145
kukatko_jobs_queue_depth{state="queued"} 12
kukatko_jobs_queue_depth{state="running"} 1
kukatko_jobs_queue_depth_by_type{type="image_embed"} 41640
kukatko_jobs_queue_depth_by_type_state{state="queued",type="image_embed"} 12
kukatko_embedding_service_up 0
`

func TestParseQueueDepth_ReadsQueuedAndRunning(t *testing.T) {
	queued, running, err := ParseQueueDepth(realSample)
	if err != nil {
		t.Fatalf("ParseQueueDepth() error = %v", err)
	}
	if queued != 12 {
		t.Errorf("queued = %d, want 12 (the by_type_state family must not be counted)", queued)
	}
	if running != 1 {
		t.Errorf("running = %d, want 1", running)
	}
}

func TestParseQueueDepth_DoneOnlyIsEmptyQueue(t *testing.T) {
	body := `kukatko_jobs_queue_depth{state="done"} 155145` + "\n"

	queued, running, err := ParseQueueDepth(body)
	if err != nil {
		t.Fatalf("ParseQueueDepth() error = %v", err)
	}
	if queued != 0 || running != 0 {
		t.Errorf("queued=%d running=%d, want 0/0", queued, running)
	}
}

func TestParseQueueDepth_RejectsBodyWithoutTheFamily(t *testing.T) {
	// A renamed metric, an HTML error page or an app that never registered its
	// collectors must not read as "queue empty" — that would license a
	// shutdown on no information at all.
	tests := []struct {
		name string
		body string
	}{
		{name: "empty body", body: ""},
		{name: "html error page", body: "<html><body>502 Bad Gateway</body></html>"},
		{name: "other metrics only", body: "kukatko_http_requests_total{code=\"200\"} 5\n"},
		{name: "only the by_type families", body: "kukatko_jobs_queue_depth_by_type{type=\"ocr\"} 3\n"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseQueueDepth(tt.body); err == nil {
				t.Fatal("ParseQueueDepth() = nil error, want a rejection")
			}
		})
	}
}

func TestMetricsClient_Fetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte(realSample))
	}))
	defer srv.Close()

	got, err := NewMetricsClient(srv.URL).Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if got.StatusCode != http.StatusOK || got.Queued != 12 || got.Running != 1 {
		t.Errorf("got %+v, want 200/12/1", got)
	}
	if got.Error != "" {
		t.Errorf("Error = %q, want empty", got.Error)
	}
}

func TestMetricsClient_Fetch_HTTPErrorKeepsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream is down"))
	}))
	defer srv.Close()

	got, err := NewMetricsClient(srv.URL).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() = nil error, want failure on 502")
	}
	if got.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want 502", got.StatusCode)
	}
	if !strings.Contains(got.BodyExcerpt, "upstream is down") {
		t.Errorf("BodyExcerpt = %q, want the response body for the UI", got.BodyExcerpt)
	}
}

func TestMetricsClient_Fetch_TransportErrorKeepsDetail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing is listening any more

	got, err := NewMetricsClient(url).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() = nil error, want failure")
	}
	if got.Error == "" {
		t.Error("Error is empty; the transport failure must be reported to the UI")
	}
}

func TestMetricsClient_Fetch_UntrustworthyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("<html>Just a moment...</html>"))
	}))
	defer srv.Close()

	got, err := NewMetricsClient(srv.URL).Fetch(context.Background())
	if err == nil {
		t.Fatal("Fetch() = nil error, want rejection of a 200 with no metrics in it")
	}
	if got.BodyExcerpt == "" {
		t.Error("BodyExcerpt is empty; the unexpected body must reach the UI")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/boxoff/ -run 'Kukatko|QueueDepth|Metrics' -v`
Expected: FAIL — `undefined: ParseQueueDepth`, `undefined: NewMetricsClient`.

- [ ] **Step 3: Write the implementation**

Create `internal/boxoff/kukatko.go`:

```go
package boxoff

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	// metricsTimeout bounds one scrape of the Kukátko metrics endpoint.
	metricsTimeout = 10 * time.Second
	// metricsBodyLimit caps how much of the response is read. The live
	// endpoint is ~100 kB; anything an order of magnitude larger is a sign
	// something else is answering.
	metricsBodyLimit = 4 << 20
	// bodyExcerptLimit is how much of an unexpected body is kept for the UI.
	bodyExcerptLimit = 200
	// queueDepthPrefix matches only the by-state family. The trailing brace
	// matters: kukatko_jobs_queue_depth_by_type and
	// kukatko_jobs_queue_depth_by_type_state share the metric-name prefix and
	// would otherwise be double-counted.
	queueDepthPrefix = "kukatko_jobs_queue_depth{"
)

// MetricsClient reads Kukátko's job queue depth from its Prometheus endpoint.
// The endpoint is unauthenticated and served outside Kukátko's /api/v1 tree.
type MetricsClient struct {
	url    string
	client *http.Client
}

// NewMetricsClient returns a client scraping the given metrics URL.
func NewMetricsClient(url string) *MetricsClient {
	return &MetricsClient{
		url:    url,
		client: &http.Client{Timeout: metricsTimeout},
	}
}

// Fetch scrapes the endpoint and returns the queue depth.
//
// The returned readings are filled in even when the error is non-nil — status
// code, transport error and a slice of an unexpected body are what the UI
// shows to explain why no shutdown happened. A non-nil error always means
// "the queue state is unknown", which callers must treat as a blocker.
func (c *MetricsClient) Fetch(ctx context.Context) (KukatkoReadings, error) {
	var out KukatkoReadings

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.url, nil)
	if err != nil {
		out.Error = err.Error()
		return out, fmt.Errorf("kukatko metrics request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		out.Error = err.Error()
		return out, fmt.Errorf("kukatko metrics: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	out.StatusCode = resp.StatusCode

	body, err := io.ReadAll(io.LimitReader(resp.Body, metricsBodyLimit))
	if err != nil {
		out.Error = err.Error()
		return out, fmt.Errorf("kukatko metrics body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		out.BodyExcerpt = excerpt(string(body))
		return out, fmt.Errorf("kukatko metrics: HTTP %d", resp.StatusCode)
	}

	queued, running, err := ParseQueueDepth(string(body))
	if err != nil {
		out.Error = err.Error()
		out.BodyExcerpt = excerpt(string(body))
		return out, fmt.Errorf("kukatko metrics: %w", err)
	}

	out.Queued = queued
	out.Running = running
	return out, nil
}

// ParseQueueDepth sums the queued and running job counts out of a Prometheus
// exposition body.
//
// It refuses a body that contains no kukatko_jobs_queue_depth sample at all.
// Without that guard a renamed metric, a captive-portal page or an app that
// never registered its collectors would all parse as "queue empty" and
// license a shutdown on no information.
func ParseQueueDepth(body string) (queued, running int, err error) {
	found := false

	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, queueDepthPrefix) {
			continue
		}
		found = true

		state, value, err := parseQueueDepthSample(line)
		if err != nil {
			return 0, 0, err
		}
		switch state {
		case "queued":
			queued += value
		case "running":
			running += value
		}
	}

	if !found {
		return 0, 0, errors.New("no kukatko_jobs_queue_depth samples in response")
	}
	return queued, running, nil
}

// parseQueueDepthSample splits one exposition line into its state label and
// value: `kukatko_jobs_queue_depth{state="queued"} 12`.
func parseQueueDepthSample(line string) (state string, value int, err error) {
	close := strings.Index(line, "}")
	if close < 0 {
		return "", 0, fmt.Errorf("malformed sample %q", line)
	}

	labels := line[len(queueDepthPrefix):close]
	for _, pair := range strings.Split(labels, ",") {
		name, val, ok := strings.Cut(strings.TrimSpace(pair), "=")
		if ok && name == "state" {
			state = strings.Trim(val, `"`)
		}
	}
	if state == "" {
		return "", 0, fmt.Errorf("sample without a state label: %q", line)
	}

	raw := strings.TrimSpace(line[close+1:])
	if raw == "" {
		return "", 0, fmt.Errorf("sample without a value: %q", line)
	}
	// Prometheus gauges are floats on the wire ("12" or "12.0"); job counts
	// are whole numbers, so truncation is exact.
	f, err := strconv.ParseFloat(strings.Fields(raw)[0], 64)
	if err != nil {
		return "", 0, fmt.Errorf("sample value %q: %w", raw, err)
	}
	return state, int(f), nil
}

// excerpt shortens an unexpected response body to something a UI can show.
func excerpt(body string) string {
	body = strings.TrimSpace(body)
	if len(body) <= bodyExcerptLimit {
		return body
	}
	return body[:bodyExcerptLimit] + "…"
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/boxoff/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/boxoff/kukatko.go internal/boxoff/kukatko_test.go
git commit -m "feat(boxoff): read Kukatko job queue depth from its metrics endpoint"
```

---

### Task 5: Configuration

**Files:**
- Modify: `internal/config/config.go` (struct near line 41, `Load` near line 166)
- Test: `internal/config/config_test.go`

**Interfaces:**
- Produces: `cfg.BoxAutoOffInterval`, `cfg.BoxAutoOffMinUptime`, `cfg.BoxAutoOffIdleChecks`, `cfg.BoxAutoOffGPUPercent`, `cfg.BoxAutoOffLoad1`, `cfg.KukatkoMetricsURL`. Task 11 reads them.

- [ ] **Step 1: Write the failing test**

Append to `internal/config/config_test.go` (match the file's existing style for setting env vars):

```go
func TestLoad_BoxAutoOffDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.BoxAutoOffInterval != 10*time.Minute {
		t.Errorf("BoxAutoOffInterval = %v, want 10m", cfg.BoxAutoOffInterval)
	}
	if cfg.BoxAutoOffMinUptime != 2*time.Hour {
		t.Errorf("BoxAutoOffMinUptime = %v, want 2h", cfg.BoxAutoOffMinUptime)
	}
	if cfg.BoxAutoOffIdleChecks != 3 {
		t.Errorf("BoxAutoOffIdleChecks = %d, want 3", cfg.BoxAutoOffIdleChecks)
	}
	if cfg.BoxAutoOffGPUPercent != 10 {
		t.Errorf("BoxAutoOffGPUPercent = %d, want 10", cfg.BoxAutoOffGPUPercent)
	}
	if cfg.BoxAutoOffLoad1 != 1.0 {
		t.Errorf("BoxAutoOffLoad1 = %v, want 1.0", cfg.BoxAutoOffLoad1)
	}
	if cfg.KukatkoMetricsURL != "https://fotky.kotrzina.cz/metrics" {
		t.Errorf("KukatkoMetricsURL = %q", cfg.KukatkoMetricsURL)
	}
}

func TestLoad_BoxAutoOffOverrides(t *testing.T) {
	t.Setenv("BOX_AUTO_OFF_INTERVAL", "3m")
	t.Setenv("BOX_AUTO_OFF_MIN_UPTIME", "45m")
	t.Setenv("BOX_AUTO_OFF_IDLE_CHECKS", "5")
	t.Setenv("BOX_AUTO_OFF_GPU_THRESHOLD", "25")
	t.Setenv("BOX_AUTO_OFF_LOAD_THRESHOLD", "2.5")
	t.Setenv("KUKATKO_METRICS_URL", "http://localhost:9999/metrics")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.BoxAutoOffInterval != 3*time.Minute {
		t.Errorf("BoxAutoOffInterval = %v, want 3m", cfg.BoxAutoOffInterval)
	}
	if cfg.BoxAutoOffIdleChecks != 5 {
		t.Errorf("BoxAutoOffIdleChecks = %d, want 5", cfg.BoxAutoOffIdleChecks)
	}
	if cfg.BoxAutoOffLoad1 != 2.5 {
		t.Errorf("BoxAutoOffLoad1 = %v, want 2.5", cfg.BoxAutoOffLoad1)
	}
	if cfg.KukatkoMetricsURL != "http://localhost:9999/metrics" {
		t.Errorf("KukatkoMetricsURL = %q", cfg.KukatkoMetricsURL)
	}
}
```

Note: if `config_test.go` has no `time` import, add it.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/config/ -run BoxAutoOff -v`
Expected: FAIL — unknown fields.

- [ ] **Step 3: Add the fields**

In `internal/config/config.go`, add to the `Config` struct after `BoxWOLCommand`:

```go
	BoxAutoOffInterval        time.Duration
	BoxAutoOffMinUptime       time.Duration
	BoxAutoOffIdleChecks      int
	BoxAutoOffGPUPercent      int
	BoxAutoOffLoad1           float64
	KukatkoMetricsURL         string
```

In `Load`, follow the existing pattern for parsed values (look at how `KeepaliveInterval` is parsed above the returned struct literal — parse into a local first, return the error), then set them in the literal after `BoxWOLCommand`:

```go
		BoxAutoOffInterval:   boxAutoOffInterval,
		BoxAutoOffMinUptime:  boxAutoOffMinUptime,
		BoxAutoOffIdleChecks: boxAutoOffIdleChecks,
		BoxAutoOffGPUPercent: boxAutoOffGPUPercent,
		BoxAutoOffLoad1:      boxAutoOffLoad1,
		KukatkoMetricsURL:    getEnv("KUKATKO_METRICS_URL", "https://fotky.kotrzina.cz/metrics"),
```

using `getEnvDuration`-equivalent parsing (the file parses durations with `time.ParseDuration` on `getEnv`), `getEnvInt` for the two ints and `getEnvFloat` for the load threshold.

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/config/ -run BoxAutoOff -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat(config): box auto-off thresholds and Kukatko metrics URL"
```

---

### Task 6: Events table and model

**Files:**
- Create: `migrations/042_box_auto_off_events.up.sql`, `migrations/042_box_auto_off_events.down.sql`, `internal/models/box_auto_off_event.go`
- Test: `internal/models/models_test.go` (append)

**Interfaces:**
- Produces: `models.BoxAutoOffEvent{ID int64, OccurredAt time.Time, Outcome string, Detail json.RawMessage}`, table `box_auto_off_events`; outcome constants `models.BoxAutoOffOutcomeShutdown`, `…Failed`, `…Aborted`. Tasks 8 and 10 use them.

- [ ] **Step 1: Write the migration**

`migrations/042_box_auto_off_events.up.sql`:

```sql
-- Automatic Box shutdowns. Only actual attempts are recorded (a handful per
-- week), never the routine evaluations that decide against one -- those live
-- in the monitor's in-memory ring buffer.
CREATE TABLE box_auto_off_events (
    id          BIGSERIAL PRIMARY KEY,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    outcome     TEXT        NOT NULL,
    detail      JSONB       NOT NULL DEFAULT '{}'
);

CREATE INDEX idx_box_auto_off_events_occurred_at ON box_auto_off_events (occurred_at DESC);
```

`migrations/042_box_auto_off_events.down.sql`:

```sql
DROP TABLE IF EXISTS box_auto_off_events;
```

- [ ] **Step 2: Write the model**

`internal/models/box_auto_off_event.go`:

```go
package models

import (
	"encoding/json"
	"time"
)

// Box auto-off outcomes. Only shutdown attempts are recorded, so these three
// cover every row.
const (
	// BoxAutoOffOutcomeShutdown means the shutdown command was accepted.
	BoxAutoOffOutcomeShutdown = "shutdown"
	// BoxAutoOffOutcomeFailed means the shutdown command failed.
	BoxAutoOffOutcomeFailed = "failed"
	// BoxAutoOffOutcomeAborted means the pre-shutdown re-check found late work
	// and called the shutdown off.
	BoxAutoOffOutcomeAborted = "aborted"
)

// BoxAutoOffEvent records one automatic shutdown attempt, with the readings
// and blockers that were current at the moment it fired.
type BoxAutoOffEvent struct {
	ID         int64           `gorm:"primaryKey;autoIncrement" json:"id"`
	OccurredAt time.Time       `gorm:"column:occurred_at;not null" json:"occurred_at"`
	Outcome    string          `gorm:"type:text;not null" json:"outcome"`
	Detail     json.RawMessage `gorm:"type:jsonb;not null;default:'{}'" json:"detail"`
}

// TableName returns the database table name for the BoxAutoOffEvent model.
func (BoxAutoOffEvent) TableName() string {
	return "box_auto_off_events"
}
```

- [ ] **Step 3: Add the table-name test**

Append to `internal/models/models_test.go`, matching the existing table-name test style:

```go
func TestBoxAutoOffEvent_TableName(t *testing.T) {
	if got := BoxAutoOffEvent{}.TableName(); got != "box_auto_off_events" {
		t.Errorf("TableName() = %q, want box_auto_off_events", got)
	}
}
```

- [ ] **Step 4: Run the tests and apply the migration**

Run: `go test ./internal/models/ -run BoxAutoOff -v`
Expected: PASS.

Run: `make migrate-up`
Expected: migration 042 applied. Verify:
`psql "postgres://botka:botka@localhost:5432/botka?sslmode=disable" -c '\d box_auto_off_events'`

- [ ] **Step 5: Commit**

```bash
git add migrations/042_box_auto_off_events.up.sql migrations/042_box_auto_off_events.down.sql internal/models/box_auto_off_event.go internal/models/models_test.go
git commit -m "feat(db): box_auto_off_events table"
```

---

### Task 7: Botka's own activity (`activity.go`)

**Files:**
- Create: `internal/boxoff/activity.go`
- Test: `internal/boxoff/activity_test.go`

**Interfaces:**
- Consumes: `runner.Runner.GetStatus()`, `claude.Registry.List()`, `models.Thread`, `models.Project`.
- Produces:
  ```go
  type ActivitySource interface {
      RunningTasks() []string
      BoxChatThreads(ctx context.Context) ([]string, error)
  }
  func NewAppActivity(tasks TaskStatusSource, chats ChatRegistry, db *gorm.DB) *AppActivity
  ```
  with `TaskStatusSource interface { GetStatus() runner.Status }` and
  `ChatRegistry interface { List() []claude.ProcessInfo }`. Task 8 consumes `ActivitySource`; Task 11 constructs `AppActivity`.

- [ ] **Step 1: Write the failing test**

Create `internal/boxoff/activity_test.go`:

```go
package boxoff

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"botka/internal/claude"
	"botka/internal/models"
	"botka/internal/runner"
)

type fakeTaskStatus struct{ status runner.Status }

func (f fakeTaskStatus) GetStatus() runner.Status { return f.status }

type fakeChatRegistry struct{ procs []claude.ProcessInfo }

func (f fakeChatRegistry) List() []claude.ProcessInfo { return f.procs }

func TestAppActivity_RunningTasks(t *testing.T) {
	act := NewAppActivity(fakeTaskStatus{status: runner.Status{
		ActiveTasks: []runner.ActiveTaskInfo{
			{TaskID: uuid.New(), TaskTitle: "fix the parser", ProjectName: "botka", StartedAt: time.Now()},
		},
	}}, fakeChatRegistry{}, nil)

	got := act.RunningTasks()
	if len(got) != 1 {
		t.Fatalf("RunningTasks() = %v, want one entry", got)
	}
	if got[0] == "" {
		t.Error("task label is empty; it is shown to the user as a blocker reason")
	}
}

func TestAppActivity_NoTasks(t *testing.T) {
	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{}, nil)

	if got := act.RunningTasks(); len(got) != 0 {
		t.Fatalf("RunningTasks() = %v, want empty", got)
	}
}

func TestAppActivity_BoxChatThreads_NoProcessesSkipsTheQuery(t *testing.T) {
	// db is nil on purpose: with an empty registry there is nothing to look up,
	// so touching the database at all would panic here.
	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{}, nil)

	got, err := act.BoxChatThreads(context.Background())
	if err != nil {
		t.Fatalf("BoxChatThreads() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("BoxChatThreads() = %v, want empty", got)
	}
}

func TestAppActivity_BoxChatThreads_OnlyBoxProjects(t *testing.T) {
	db := setupTestDB(t) // skips when botka_test is unavailable

	local := models.Project{ID: uuid.New(), Name: "botka", Path: "/home/pi/projects/botka"}
	remote := models.Project{ID: uuid.New(), Name: "render", Path: "box:/home/box/projects/render"}
	if err := db.Create(&local).Error; err != nil {
		t.Fatalf("create local project: %v", err)
	}
	if err := db.Create(&remote).Error; err != nil {
		t.Fatalf("create remote project: %v", err)
	}

	localThread := models.Thread{Title: "local chat", ProjectID: &local.ID}
	remoteThread := models.Thread{Title: "render chat", ProjectID: &remote.ID}
	if err := db.Create(&localThread).Error; err != nil {
		t.Fatalf("create local thread: %v", err)
	}
	if err := db.Create(&remoteThread).Error; err != nil {
		t.Fatalf("create remote thread: %v", err)
	}

	act := NewAppActivity(fakeTaskStatus{}, fakeChatRegistry{procs: []claude.ProcessInfo{
		{ThreadID: localThread.ID, Title: "local chat"},
		{ThreadID: remoteThread.ID, Title: "render chat"},
	}}, db)

	got, err := act.BoxChatThreads(context.Background())
	if err != nil {
		t.Fatalf("BoxChatThreads() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("BoxChatThreads() = %v, want only the box: thread", got)
	}
}
```

`setupTestDB` must follow whatever `internal/handlers` already does for
`botka_test` (connect via `DATABASE_TEST_URL`, `t.Skip` when unavailable,
`AutoMigrate` or migrate, clean up rows). Copy that helper into
`internal/boxoff/activity_test.go` rather than exporting it — read
`internal/handlers/*_test.go` for the existing shape first, and check the
actual field names on `claude.ProcessInfo` (`internal/claude/registry.go:10`)
before writing the literals above.

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/boxoff/ -run Activity -v`
Expected: FAIL — `undefined: NewAppActivity`.

- [ ] **Step 3: Write the implementation**

Create `internal/boxoff/activity.go`:

```go
package boxoff

import (
	"context"
	"fmt"

	"gorm.io/gorm"

	"botka/internal/claude"
	"botka/internal/runner"
)

// ActivitySource reports the work Botka itself has in flight. Both methods
// return human-readable labels because they end up in the UI as the reason a
// shutdown did not happen.
type ActivitySource interface {
	// RunningTasks lists the tasks the runner currently executes.
	RunningTasks() []string
	// BoxChatThreads lists running chat sessions whose project lives on Box.
	BoxChatThreads(ctx context.Context) ([]string, error)
}

// TaskStatusSource is the slice of *runner.Runner this package needs.
type TaskStatusSource interface {
	GetStatus() runner.Status
}

// ChatRegistry is the slice of claude.ProcessRegistry this package needs.
type ChatRegistry interface {
	List() []claude.ProcessInfo
}

// AppActivity answers the activity questions from the live runner, the chat
// process registry and the database.
type AppActivity struct {
	tasks TaskStatusSource
	chats ChatRegistry
	db    *gorm.DB
}

// NewAppActivity wires an ActivitySource over the running application.
func NewAppActivity(tasks TaskStatusSource, chats ChatRegistry, db *gorm.DB) *AppActivity {
	return &AppActivity{tasks: tasks, chats: chats, db: db}
}

// RunningTasks lists every task the runner has in flight — including tasks on
// projects unrelated to Box, because a shutdown mid-run costs the user a task
// either way.
func (a *AppActivity) RunningTasks() []string {
	if a.tasks == nil {
		return nil
	}
	status := a.tasks.GetStatus()
	labels := make([]string, 0, len(status.ActiveTasks))
	for _, t := range status.ActiveTasks {
		labels = append(labels, fmt.Sprintf("%s (%s)", t.TaskTitle, t.ProjectName))
	}
	return labels
}

// BoxChatThreads lists running chat sessions that spawned Claude Code on Box.
// A chat is on Box when its thread's project path carries the "box:" prefix
// (see internal/claude/remote.go).
//
// With no chat process running at all the database is never touched.
func (a *AppActivity) BoxChatThreads(ctx context.Context) ([]string, error) {
	if a.chats == nil {
		return nil, nil
	}
	procs := a.chats.List()
	if len(procs) == 0 {
		return nil, nil
	}
	if a.db == nil {
		return nil, fmt.Errorf("no database to resolve %d chat threads", len(procs))
	}

	ids := make([]int64, 0, len(procs))
	titles := make(map[int64]string, len(procs))
	for _, p := range procs {
		ids = append(ids, p.ThreadID)
		titles[p.ThreadID] = p.Title
	}

	var boxThreadIDs []int64
	err := a.db.WithContext(ctx).
		Table("threads").
		Joins("JOIN projects ON projects.id = threads.project_id").
		Where("threads.id IN ?", ids).
		Where("projects.path LIKE ?", claude.RemotePrefix+"%").
		Pluck("threads.id", &boxThreadIDs).Error
	if err != nil {
		return nil, fmt.Errorf("resolve box chat threads: %w", err)
	}

	labels := make([]string, 0, len(boxThreadIDs))
	for _, id := range boxThreadIDs {
		labels = append(labels, titles[id])
	}
	return labels, nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/boxoff/ -v`
Expected: PASS (the DB-backed subtest skips if `botka_test` is missing — run `make test-db` once if so).

- [ ] **Step 5: Commit**

```bash
git add internal/boxoff/activity.go internal/boxoff/activity_test.go
git commit -m "feat(boxoff): report Botka's own running tasks and Box chats"
```

---

### Task 8: The monitor loop (`monitor.go`)

**Files:**
- Create: `internal/boxoff/monitor.go`
- Test: `internal/boxoff/monitor_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2, 3, 4, 6, 7; `box.Shutdown` (Task 1).
- Produces:
  ```go
  type Prober interface { Probe(ctx context.Context) (BoxReadings, error) }
  type KukatkoSource interface { Fetch(ctx context.Context) (KukatkoReadings, error) }
  type Shutdowner interface { Shutdown(ctx context.Context) error }
  type Config struct { DB *gorm.DB; Prober Prober; Kukatko KukatkoSource; Activity ActivitySource; Shutdowner Shutdowner; Thresholds Thresholds }
  func NewMonitor(cfg Config) *Monitor
  func (m *Monitor) Start()
  func (m *Monitor) Stop()
  func (m *Monitor) ReloadSetting()
  func (m *Monitor) Snapshot() Snapshot
  func (m *Monitor) RunOnce(ctx context.Context) Evaluation   // one tick, exported for tests
  ```
  Task 10 consumes `Snapshot`; Task 11 calls `NewMonitor`/`Start`/`Stop`/`ReloadSetting`.

- [ ] **Step 1: Write the failing tests**

Create `internal/boxoff/monitor_test.go`. Drive `RunOnce` directly rather than
waiting on the ticker — the loop is a thin wrapper around it, and time-based
tests are flaky:

```go
package boxoff

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeProber struct {
	readings BoxReadings
	err      error
	calls    int
}

func (f *fakeProber) Probe(context.Context) (BoxReadings, error) {
	f.calls++
	return f.readings, f.err
}

type fakeKukatko struct {
	readings KukatkoReadings
	err      error
	calls    int
}

func (f *fakeKukatko) Fetch(context.Context) (KukatkoReadings, error) {
	f.calls++
	return f.readings, f.err
}

type fakeActivity struct {
	tasks []string
	chats []string
	err   error
}

func (f *fakeActivity) RunningTasks() []string { return f.tasks }
func (f *fakeActivity) BoxChatThreads(context.Context) ([]string, error) {
	return f.chats, f.err
}

type fakeShutdowner struct {
	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeShutdowner) Shutdown(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *fakeShutdowner) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// newTestMonitor builds a monitor over fakes with a clean, idle world.
// The nil DB is fine: event persistence is best-effort and must not panic.
func newTestMonitor(t *testing.T) (*Monitor, *fakeProber, *fakeKukatko, *fakeActivity, *fakeShutdowner) {
	t.Helper()

	prober := &fakeProber{readings: BoxReadings{UptimeSeconds: 33390, Load1: 0.01, Threads: 24}}
	kuk := &fakeKukatko{readings: KukatkoReadings{StatusCode: 200}}
	act := &fakeActivity{}
	sd := &fakeShutdowner{}

	m := NewMonitor(Config{
		Prober:     prober,
		Kukatko:    kuk,
		Activity:   act,
		Shutdowner: sd,
		Thresholds: testThresholds(),
	})
	m.setEnabled(true)

	return m, prober, kuk, act, sd
}

func TestMonitor_ShutsDownAfterThreeCleanChecks(t *testing.T) {
	m, _, _, _, sd := newTestMonitor(t)
	ctx := context.Background()

	for i := 1; i <= 2; i++ {
		if ev := m.RunOnce(ctx); !ev.Idle {
			t.Fatalf("check %d: not idle, blockers %v", i, blockerNames(ev))
		}
		if sd.count() != 0 {
			t.Fatalf("shut down after %d clean checks, want 3", i)
		}
	}

	m.RunOnce(ctx)

	if sd.count() != 1 {
		t.Fatalf("shutdown calls = %d, want exactly 1", sd.count())
	}
	if got := m.Snapshot().Streak; got != 0 {
		t.Errorf("streak = %d after shutdown, want 0", got)
	}
}

func TestMonitor_BlockerResetsTheStreak(t *testing.T) {
	m, _, kuk, _, sd := newTestMonitor(t)
	ctx := context.Background()

	m.RunOnce(ctx)
	m.RunOnce(ctx)

	kuk.readings = KukatkoReadings{StatusCode: 200, Queued: 4}
	m.RunOnce(ctx)

	if got := m.Snapshot().Streak; got != 0 {
		t.Fatalf("streak = %d after a blocker, want 0", got)
	}

	kuk.readings = KukatkoReadings{StatusCode: 200}
	m.RunOnce(ctx)
	m.RunOnce(ctx)

	if sd.count() != 0 {
		t.Fatalf("shut down after 2 clean checks following a reset, want 3")
	}
}

func TestMonitor_DisabledDoesNotProbe(t *testing.T) {
	m, prober, kuk, _, sd := newTestMonitor(t)
	m.setEnabled(false)

	ev := m.RunOnce(context.Background())

	if prober.calls != 0 || kuk.calls != 0 {
		t.Errorf("probed %d / scraped %d while disabled, want 0/0", prober.calls, kuk.calls)
	}
	if sd.count() != 0 {
		t.Error("shut down while disabled")
	}
	if len(ev.Blockers) != 1 || ev.Blockers[0].Name != BlockerDisabled {
		t.Errorf("blockers = %v, want [disabled]", blockerNames(ev))
	}
}

func TestMonitor_ProbeFailureIsOfflineNotIdle(t *testing.T) {
	m, prober, kuk, _, sd := newTestMonitor(t)
	prober.err = errors.New("no route to host")

	for i := 0; i < 5; i++ {
		m.RunOnce(context.Background())
	}

	if sd.count() != 0 {
		t.Fatal("shut down while Box was unreachable")
	}
	if kuk.calls != 0 {
		t.Error("scraped Kukátko even though the probe failed; offline is terminal")
	}
	ev := m.Snapshot().LastEvaluation
	if ev == nil || len(ev.Blockers) != 1 || ev.Blockers[0].Name != BlockerBoxOffline {
		t.Errorf("blockers = %v, want [box_offline]", ev)
	}
}

func TestMonitor_KukatkoFailureBlocksForever(t *testing.T) {
	m, _, kuk, _, sd := newTestMonitor(t)
	kuk.err = errors.New("bad gateway")
	kuk.readings = KukatkoReadings{StatusCode: 502, BodyExcerpt: "upstream is down"}

	for i := 0; i < 5; i++ {
		m.RunOnce(context.Background())
	}

	if sd.count() != 0 {
		t.Fatal("shut down without a trustworthy Kukátko reading")
	}
	ev := m.Snapshot().LastEvaluation
	if ev == nil || ev.Kukatko == nil || ev.Kukatko.BodyExcerpt == "" {
		t.Error("the failing response detail must survive into the snapshot for the UI")
	}
}

func TestMonitor_LateTaskAbortsTheShutdown(t *testing.T) {
	m, _, _, act, sd := newTestMonitor(t)
	ctx := context.Background()

	m.RunOnce(ctx)
	m.RunOnce(ctx)

	// A task is claimed between the readings and the shutdown: the monitor
	// re-checks its own cheap, local conditions right before firing.
	m.beforeShutdown = func() { act.tasks = []string{"a task that just started"} }

	m.RunOnce(ctx)

	if sd.count() != 0 {
		t.Fatal("shut down even though a task started during the evaluation")
	}
	if got := m.Snapshot().Streak; got != 0 {
		t.Errorf("streak = %d after an abort, want 0", got)
	}
}

func TestMonitor_ShutdownErrorIsRecordedNotSwallowed(t *testing.T) {
	m, _, _, _, sd := newTestMonitor(t)
	sd.err = errors.New("permission denied")
	ctx := context.Background()

	m.RunOnce(ctx)
	m.RunOnce(ctx)
	m.RunOnce(ctx)

	if sd.count() != 1 {
		t.Fatalf("shutdown calls = %d, want 1", sd.count())
	}
	snap := m.Snapshot()
	if snap.LastError == "" {
		t.Error("a failed shutdown must be visible; LastError is empty")
	}
}

func TestMonitor_SnapshotKeepsRecentEvaluations(t *testing.T) {
	m, _, _, _, _ := newTestMonitor(t)

	for i := 0; i < recentEvaluations+5; i++ {
		m.RunOnce(context.Background())
	}

	if got := len(m.Snapshot().Recent); got != recentEvaluations {
		t.Fatalf("Recent = %d entries, want the ring to cap at %d", got, recentEvaluations)
	}
}

func TestMonitor_SnapshotCarriesTheCountdown(t *testing.T) {
	m, _, _, _, _ := newTestMonitor(t)

	m.RunOnce(context.Background())

	snap := m.Snapshot()
	if snap.EarliestShutdownAt == nil {
		t.Fatal("EarliestShutdownAt is nil after a clean check")
	}
	if !snap.EarliestShutdownAt.After(time.Now()) {
		t.Error("EarliestShutdownAt is in the past")
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/boxoff/ -run Monitor -v`
Expected: FAIL — `undefined: NewMonitor`.

- [ ] **Step 3: Write the implementation**

Create `internal/boxoff/monitor.go`:

```go
package boxoff

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"gorm.io/gorm"

	"botka/internal/models"
)

const (
	// settingKey is the app_settings row holding the on/off switch.
	settingKey = "box_auto_off"
	// recentEvaluations is how many past evaluations the snapshot keeps for
	// the UI. They are in memory only: transient by nature, and refilled
	// within one interval after a restart.
	recentEvaluations = 20
	// firstTickDelay is how long after startup the first evaluation runs, so
	// the UI is not blank for a whole interval.
	firstTickDelay = 30 * time.Second
	// gatherTimeout bounds one full round of readings.
	gatherTimeout = 60 * time.Second
	// shutdownTimeout bounds the shutdown command itself.
	shutdownTimeout = 30 * time.Second
)

// Prober samples Box over SSH.
type Prober interface {
	Probe(ctx context.Context) (BoxReadings, error)
}

// KukatkoSource reads Kukátko's job queue depth.
type KukatkoSource interface {
	Fetch(ctx context.Context) (KukatkoReadings, error)
}

// Shutdowner powers Box off.
type Shutdowner interface {
	Shutdown(ctx context.Context) error
}

// Config wires a Monitor. DB may be nil, in which case events are not
// persisted; everything else is required.
type Config struct {
	DB         *gorm.DB
	Prober     Prober
	Kukatko    KukatkoSource
	Activity   ActivitySource
	Shutdowner Shutdowner
	Thresholds Thresholds
}

// Snapshot is the monitor's state as the API exposes it.
type Snapshot struct {
	Enabled            bool         `json:"enabled"`
	Streak             int          `json:"streak"`
	RequiredChecks     int          `json:"required_checks"`
	IntervalSeconds    int          `json:"interval_seconds"`
	NextCheckAt        *time.Time   `json:"next_check_at"`
	EarliestShutdownAt *time.Time   `json:"earliest_shutdown_at"`
	LastEvaluation     *Evaluation  `json:"last_evaluation"`
	Recent             []Evaluation `json:"recent"`
	LastError          string       `json:"last_error,omitempty"`
}

// Monitor evaluates Box's idleness on a ticker and shuts it down once the
// conditions have held for Thresholds.IdleChecks consecutive evaluations.
type Monitor struct {
	cfg Config

	mu          sync.RWMutex
	enabled     bool
	streak      int
	nextCheckAt time.Time
	last        *Evaluation
	recent      []Evaluation
	lastErr     string

	// beforeShutdown is a test seam fired after the decision to shut down and
	// before the pre-shutdown re-check, so a test can make work appear in the
	// window the re-check exists to close.
	beforeShutdown func()

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewMonitor returns a monitor for the given wiring. Call Start to run it.
func NewMonitor(cfg Config) *Monitor {
	return &Monitor{cfg: cfg, stopCh: make(chan struct{})}
}

// Start reads the setting and launches the evaluation loop.
func (m *Monitor) Start() {
	m.ReloadSetting()
	m.wg.Add(1)
	go m.loop()
}

// Stop halts the loop and waits for it to finish.
func (m *Monitor) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// ReloadSetting re-reads box_auto_off from the database, so flipping the
// switch takes effect without waiting for the next tick.
func (m *Monitor) ReloadSetting() {
	if m.cfg.DB == nil {
		return
	}
	var value string
	err := m.cfg.DB.Table("app_settings").Where("key = ?", settingKey).Pluck("value", &value).Error
	if err != nil {
		slog.Warn("box auto-off: reading the setting failed", "error", err)
		return
	}
	m.setEnabled(value == "true")
}

// setEnabled sets the switch without touching the database.
func (m *Monitor) setEnabled(on bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !on {
		m.streak = 0
	}
	m.enabled = on
}

// loop runs an evaluation firstTickDelay after start and every Interval after
// that.
func (m *Monitor) loop() {
	defer m.wg.Done()

	interval := m.cfg.Thresholds.Interval
	slog.Info("box auto-off monitor started", "interval", interval, "idle_checks", m.cfg.Thresholds.IdleChecks)

	timer := time.NewTimer(firstTickDelay)
	defer timer.Stop()
	m.setNextCheckAt(time.Now().Add(firstTickDelay))

	for {
		select {
		case <-m.stopCh:
			slog.Info("box auto-off monitor stopped")
			return
		case <-timer.C:
			ctx, cancel := context.WithTimeout(context.Background(), gatherTimeout)
			m.RunOnce(ctx)
			cancel()
			timer.Reset(interval)
			m.setNextCheckAt(time.Now().Add(interval))
		}
	}
}

// RunOnce performs one full evaluation and, when the streak is complete,
// shuts Box down. It returns the evaluation it acted on.
func (m *Monitor) RunOnce(ctx context.Context) Evaluation {
	in := m.gather(ctx)
	ev := Evaluate(time.Now(), in, m.cfg.Thresholds)

	m.mu.Lock()
	if ev.Idle {
		m.streak++
	} else {
		m.streak = 0
	}
	streak := m.streak
	m.last = &ev
	m.recent = append(m.recent, ev)
	if len(m.recent) > recentEvaluations {
		m.recent = m.recent[len(m.recent)-recentEvaluations:]
	}
	m.mu.Unlock()

	if !ev.Idle || streak < m.cfg.Thresholds.IdleChecks {
		return ev
	}

	m.fireShutdown(ctx, ev)
	return ev
}

// fireShutdown re-checks the cheap local conditions and, if they still hold,
// powers Box off. The streak is reset either way: after a shutdown there is
// nothing to count, and after an abort the count must start over.
func (m *Monitor) fireShutdown(ctx context.Context, ev Evaluation) {
	if m.beforeShutdown != nil {
		m.beforeShutdown()
	}

	defer func() {
		m.mu.Lock()
		m.streak = 0
		m.mu.Unlock()
	}()

	// Gathering the readings took seconds, during which the runner may have
	// claimed a task or a chat may have started on Box. Those two checks are
	// local and cheap, so there is no reason to act on stale ones.
	if blocker, ok := m.recheckLocalWork(ctx); !ok {
		slog.Info("box auto-off: shutdown aborted", "reason", blocker.Detail)
		m.setLastError("")
		m.recordEvent(models.BoxAutoOffOutcomeAborted, ev, blocker.Detail)
		return
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := m.cfg.Shutdowner.Shutdown(shutdownCtx); err != nil {
		slog.Error("box auto-off: shutdown failed", "error", err)
		m.setLastError(err.Error())
		m.recordEvent(models.BoxAutoOffOutcomeFailed, ev, err.Error())
		return
	}

	slog.Info("box auto-off: box shut down",
		"uptime_seconds", ev.Box.UptimeSeconds,
		"load1", ev.Box.Load1,
		"gpu_max_percent", ev.Box.GPUMaxPercent,
	)
	m.setLastError("")
	m.recordEvent(models.BoxAutoOffOutcomeShutdown, ev, "")
}

// recheckLocalWork re-runs the two conditions that can change between the
// readings and the shutdown command. A false return carries the blocker.
func (m *Monitor) recheckLocalWork(ctx context.Context) (Blocker, bool) {
	if tasks := m.cfg.Activity.RunningTasks(); len(tasks) > 0 {
		return Blocker{Name: BlockerBotkaTasks, Detail: "a task started during the evaluation"}, false
	}
	chats, err := m.cfg.Activity.BoxChatThreads(ctx)
	if err != nil {
		return Blocker{Name: BlockerBotkaBoxChats, Detail: "could not be determined: " + err.Error()}, false
	}
	if len(chats) > 0 {
		return Blocker{Name: BlockerBotkaBoxChats, Detail: "a chat started on box during the evaluation"}, false
	}
	return Blocker{}, true
}

// gather collects every reading the evaluation needs. Disabled and offline are
// terminal, so neither the scrape nor the activity lookup runs in those cases.
func (m *Monitor) gather(ctx context.Context) Inputs {
	in := Inputs{Enabled: m.isEnabled()}
	if !in.Enabled {
		return in
	}

	readings, err := m.cfg.Prober.Probe(ctx)
	if err != nil {
		in.ProbeError = err.Error()
		return in
	}
	in.BoxOnline = true
	in.Box = readings

	kukatko, err := m.cfg.Kukatko.Fetch(ctx)
	in.Kukatko = kukatko
	in.KukatkoOK = err == nil

	in.RunningTasks = m.cfg.Activity.RunningTasks()
	chats, err := m.cfg.Activity.BoxChatThreads(ctx)
	if err != nil {
		in.ActivityError = err.Error()
	} else {
		in.BoxChats = chats
	}

	return in
}

// Snapshot returns the monitor's current state for the API.
func (m *Monitor) Snapshot() Snapshot {
	m.mu.RLock()
	defer m.mu.RUnlock()

	snap := Snapshot{
		Enabled:         m.enabled,
		Streak:          m.streak,
		RequiredChecks:  m.cfg.Thresholds.IdleChecks,
		IntervalSeconds: int(m.cfg.Thresholds.Interval.Seconds()),
		LastEvaluation:  m.last,
		Recent:          reversed(m.recent),
		LastError:       m.lastErr,
	}
	if !m.nextCheckAt.IsZero() {
		next := m.nextCheckAt
		snap.NextCheckAt = &next
		if m.last != nil {
			snap.EarliestShutdownAt = EarliestShutdownAt(time.Now(), *m.last, m.streak, next, m.cfg.Thresholds)
		}
	}
	return snap
}

// reversed returns the evaluations newest-first without aliasing the ring.
func reversed(in []Evaluation) []Evaluation {
	out := make([]Evaluation, 0, len(in))
	for i := len(in) - 1; i >= 0; i-- {
		out = append(out, in[i])
	}
	return out
}

// isEnabled reports the switch state.
func (m *Monitor) isEnabled() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.enabled
}

// setNextCheckAt records when the next evaluation is due.
func (m *Monitor) setNextCheckAt(t time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.nextCheckAt = t
}

// setLastError records the most recent shutdown error, or clears it.
func (m *Monitor) setLastError(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastErr = msg
}

// recordEvent persists one shutdown attempt. It is best-effort: a database
// error is logged, never propagated, because losing a history row must not
// change what the monitor does.
func (m *Monitor) recordEvent(outcome string, ev Evaluation, note string) {
	if m.cfg.DB == nil {
		return
	}

	detail, err := json.Marshal(struct {
		Evaluation Evaluation `json:"evaluation"`
		Note       string     `json:"note,omitempty"`
	}{Evaluation: ev, Note: note})
	if err != nil {
		slog.Warn("box auto-off: encoding the event failed", "error", err)
		return
	}

	row := models.BoxAutoOffEvent{OccurredAt: time.Now(), Outcome: outcome, Detail: detail}
	if err := m.cfg.DB.Create(&row).Error; err != nil {
		slog.Warn("box auto-off: recording the event failed", "error", err, "outcome", outcome)
	}
}
```

Also add the small `Shutdowner` implementation used in production at the
bottom of `monitor.go`:

```go
// SSHShutdowner powers Box off over SSH, sharing the teardown-detection logic
// with the manual shutdown button.
type SSHShutdowner struct {
	run    box.RunFunc
	target string
}

// NewSSHShutdowner returns a Shutdowner running commands with run against
// sshTarget ("user@host").
func NewSSHShutdowner(run box.RunFunc, sshTarget string) *SSHShutdowner {
	return &SSHShutdowner{run: run, target: sshTarget}
}

// Shutdown powers Box off.
func (s *SSHShutdowner) Shutdown(ctx context.Context) error {
	return box.Shutdown(ctx, s.run, s.target)
}
```

(add `"botka/internal/box"` to the imports).

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/boxoff/ -race -v`
Expected: PASS, no race warnings.

- [ ] **Step 5: Commit**

```bash
git add internal/boxoff/monitor.go internal/boxoff/monitor_test.go
git commit -m "feat(boxoff): monitor loop with idle streak and pre-shutdown re-check"
```

---

### Task 9: The `box_auto_off` setting

**Files:**
- Modify: `internal/handlers/settings.go` (`Get` near line 45, `settingsUpdateRequest` near line 66, `Update` near line 72)
- Test: `internal/handlers/settings_test.go` (create if absent; otherwise append)

**Interfaces:**
- Produces: `GET /api/v1/settings` returns `box_auto_off` as a JSON boolean; `PUT /api/v1/settings` accepts `{"box_auto_off": true}` and fires `onChange("box_auto_off", "true")`.

- [ ] **Step 1: Write the failing test**

Append to `internal/handlers/settings_test.go` (follow the existing handler-test setup: `gin.SetMode(gin.TestMode)`, a `botka_test` DB via the package helper, `httptest`):

```go
func TestSettingsHandler_UpdateBoxAutoOff(t *testing.T) {
	db := setupTestDB(t)
	h := NewSettingsHandler(db)

	var gotKey, gotValue string
	h.SetOnChange(func(key, value string) { gotKey, gotValue = key, value })

	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(`{"box_auto_off": true}`))
	req.Header.Set("Content-Type", "application/json")
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.Data["box_auto_off"] != true {
		t.Errorf("box_auto_off = %v (%T), want the boolean true", resp.Data["box_auto_off"], resp.Data["box_auto_off"])
	}
	if gotKey != "box_auto_off" || gotValue != "true" {
		t.Errorf("onChange(%q, %q), want (box_auto_off, true)", gotKey, gotValue)
	}
}

func TestSettingsHandler_BoxAutoOffRoundTripsFalse(t *testing.T) {
	db := setupTestDB(t)
	h := NewSettingsHandler(db)
	router := gin.New()
	RegisterSettingsRoutes(router.Group("/api/v1"), h)

	for _, want := range []bool{true, false} {
		body := fmt.Sprintf(`{"box_auto_off": %t}`, want)
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPut, "/api/v1/settings", strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		router.ServeHTTP(w, req)

		var resp struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if resp.Data["box_auto_off"] != want {
			t.Errorf("box_auto_off = %v, want %t", resp.Data["box_auto_off"], want)
		}
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/handlers/ -run BoxAutoOff -v`
Expected: FAIL — the response carries the string `"true"`, not a boolean, and `onChange` never fires.

- [ ] **Step 3: Implement**

In `internal/handlers/settings.go`:

```go
// boxAutoOffSettingKey is the app_settings row holding the Box auto-off switch.
const boxAutoOffSettingKey = "box_auto_off"
```

In `Get`, replace the `if row.Key == "max_workers"` chain with a switch that also
special-cases the new key, so the frontend gets a real boolean:

```go
	for _, row := range rows {
		switch row.Key {
		case "max_workers":
			n, err := strconv.Atoi(row.Value)
			if err == nil {
				result["max_workers"] = n
			} else {
				result["max_workers"] = row.Value
			}
		case boxAutoOffSettingKey:
			result[boxAutoOffSettingKey] = row.Value == "true"
		default:
			result[row.Key] = row.Value
		}
	}
```

Extend the request struct and `Update`:

```go
type settingsUpdateRequest struct {
	MaxWorkers *int  `json:"max_workers"`
	BoxAutoOff *bool `json:"box_auto_off"`
}
```

```go
	if req.BoxAutoOff != nil {
		val := strconv.FormatBool(*req.BoxAutoOff)
		if err := h.db.Save(&models.Setting{Key: boxAutoOffSettingKey, Value: val}).Error; err != nil {
			respondError(c, http.StatusInternalServerError, "failed to save setting")
			return
		}
		if h.onChange != nil {
			h.onChange(boxAutoOffSettingKey, val)
		}
	}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/handlers/ -run 'Settings' -v`
Expected: PASS, including the pre-existing `max_workers` tests.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/settings.go internal/handlers/settings_test.go
git commit -m "feat(settings): box_auto_off switch"
```

---

### Task 10: `GET /api/v1/box/auto-off`

**Files:**
- Modify: `internal/handlers/box.go` (`BoxHandler` struct, `NewBoxHandler`, `RegisterBoxRoutes`)
- Test: `internal/handlers/box_test.go` (append)

**Interfaces:**
- Consumes: `boxoff.Monitor.Snapshot()` (Task 8), `models.BoxAutoOffEvent` (Task 6).
- Produces: `(*BoxHandler).SetAutoOffMonitor(m AutoOffSnapshotter)`, `(*BoxHandler).AutoOff(c *gin.Context)`, route `GET /box/auto-off`. Task 11 calls `SetAutoOffMonitor`; Task 12 consumes the payload.

- [ ] **Step 1: Write the failing test**

Append to `internal/handlers/box_test.go`:

```go
type fakeSnapshotter struct{ snap boxoff.Snapshot }

func (f fakeSnapshotter) Snapshot() boxoff.Snapshot { return f.snap }

func TestBoxHandler_AutoOff_Shape(t *testing.T) {
	gin.SetMode(gin.TestMode)

	next := time.Now().Add(7 * time.Minute)
	earliest := next.Add(20 * time.Minute)
	h := NewBoxHandler(nil, "10.0.0.1", "panbotka", "/bin/true")
	h.SetAutoOffMonitor(fakeSnapshotter{snap: boxoff.Snapshot{
		Enabled:            true,
		Streak:             1,
		RequiredChecks:     3,
		IntervalSeconds:    600,
		NextCheckAt:        &next,
		EarliestShutdownAt: &earliest,
		LastEvaluation: &boxoff.Evaluation{
			CheckedAt: time.Now(),
			Blockers:  []boxoff.Blocker{{Name: boxoff.BlockerKukatkoQueue, Detail: "12 queued, 1 running"}},
			Kukatko:   &boxoff.KukatkoReadings{StatusCode: 200, Queued: 12, Running: 1},
		},
	}})

	router := gin.New()
	RegisterBoxRoutes(router.Group("/api/v1"), h)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/box/auto-off", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data struct {
			Enabled            bool    `json:"enabled"`
			Streak             int     `json:"streak"`
			RequiredChecks     int     `json:"required_checks"`
			EarliestShutdownAt *string `json:"earliest_shutdown_at"`
			LastEvaluation     *struct {
				Blockers []struct {
					Name   string `json:"name"`
					Detail string `json:"detail"`
				} `json:"blockers"`
			} `json:"last_evaluation"`
			Events []any `json:"events"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if !resp.Data.Enabled || resp.Data.Streak != 1 || resp.Data.RequiredChecks != 3 {
		t.Errorf("got %+v", resp.Data)
	}
	if resp.Data.EarliestShutdownAt == nil {
		t.Error("earliest_shutdown_at missing; the UI counts down off it")
	}
	if resp.Data.LastEvaluation == nil || len(resp.Data.LastEvaluation.Blockers) != 1 {
		t.Fatal("blockers missing from the payload")
	}
	if resp.Data.LastEvaluation.Blockers[0].Detail != "12 queued, 1 running" {
		t.Errorf("blocker detail = %q", resp.Data.LastEvaluation.Blockers[0].Detail)
	}
	if resp.Data.Events == nil {
		t.Error("events must be an empty array, not null, so the frontend can map it")
	}
}

func TestBoxHandler_AutoOff_WithoutMonitor(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	RegisterBoxRoutes(router.Group("/api/v1"), NewBoxHandler(nil, "10.0.0.1", "panbotka", "/bin/true"))

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/box/auto-off", nil))

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 when the monitor is not wired", w.Code)
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/handlers/ -run AutoOff -v`
Expected: FAIL — `undefined: SetAutoOffMonitor`.

- [ ] **Step 3: Implement**

In `internal/handlers/box.go`, add to the `BoxHandler` struct:

```go
	// autoOff exposes the auto-off monitor's state. Nil when the monitor is
	// not wired (tests, or a build without it), in which case the endpoint
	// reports 503 rather than pretending the feature is off.
	autoOff AutoOffSnapshotter
```

and:

```go
// AutoOffSnapshotter is the slice of the auto-off monitor this handler needs.
type AutoOffSnapshotter interface {
	Snapshot() boxoff.Snapshot
}

// SetAutoOffMonitor wires the auto-off monitor into the handler.
func (h *BoxHandler) SetAutoOffMonitor(m AutoOffSnapshotter) {
	h.autoOff = m
}

// autoOffEventsLimit is how many past shutdown attempts the endpoint returns.
const autoOffEventsLimit = 20

// AutoOff reports the auto-off monitor's current state: the switch, the idle
// streak, when the next check runs, the earliest moment a shutdown could
// happen, the last evaluation with its blockers, and the recent shutdown
// attempts.
func (h *BoxHandler) AutoOff(c *gin.Context) {
	if h.autoOff == nil {
		respondError(c, http.StatusServiceUnavailable, "box auto-off monitor is not running")
		return
	}

	snap := h.autoOff.Snapshot()

	events := make([]models.BoxAutoOffEvent, 0, autoOffEventsLimit)
	if h.db != nil {
		if err := h.db.WithContext(c.Request.Context()).
			Order("occurred_at DESC").
			Limit(autoOffEventsLimit).
			Find(&events).Error; err != nil {
			// History is decoration; the live state is the point. Log and
			// return the rest rather than failing the whole endpoint.
			slog.Warn("box auto-off: reading events failed", "error", err)
		}
	}

	respondOK(c, gin.H{
		"enabled":              snap.Enabled,
		"streak":               snap.Streak,
		"required_checks":      snap.RequiredChecks,
		"interval_seconds":     snap.IntervalSeconds,
		"next_check_at":        snap.NextCheckAt,
		"earliest_shutdown_at": snap.EarliestShutdownAt,
		"last_evaluation":      snap.LastEvaluation,
		"recent":               snap.Recent,
		"events":               events,
		"last_error":           snap.LastError,
	})
}
```

Register the route in `RegisterBoxRoutes`:

```go
	box.GET("/auto-off", h.AutoOff)
```

Add `"log/slog"`, `"botka/internal/boxoff"` to the imports.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/handlers/ -run Box -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handlers/box.go internal/handlers/box_test.go
git commit -m "feat(api): GET /box/auto-off exposing the monitor state"
```

---

### Task 11: Wire it up in `main.go`

**Files:**
- Modify: `cmd/server/main.go` (after the `scheduleScheduler` block near line 172, and near `boxHandler` at line 391)

**Interfaces:**
- Consumes: Tasks 3, 4, 7, 8, 10 and the config from Task 5.

- [ ] **Step 1: Construct and start the monitor**

After the `scheduleScheduler` block in `cmd/server/main.go`:

```go
	// Box auto-off: shut the build machine down once it has been idle long
	// enough. The SSH target matches the Waker's, so probe, wake and shutdown
	// all address Box the same way.
	boxSSHTarget := boxWaker.SSHTarget()
	boxRun := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return exec.CommandContext(ctx, name, args...).CombinedOutput() //nolint:gosec // args are controlled
	}
	autoOffMonitor := boxoff.NewMonitor(boxoff.Config{
		DB:         db,
		Prober:     boxoff.NewSSHProbe(boxRun, boxSSHTarget),
		Kukatko:    boxoff.NewMetricsClient(cfg.KukatkoMetricsURL),
		Activity:   boxoff.NewAppActivity(taskRunner, claude.Registry, db),
		Shutdowner: boxoff.NewSSHShutdowner(boxRun, boxSSHTarget),
		Thresholds: boxoff.Thresholds{
			Interval:   cfg.BoxAutoOffInterval,
			MinUptime:  cfg.BoxAutoOffMinUptime,
			IdleChecks: cfg.BoxAutoOffIdleChecks,
			GPUPercent: cfg.BoxAutoOffGPUPercent,
			Load1:      cfg.BoxAutoOffLoad1,
		},
	})
	autoOffMonitor.Start()
	defer autoOffMonitor.Stop()
```

Add `"os/exec"`, `"botka/internal/boxoff"` to the imports (`claude` is already imported).

- [ ] **Step 2: Wire the handler and the settings callback**

Next to `boxHandler := handlers.NewBoxHandler(...)` (line ~391):

```go
	boxHandler.SetAutoOffMonitor(autoOffMonitor)
```

Find the existing `settingsHandler.SetOnChange(...)` call and extend its
callback so flipping the switch takes effect at once rather than at the next
tick — keep whatever it already does for `max_workers`:

```go
		case "box_auto_off":
			autoOffMonitor.ReloadSetting()
```

- [ ] **Step 3: Verify the build and the whole suite**

Run: `make check`
Expected: PASS.

- [ ] **Step 4: Verify against the real world, without shutting Box down**

The monitor is armed only by the setting, which defaults to off, so starting
the binary is safe. Confirm the probe and scrape work against the live
systems by running the two commands the code runs:

```bash
ssh -o BatchMode=yes -o ConnectTimeout=5 box "cut -d' ' -f1 /proc/uptime; cut -d' ' -f1 /proc/loadavg; nproc; for i in 1 2 3; do nvidia-smi --query-gpu=utilization.gpu --format=csv,noheader,nounits; sleep 1; done"
curl -s https://fotky.kotrzina.cz/metrics | grep '^kukatko_jobs_queue_depth{'
```

Expected: six-ish numeric lines from the first, at least one `state="…"`
sample from the second. Paste both outputs into the commit or the summary as
evidence.

- [ ] **Step 5: Commit**

```bash
git add cmd/server/main.go
git commit -m "feat(box): start the auto-off monitor"
```

---

### Task 12: Frontend types, API client and hook

**Files:**
- Modify: `frontend/src/types/index.ts` (near `BoxStatus`, line ~504; and `ServerSettings`, line ~215)
- Modify: `frontend/src/api/client.ts` (near `fetchBoxStatus`, line ~1144; and the default-export object, line ~1522)
- Create: `frontend/src/hooks/useBoxAutoOff.ts`
- Test: `frontend/src/hooks/useBoxAutoOff.test.ts`

**Interfaces:**
- Produces: `BoxAutoOffStatus`, `BoxAutoOffEvaluation`, `BoxAutoOffBlocker`, `BoxAutoOffEvent`; `fetchBoxAutoOff(): Promise<BoxAutoOffStatus>`; `useBoxAutoOff()` returning `{ status, loading, error, countdown, refresh, setEnabled }`. Task 13 consumes the hook.

- [ ] **Step 1: Add the types**

In `frontend/src/types/index.ts`:

```ts
export interface BoxAutoOffBlocker {
  name: string
  detail: string
}

export interface BoxAutoOffBoxReadings {
  uptime_seconds: number
  load1: number
  threads: number
  gpu_max_percent: number
}

export interface BoxAutoOffKukatkoReadings {
  status_code: number
  queued: number
  running: number
  error?: string
  body_excerpt?: string
}

export interface BoxAutoOffEvaluation {
  checked_at: string
  idle: boolean
  blockers: BoxAutoOffBlocker[] | null
  box: BoxAutoOffBoxReadings | null
  kukatko: BoxAutoOffKukatkoReadings | null
}

export interface BoxAutoOffEvent {
  id: number
  occurred_at: string
  outcome: 'shutdown' | 'failed' | 'aborted'
}

export interface BoxAutoOffStatus {
  enabled: boolean
  streak: number
  required_checks: number
  interval_seconds: number
  next_check_at: string | null
  earliest_shutdown_at: string | null
  last_evaluation: BoxAutoOffEvaluation | null
  recent: BoxAutoOffEvaluation[] | null
  events: BoxAutoOffEvent[]
  last_error?: string
}
```

Add `box_auto_off?: boolean` to `ServerSettings`.

- [ ] **Step 2: Add the client call**

In `frontend/src/api/client.ts`, next to `fetchBoxStatus`:

```ts
export function fetchBoxAutoOff(): Promise<BoxAutoOffStatus> {
  return requestData<BoxAutoOffStatus>('/box/auto-off')
}
```

Add `BoxAutoOffStatus` to the type import on line 1 and `fetchBoxAutoOff` to
the default-export object.

- [ ] **Step 3: Write the failing hook test**

Create `frontend/src/hooks/useBoxAutoOff.test.ts`. Mock `../api/client` the
way the existing hook tests in this repo do (check a neighbouring
`*.test.ts` for the exact `vi.mock` shape first):

```ts
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useBoxAutoOff } from './useBoxAutoOff'
import * as client from '../api/client'

vi.mock('../api/client')

const baseStatus = {
  enabled: true,
  streak: 1,
  required_checks: 3,
  interval_seconds: 600,
  next_check_at: null,
  earliest_shutdown_at: null,
  last_evaluation: null,
  recent: null,
  events: [],
}

describe('useBoxAutoOff', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true })
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('loads the status', async () => {
    vi.mocked(client.fetchBoxAutoOff).mockResolvedValue({ ...baseStatus })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.status?.streak).toBe(1))
    expect(result.current.error).toBeNull()
  })

  it('counts down to earliest_shutdown_at and ticks', async () => {
    vi.mocked(client.fetchBoxAutoOff).mockResolvedValue({
      ...baseStatus,
      earliest_shutdown_at: new Date(Date.now() + 90_000).toISOString(),
    })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.countdown).not.toBeNull())
    const first = result.current.countdown as number

    act(() => {
      vi.advanceTimersByTime(5_000)
    })

    expect(result.current.countdown as number).toBeLessThan(first)
  })

  it('has no countdown when the server gives none', async () => {
    vi.mocked(client.fetchBoxAutoOff).mockResolvedValue({ ...baseStatus })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.status).not.toBeNull())
    expect(result.current.countdown).toBeNull()
  })

  it('clamps a countdown that has already elapsed to zero', async () => {
    vi.mocked(client.fetchBoxAutoOff).mockResolvedValue({
      ...baseStatus,
      earliest_shutdown_at: new Date(Date.now() - 30_000).toISOString(),
    })

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.countdown).toBe(0))
  })

  it('surfaces a fetch failure', async () => {
    vi.mocked(client.fetchBoxAutoOff).mockRejectedValue(new Error('boom'))

    const { result } = renderHook(() => useBoxAutoOff())

    await waitFor(() => expect(result.current.error).toBe('boom'))
  })

  it('writes the switch through the settings endpoint and refreshes', async () => {
    vi.mocked(client.fetchBoxAutoOff).mockResolvedValue({ ...baseStatus, enabled: false })
    vi.mocked(client.updateServerSettings).mockResolvedValue({ box_auto_off: true } as never)

    const { result } = renderHook(() => useBoxAutoOff())
    await waitFor(() => expect(result.current.status).not.toBeNull())

    vi.mocked(client.fetchBoxAutoOff).mockResolvedValue({ ...baseStatus, enabled: true })
    await act(async () => {
      await result.current.setEnabled(true)
    })

    expect(client.updateServerSettings).toHaveBeenCalledWith({ box_auto_off: true })
    await waitFor(() => expect(result.current.status?.enabled).toBe(true))
  })
})
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/hooks/useBoxAutoOff.test.ts`
Expected: FAIL — the module does not exist.

- [ ] **Step 5: Write the hook**

Create `frontend/src/hooks/useBoxAutoOff.ts`:

```ts
import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchBoxAutoOff, updateServerSettings } from '../api/client'
import type { BoxAutoOffStatus } from '../types'

/** How often the auto-off state is re-fetched. */
const POLL_INTERVAL_MS = 15_000
/** How often the countdown re-renders between fetches. */
const TICK_INTERVAL_MS = 1_000

export interface UseBoxAutoOffResult {
  status: BoxAutoOffStatus | null
  loading: boolean
  error: string | null
  /** Seconds until the earliest possible shutdown, or null when there is none. */
  countdown: number | null
  refresh: () => Promise<void>
  setEnabled: (on: boolean) => Promise<void>
}

/**
 * useBoxAutoOff polls the auto-off endpoint and derives a live countdown from
 * `earliest_shutdown_at`.
 *
 * The countdown is computed locally on a one-second tick rather than being
 * fetched: the server only changes its estimate once per evaluation, so
 * polling at that resolution would be pointless traffic for a clock the
 * browser can run itself.
 */
export function useBoxAutoOff(): UseBoxAutoOffResult {
  const [status, setStatus] = useState<BoxAutoOffStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [countdown, setCountdown] = useState<number | null>(null)
  const inFlight = useRef(false)

  const refresh = useCallback(async () => {
    if (inFlight.current) return
    inFlight.current = true
    try {
      const data = await fetchBoxAutoOff()
      setStatus(data)
      setError(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load auto-off status')
    } finally {
      inFlight.current = false
      setLoading(false)
    }
  }, [])

  const setEnabled = useCallback(
    async (on: boolean) => {
      await updateServerSettings({ box_auto_off: on })
      await refresh()
    },
    [refresh],
  )

  useEffect(() => {
    void refresh()
    const timer = setInterval(() => void refresh(), POLL_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [refresh])

  // Derive the countdown from the target timestamp on every tick, rather than
  // decrementing a counter: a background tab throttles timers, and a
  // decremented counter would drift out of step with the real clock.
  useEffect(() => {
    const target = status?.earliest_shutdown_at
    if (!target) {
      setCountdown(null)
      return
    }
    const targetMs = new Date(target).getTime()
    const tick = () => setCountdown(Math.max(0, Math.round((targetMs - Date.now()) / 1000)))
    tick()
    const timer = setInterval(tick, TICK_INTERVAL_MS)
    return () => clearInterval(timer)
  }, [status?.earliest_shutdown_at])

  return { status, loading, error, countdown, refresh, setEnabled }
}
```

- [ ] **Step 6: Run the tests to verify they pass**

Run: `cd frontend && npx vitest run src/hooks/useBoxAutoOff.test.ts && npx tsc --noEmit`
Expected: PASS, no type errors.

- [ ] **Step 7: Commit**

```bash
git add frontend/src/types/index.ts frontend/src/api/client.ts frontend/src/hooks/useBoxAutoOff.ts frontend/src/hooks/useBoxAutoOff.test.ts
git commit -m "feat(frontend): box auto-off client, types and countdown hook"
```

---

### Task 13: The Box page card

**Files:**
- Create: `frontend/src/components/BoxAutoOffCard.tsx`, `frontend/src/components/BoxAutoOffCard.test.tsx`
- Modify: `frontend/src/pages/BoxPage.tsx`

**Interfaces:**
- Consumes: `useBoxAutoOff` (Task 12).
- Produces: `<BoxAutoOffCard />`, rendered by `BoxPage`.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/BoxAutoOffCard.test.tsx`. Mock the hook so the
card is tested on its own:

```tsx
import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import BoxAutoOffCard from './BoxAutoOffCard'
import * as hook from '../hooks/useBoxAutoOff'

vi.mock('../hooks/useBoxAutoOff')

const base = {
  status: {
    enabled: true,
    streak: 1,
    required_checks: 3,
    interval_seconds: 600,
    next_check_at: new Date(Date.now() + 300_000).toISOString(),
    earliest_shutdown_at: null,
    last_evaluation: null,
    recent: null,
    events: [],
  },
  loading: false,
  error: null,
  countdown: null,
  refresh: vi.fn(),
  setEnabled: vi.fn(),
}

describe('BoxAutoOffCard', () => {
  it('shows the countdown when there is one', () => {
    vi.mocked(hook.useBoxAutoOff).mockReturnValue({
      ...base,
      countdown: 1634,
      status: { ...base.status, earliest_shutdown_at: new Date().toISOString() },
    })

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/27:14/)).toBeInTheDocument()
  })

  it('names the blocker instead of a countdown when blocked', () => {
    vi.mocked(hook.useBoxAutoOff).mockReturnValue({
      ...base,
      status: {
        ...base.status,
        last_evaluation: {
          checked_at: new Date().toISOString(),
          idle: false,
          blockers: [{ name: 'kukatko_queue', detail: '12 queued, 1 running' }],
          box: null,
          kukatko: { status_code: 200, queued: 12, running: 1 },
        },
      },
    })

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/Kukátko/)).toBeInTheDocument()
    expect(screen.getByText(/12 queued, 1 running/)).toBeInTheDocument()
  })

  it('shows the Kukátko failure detail', () => {
    vi.mocked(hook.useBoxAutoOff).mockReturnValue({
      ...base,
      status: {
        ...base.status,
        last_evaluation: {
          checked_at: new Date().toISOString(),
          idle: false,
          blockers: [{ name: 'kukatko_queue', detail: 'queue could not be read: HTTP 502: bad gateway' }],
          box: null,
          kukatko: { status_code: 502, queued: 0, running: 0, error: 'bad gateway', body_excerpt: 'upstream is down' },
        },
      },
    })

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/502/)).toBeInTheDocument()
    expect(screen.getByText(/upstream is down/)).toBeInTheDocument()
  })

  it('shows the streak', () => {
    vi.mocked(hook.useBoxAutoOff).mockReturnValue({ ...base, status: { ...base.status, streak: 2 } })

    render(<BoxAutoOffCard />)

    expect(screen.getByText(/2\s*\/\s*3/)).toBeInTheDocument()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd frontend && npx vitest run src/components/BoxAutoOffCard.test.tsx`
Expected: FAIL — the component does not exist.

- [ ] **Step 3: Write the component**

Create `frontend/src/components/BoxAutoOffCard.tsx`. Match `BoxPage`'s existing
card styling (read it first — rounded borders, `text-sm`, lucide icons,
`clsx`). Requirements:

- A header with the title "Automatické vypínání" and a checkbox/switch bound
  to `setEnabled`, disabled while a write is in flight.
- The headline slot: `countdown !== null` renders `Vypnutí za MM:SS` (or
  `H:MM:SS` past an hour) with the subtitle `pokud zůstane klid`; otherwise it
  renders the first blocker as `Blokuje: <label> — <detail>`.
- `Další kontrola za …` from `next_check_at`, and `streak / required_checks`
  rendered as `2 / 3 klidných kontrol`.
- The full condition list from `last_evaluation.blockers`, each with a Czech
  label from this map (keys are the Go constants — do not invent new ones):

```ts
const BLOCKER_LABELS: Record<string, string> = {
  disabled: 'Automatické vypínání je vypnuté',
  box_offline: 'Box je nedostupný',
  uptime: 'Box běží krátce',
  botka_tasks: 'Botka zpracovává tasky',
  botka_box_chats: 'Na Boxu běží chat',
  kukatko_queue: 'Kukátko má frontu',
  gpu: 'GPU je vytížené',
  cpu: 'CPU je vytížené',
}
```

- The readings line when `last_evaluation.box` exists: uptime, `load1` with the
  thread count, GPU max %.
- The Kukátko line when `last_evaluation.kukatko` exists: `HTTP <status>`,
  queued/running, and `error` / `body_excerpt` when present. This is the
  requirement that the reason be visible *with the response*, so do not hide
  the excerpt behind a toggle.
- The last shutdown attempts from `events` — time and outcome
  (`shutdown` → "vypnuto", `failed` → "selhalo", `aborted` → "zrušeno").
- `error` from the hook rendered as an inline error line.

- [ ] **Step 4: Render it on the page**

In `frontend/src/pages/BoxPage.tsx`, import the card and render it after the
services section, inside the same container the other cards use.

- [ ] **Step 5: Run the tests to verify they pass**

Run: `cd frontend && npx vitest run src/components/BoxAutoOffCard.test.tsx src/pages/BoxPage.test.tsx && npx tsc --noEmit`
Expected: PASS, including the existing `BoxPage` test.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/components/BoxAutoOffCard.tsx frontend/src/components/BoxAutoOffCard.test.tsx frontend/src/pages/BoxPage.tsx
git commit -m "feat(frontend): box auto-off card with live countdown"
```

---

### Task 14: Documentation and the final gate

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Document the environment variables**

Add six rows to the environment-variable table in `CLAUDE.md`, in the same
style as the existing ones:

| Variable | Default | Description |
|---|---|---|
| `BOX_AUTO_OFF_INTERVAL` | `10m` | How often the Box auto-off monitor evaluates its conditions (Go duration) |
| `BOX_AUTO_OFF_MIN_UPTIME` | `2h` | Minimum Box uptime before it may be shut down automatically |
| `BOX_AUTO_OFF_IDLE_CHECKS` | `3` | Consecutive clean evaluations required before a shutdown fires |
| `BOX_AUTO_OFF_GPU_THRESHOLD` | `10` | GPU utilization percentage above which Box counts as busy |
| `BOX_AUTO_OFF_LOAD_THRESHOLD` | `1.0` | 1-minute load average above which Box counts as busy |
| `KUKATKO_METRICS_URL` | `https://fotky.kotrzina.cz/metrics` | Production Kukátko Prometheus endpoint read for job queue depth |

- [ ] **Step 2: Add the pattern entry**

Add a bullet to **Important Patterns** in `CLAUDE.md`:

> - **Box auto-off:** `internal/boxoff` shuts the Box build machine down once
>   it has been idle for `BOX_AUTO_OFF_IDLE_CHECKS` consecutive evaluations.
>   The switch is the `box_auto_off` row in `app_settings` (default off), not
>   an env var. Every unknown reading is a **blocker**, never a licence to
>   shut down: a failed SSH probe, an unparsable probe output, an unreachable
>   Kukátko endpoint and a failed activity lookup all keep Box on. Kukátko's
>   `/metrics` is only trusted when the body actually contains a
>   `kukatko_jobs_queue_depth{` sample — otherwise a renamed metric or an HTML
>   error page would read as "queue empty". GPU **utilization** is the busy
>   signal, not the presence of a compute process: the idle photo-enhancer and
>   image-embeddings services hold ~6 GB of VRAM permanently. Because
>   gathering readings takes seconds, the two cheap local conditions (running
>   tasks, Box chats) are re-checked immediately before the shutdown command;
>   a late task records an `aborted` event instead of powering the machine
>   off.

- [ ] **Step 3: Run the full gate**

Run: `make check`
Expected: PASS — formatting, vet, lint, `go test -race`, frontend type-check.

- [ ] **Step 4: Commit**

```bash
git add CLAUDE.md
git commit -m "docs: box auto-off configuration and behavior"
```

- [ ] **Step 5: Report**

Summarize for the user: what was built, the `make check` output as evidence,
and the reminder that **deployment is manual** — a task agent must never
restart the botka service. Tell them the feature ships **switched off**: it
does nothing until `box_auto_off` is turned on from the Box page.

---

## Self-Review Notes

- **Spec coverage.** Conditions 1–8 → Task 2; probe → Task 3; Kukátko trust
  guard → Task 4; thresholds/env → Task 5; events table → Task 6; tasks and
  Box chats → Task 7; streak, re-check, ring buffer, fail-safe → Task 8;
  setting → Task 9; API → Task 10; wiring → Task 11; countdown → Tasks 2, 12,
  13; UI card with the Kukátko response → Task 13; docs → Task 14. The
  `internal/box` extraction the spec calls for is Task 1.
- **Naming consistency.** `BoxReadings.Threads` (not `Cores`) throughout Go
  and TypeScript; `Blocker.Name` values come only from the Task 2 constants
  and the Task 13 label map keys match them exactly;
  `EarliestShutdownAt`/`earliest_shutdown_at` is the same value end to end.
- **Deviation from the spec, deliberate:** the spec's API sketch named the
  thread-count field `cores`; the probe reads `nproc`, which reports hardware
  threads (24 on a 12-core CPU), so the field is `threads` everywhere. The
  spec's prose already says "hardware threads".
