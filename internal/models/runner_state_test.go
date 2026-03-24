package models

import (
	"testing"
)

// TestRunnerStateType_IsValid verifies that all defined runner states are
// recognized as valid and that arbitrary strings are rejected.
func TestRunnerStateType_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state RunnerStateType
		want  bool
	}{
		{name: "running is valid", state: StateRunning, want: true},
		{name: "paused is valid", state: StatePaused, want: true},
		{name: "stopped is valid", state: StateStopped, want: true},
		{name: "empty string is invalid", state: RunnerStateType(""), want: false},
		{name: "arbitrary string is invalid", state: RunnerStateType("bogus"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.state.IsValid(); got != tt.want {
				t.Errorf("RunnerStateType(%q).IsValid() = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

// TestRunnerStateType_Scan verifies that scanning database values into
// RunnerStateType works for valid strings and returns errors for invalid inputs.
func TestRunnerStateType_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		want    RunnerStateType
		wantErr bool
	}{
		{name: "valid running", input: "running", want: StateRunning, wantErr: false},
		{name: "valid stopped", input: "stopped", want: StateStopped, wantErr: false},
		{name: "invalid string", input: "bogus", want: RunnerStateType(""), wantErr: true},
		{name: "non-string type", input: 42, want: RunnerStateType(""), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var s RunnerStateType
			err := s.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err == nil && s != tt.want {
				t.Errorf("Scan(%v) = %q, want %q", tt.input, s, tt.want)
			}
		})
	}
}

// TestRunnerStateType_Value verifies that RunnerStateType produces a valid
// driver.Value for recognized states and returns an error for invalid ones.
func TestRunnerStateType_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		state   RunnerStateType
		want    string
		wantErr bool
	}{
		{name: "running produces string", state: StateRunning, want: "running", wantErr: false},
		{name: "paused produces string", state: StatePaused, want: "paused", wantErr: false},
		{name: "invalid produces error", state: RunnerStateType("bogus"), want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, err := tt.state.Value()
			if (err != nil) != tt.wantErr {
				t.Errorf("Value() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err == nil {
				str, ok := val.(string)
				if !ok {
					t.Errorf("Value() returned %T, want string", val)
				} else if str != tt.want {
					t.Errorf("Value() = %q, want %q", str, tt.want)
				}
			}
		})
	}
}

// TestRunnerState_TableName verifies that RunnerState returns the correct table name.
func TestRunnerState_TableName(t *testing.T) {
	t.Parallel()
	rs := RunnerState{}
	if got := rs.TableName(); got != "runner_state" {
		t.Errorf("RunnerState.TableName() = %q, want %q", got, "runner_state")
	}
}
