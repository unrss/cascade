package shell

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"
)

type zshShell struct{}

// Zsh is the Shell implementation for zsh.
var Zsh Shell = &zshShell{}

// zshHookTemplate is the template for the zsh hook.
// It uses precmd_functions and chpwd_functions arrays to run the hook
// before each prompt and on directory change.
//
// Deduplication: Both chpwd and precmd fire when the user runs `cd`, which
// would cause the hook to run twice per prompt. We use a prompt sequence
// counter (_cascade_prompt_seq) that increments before each prompt via
// _cascade_precmd_seq. The hook checks if it already ran this cycle by
// comparing _cascade_last_run to the current sequence, skipping if equal.
// This ensures the hook runs exactly once per prompt, regardless of whether
// chpwd triggered it first.
//
// The hook traps SIGINT during eval to prevent interruption of environment
// updates.
const zshHookTemplate = `_cascade_precmd_seq() { (( ++_cascade_prompt_seq )) }

_cascade_hook() {
  [[ "$_cascade_last_run" == "$_cascade_prompt_seq" ]] && return
  _cascade_last_run=$_cascade_prompt_seq

  trap -- '' SIGINT
  eval "$("{{.SelfPath}}" export zsh)"
  trap - SIGINT
}

typeset -ag precmd_functions
if (( ! ${precmd_functions[(I)_cascade_precmd_seq]} )); then
  precmd_functions=(_cascade_precmd_seq $precmd_functions)
fi
if (( ! ${precmd_functions[(I)_cascade_hook]} )); then
  precmd_functions+=(_cascade_hook)
fi
typeset -ag chpwd_functions
if (( ! ${chpwd_functions[(I)_cascade_hook]} )); then
  chpwd_functions=(_cascade_hook $chpwd_functions)
fi
`

var zshHookTmpl = template.Must(template.New("zsh-hook").Parse(zshHookTemplate))

func (z *zshShell) Name() string {
	return "zsh"
}

func (z *zshShell) Hook(selfPath string) string {
	var buf bytes.Buffer
	data := struct {
		SelfPath string
	}{
		SelfPath: selfPath,
	}
	// Template is validated at init time, so this cannot fail.
	_ = zshHookTmpl.Execute(&buf, data)
	return buf.String()
}

// Export formats environment changes as shell commands.
// Zsh uses the same export/unset syntax as bash.
func (z *zshShell) Export(e ShellExport) string {
	if len(e) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(e))
	for k := range e {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var sb strings.Builder
	for _, key := range keys {
		value := e[key]
		if value == nil {
			fmt.Fprintf(&sb, "unset %s;\n", key)
		} else {
			fmt.Fprintf(&sb, "export %s=\"%s\";\n", key, BashEscape(*value))
		}
	}

	return sb.String()
}

// Dump formats a complete environment as shell commands.
// Zsh uses the same export syntax as bash.
func (z *zshShell) Dump(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}

	// Sort keys for deterministic output
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	slices.Sort(keys)

	var sb strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&sb, "export %s=\"%s\";\n", key, BashEscape(env[key]))
	}

	return sb.String()
}
