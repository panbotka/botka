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
