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
