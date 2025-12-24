package env

import "strings"

// ignoredKeys contains environment variables that should be excluded from diffs.
// These are shell-managed or session-specific variables that change frequently
// and are not meaningful to track.
var ignoredKeys = map[string]bool{
	"PWD":             true, // Current working directory
	"OLDPWD":          true, // Previous working directory
	"SHLVL":           true, // Shell nesting level
	"_":               true, // Last command executed
	"TERM_SESSION_ID": true, // Terminal session identifier
}

// IgnoredEnv returns true for env vars that should be excluded from diffs.
// This includes PWD, OLDPWD, SHLVL, _, TERM_SESSION_ID, and all CASCADE_* vars.
func IgnoredEnv(key string) bool {
	if ignoredKeys[key] {
		return true
	}
	// Ignore all CASCADE_* variables to prevent feedback loops
	return strings.HasPrefix(key, "CASCADE_")
}
