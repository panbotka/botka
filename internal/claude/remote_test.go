package claude

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestIsRemotePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{"box:/home/box/app", true},
		{"box:", true},
		{"/home/pi/projects/botka", false},
		{"", false},
		{"boxy:/nope", false},
	}
	for _, tt := range tests {
		if got := IsRemotePath(tt.in); got != tt.want {
			t.Errorf("IsRemotePath(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestSplitRemotePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in         string
		wantPath   string
		wantRemote bool
	}{
		{"box:/home/box/app", "/home/box/app", true},
		{"/home/pi/projects/botka", "/home/pi/projects/botka", false},
		{"box:relative/path", "relative/path", true},
	}
	for _, tt := range tests {
		gotPath, gotRemote := SplitRemotePath(tt.in)
		if gotPath != tt.wantPath || gotRemote != tt.wantRemote {
			t.Errorf("SplitRemotePath(%q) = (%q,%v), want (%q,%v)",
				tt.in, gotPath, gotRemote, tt.wantPath, tt.wantRemote)
		}
	}
}

func TestShellQuote(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
	}{
		{"", "''"},
		{"plain", "'plain'"},
		{"with space", "'with space'"},
		{"with'quote", `'with'\''quote'`},
		{"--flag=value", "'--flag=value'"},
	}
	for _, tt := range tests {
		if got := shellQuote(tt.in); got != tt.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBuildSSHArgs_PrefixAndDestination(t *testing.T) {
	t.Parallel()
	args := BuildSSHArgs("box@box", "/home/box/projects/app", "claude",
		[]string{"-p", "--verbose"}, "hello there")
	if args[0] != "ssh" {
		t.Errorf("argv[0] = %q, want ssh", args[0])
	}
	// SSH destination should appear exactly once and be followed by the
	// assembled remote command.
	dstIdx := -1
	for i, a := range args {
		if a == "box@box" {
			dstIdx = i
			break
		}
	}
	if dstIdx == -1 {
		t.Fatalf("destination box@box not found in args: %v", args)
	}
	if dstIdx != len(args)-2 {
		t.Errorf("destination should be the penultimate arg, got index %d of %d", dstIdx, len(args))
	}
	remoteCmd := args[len(args)-1]
	if !strings.HasPrefix(remoteCmd, "cd '/home/box/projects/app'") {
		t.Errorf("remote command should cd into working dir, got %q", remoteCmd)
	}
	if !strings.Contains(remoteCmd, "exec 'claude'") {
		t.Errorf("remote command should exec claude, got %q", remoteCmd)
	}
	if !strings.Contains(remoteCmd, "'-p'") || !strings.Contains(remoteCmd, "'--verbose'") {
		t.Errorf("remote command missing claude flags: %q", remoteCmd)
	}
	if !strings.HasSuffix(remoteCmd, "'hello there'") {
		t.Errorf("remote command should end with the prompt, got %q", remoteCmd)
	}
}

func TestBuildSSHArgs_QuotesPromptWithQuotes(t *testing.T) {
	t.Parallel()
	args := BuildSSHArgs("box@box", "/dir", "claude", nil, "it's tricky")
	remote := args[len(args)-1]
	if !strings.HasSuffix(remote, `'it'\''s tricky'`) {
		t.Errorf("prompt not quoted correctly, got %q", remote)
	}
}

func TestBuildSSHArgs_OptionsOrdering(t *testing.T) {
	t.Parallel()
	args := BuildSSHArgs("box@box", "/dir", "claude", nil, "x")
	// The first few args after "ssh" should be SSH options, not the target.
	wantPrefix := []string{"ssh", "-o", "BatchMode=yes"}
	if !reflect.DeepEqual(args[:len(wantPrefix)], wantPrefix) {
		t.Errorf("prefix = %v, want %v", args[:len(wantPrefix)], wantPrefix)
	}
}

func TestSessionExists_RemoteSkipsCheck(t *testing.T) {
	t.Parallel()
	// A session file cannot exist locally for a remote path; SessionExists
	// should short-circuit and return true so the caller proceeds to SSH.
	if !SessionExists("fake-session-id", "box:/home/box/projects/app") {
		t.Error("SessionExists(remote) = false, want true")
	}
}

func TestBuildRunCmd_RemoteUsesSSH(t *testing.T) {
	t.Parallel()
	cfg := RunConfig{
		ClaudePath: "claude",
		WorkDir:    "box:/home/box/projects/app",
		SessionID:  "abc",
		Remote: &RemoteSpec{
			SSHTarget: "box@box",
			// Waker nil: ensureRemoteUp is a no-op in tests.
		},
	}
	cmd, err := buildRunCmd(context.Background(), cfg, "do a thing")
	if err != nil {
		t.Fatalf("buildRunCmd() error = %v", err)
	}
	// The local argv[0] should be ssh; cmd.Path resolves to /usr/bin/ssh.
	if !strings.HasSuffix(cmd.Path, "/ssh") && cmd.Path != "ssh" {
		t.Errorf("cmd.Path = %q, want ssh", cmd.Path)
	}
	// The destination box@box must appear in args.
	found := false
	for _, a := range cmd.Args {
		if a == "box@box" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("cmd.Args missing box@box destination: %v", cmd.Args)
	}
	// The remote command (last arg) must cd into the stripped path.
	last := cmd.Args[len(cmd.Args)-1]
	if !strings.Contains(last, "/home/box/projects/app") {
		t.Errorf("remote command missing stripped path, got %q", last)
	}
	if strings.Contains(last, "box:/home/box") {
		t.Errorf("remote command still contains 'box:' prefix: %q", last)
	}
}

func TestBuildRunCmd_LocalPreservesWorkDir(t *testing.T) {
	t.Parallel()
	cfg := RunConfig{
		ClaudePath: "/usr/local/bin/claude",
		WorkDir:    "/home/pi/projects/app",
	}
	cmd, err := buildRunCmd(context.Background(), cfg, "hello")
	if err != nil {
		t.Fatalf("buildRunCmd() error = %v", err)
	}
	if cmd.Dir != "/home/pi/projects/app" {
		t.Errorf("cmd.Dir = %q, want /home/pi/projects/app", cmd.Dir)
	}
	if cmd.Path != "/usr/local/bin/claude" {
		t.Errorf("cmd.Path = %q, want /usr/local/bin/claude", cmd.Path)
	}
	// The prompt should be the last argument for local runs.
	if got := cmd.Args[len(cmd.Args)-1]; got != "hello" {
		t.Errorf("last arg = %q, want hello", got)
	}
}

func TestBuildRunCmd_RemoteEmptyPath(t *testing.T) {
	t.Parallel()
	cfg := RunConfig{
		ClaudePath: "claude",
		WorkDir:    "box:",
		Remote: &RemoteSpec{
			SSHTarget: "box@box",
		},
	}
	_, err := buildRunCmd(context.Background(), cfg, "x")
	if err == nil {
		t.Error("buildRunCmd(empty remote path) err = nil, want error")
	}
}
