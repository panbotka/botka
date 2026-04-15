package models

import (
	"testing"
)

func TestMCPServerType_IsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serverType MCPServerType
		want       bool
	}{
		{name: "stdio is valid", serverType: MCPServerTypeStdio, want: true},
		{name: "sse is valid", serverType: MCPServerTypeSSE, want: true},
		{name: "empty string is invalid", serverType: MCPServerType(""), want: false},
		{name: "arbitrary string is invalid", serverType: MCPServerType("grpc"), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.serverType.IsValid(); got != tt.want {
				t.Errorf("MCPServerType(%q).IsValid() = %v, want %v", tt.serverType, got, tt.want)
			}
		})
	}
}

func TestMCPServerType_Scan(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   interface{}
		want    MCPServerType
		wantErr bool
	}{
		{name: "valid stdio", input: "stdio", want: MCPServerTypeStdio, wantErr: false},
		{name: "valid sse", input: "sse", want: MCPServerTypeSSE, wantErr: false},
		{name: "invalid string", input: "grpc", want: MCPServerType(""), wantErr: true},
		{name: "non-string type", input: 42, want: MCPServerType(""), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var st MCPServerType
			err := st.Scan(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Scan(%v) error = %v, wantErr %v", tt.input, err, tt.wantErr)
				return
			}
			if err == nil && st != tt.want {
				t.Errorf("Scan(%v) = %q, want %q", tt.input, st, tt.want)
			}
		})
	}
}

func TestMCPServerType_Value(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		serverType MCPServerType
		want       string
		wantErr    bool
	}{
		{name: "stdio produces string", serverType: MCPServerTypeStdio, want: "stdio", wantErr: false},
		{name: "sse produces string", serverType: MCPServerTypeSSE, want: "sse", wantErr: false},
		{name: "invalid produces error", serverType: MCPServerType("grpc"), want: "", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			val, err := tt.serverType.Value()
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
