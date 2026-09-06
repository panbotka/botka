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
