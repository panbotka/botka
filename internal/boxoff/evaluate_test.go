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
