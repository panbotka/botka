package claude

import (
	"os"
	"strings"
)

// sanitizedEnvPrefixes lists environment variable prefixes that should be
// stripped from Claude Code subprocesses. These are Botka-specific variables
// that could cause conflicts — most notably PORT, which caused a production
// crash when a child script read it and killed the process on that port.
var sanitizedEnvPrefixes = []string{
	"PORT=",
	"DATABASE_URL=",
	"MCP_TOKEN=",
	"SESSION_MAX_AGE=",
	"WEBAUTHN_ORIGIN=",
	"WEBAUTHN_RPID=",
}

// SanitizedEnv returns os.Environ() with Botka-specific variables removed.
// The returned slice can be assigned directly to cmd.Env. Additional variables
// can be appended to the result as needed.
func SanitizedEnv() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, e := range env {
		if isSanitized(e) {
			continue
		}
		filtered = append(filtered, e)
	}
	return filtered
}

// isSanitized returns true if the env var should be excluded from child processes.
func isSanitized(envVar string) bool {
	for _, prefix := range sanitizedEnvPrefixes {
		if strings.HasPrefix(envVar, prefix) {
			return true
		}
	}
	return false
}
