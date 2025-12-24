package shell

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"
)

type bashShell struct{}

// Bash is the Shell implementation for bash.
var Bash Shell = &bashShell{}

// bashHookTemplate is the template for the bash hook.
// It preserves exit status, traps SIGINT during eval, and handles
// PROMPT_COMMAND as both string and array.
const bashHookTemplate = `_cascade_hook() {
  local previous_exit_status=$?;
  trap -- '' SIGINT;
  eval "$("{{.SelfPath}}" export bash)";
  trap - SIGINT;
  return $previous_exit_status;
};
if [[ ";${PROMPT_COMMAND[*]:-};" != *";_cascade_hook;"* ]]; then
  if [[ "$(declare -p PROMPT_COMMAND 2>&1)" == "declare -a"* ]]; then
    PROMPT_COMMAND=(_cascade_hook "${PROMPT_COMMAND[@]}")
  else
    PROMPT_COMMAND="_cascade_hook${PROMPT_COMMAND:+;$PROMPT_COMMAND}"
  fi
fi
`

var bashHookTmpl = template.Must(template.New("bash-hook").Parse(bashHookTemplate))

func (b *bashShell) Name() string {
	return "bash"
}

func (b *bashShell) Hook(selfPath string) string {
	var buf bytes.Buffer
	data := struct {
		SelfPath string
	}{
		SelfPath: selfPath,
	}
	// Template is validated at init time, so this cannot fail.
	_ = bashHookTmpl.Execute(&buf, data)
	return buf.String()
}

func (b *bashShell) Export(e ShellExport) string {
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

func (b *bashShell) Dump(env map[string]string) string {
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
