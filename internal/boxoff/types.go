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
