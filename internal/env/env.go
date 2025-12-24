// Package env provides environment variable types and operations for cascade.
package env

import (
	"slices"
	"strings"
)

// Env represents environment variables as a map.
type Env map[string]string

// FromGoEnv creates an Env from os.Environ() format ([]string{"KEY=value"}).
// Entries without an "=" are ignored. Empty values are preserved.
func FromGoEnv(environ []string) Env {
	env := make(Env, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		env[key] = value
	}
	return env
}

// ToGoEnv converts to os.Environ() format for exec.Cmd.Env.
// Keys are sorted for deterministic output.
func (e Env) ToGoEnv() []string {
	if e == nil {
		return nil
	}
	result := make([]string, 0, len(e))
	for key, value := range e {
		result = append(result, key+"="+value)
	}
	slices.Sort(result)
	return result
}

// Copy returns a deep copy of the environment.
func (e Env) Copy() Env {
	if e == nil {
		return nil
	}
	cp := make(Env, len(e))
	for k, v := range e {
		cp[k] = v
	}
	return cp
}

// Filtered returns a copy with ignored keys removed.
func (e Env) Filtered() Env {
	if e == nil {
		return nil
	}
	filtered := make(Env, len(e))
	for k, v := range e {
		if !IgnoredEnv(k) {
			filtered[k] = v
		}
	}
	return filtered
}
