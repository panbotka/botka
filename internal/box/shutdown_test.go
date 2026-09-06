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
