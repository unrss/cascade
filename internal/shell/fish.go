package shell

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"text/template"
)

type fishShell struct{}

// Fish is the Shell implementation for fish.
var Fish Shell = &fishShell{}

// fishHookTemplate is the template for the fish hook.
// It uses fish's event system to trigger on prompt and directory changes.
// The PWD variable hook handles cd, pushd, popd, and any other directory changes.
const fishHookTemplate = `function __cascade_export_eval --on-event fish_prompt
    "{{.SelfPath}}" export fish | source
end

function __cascade_cd_hook --on-variable PWD
    if test "$CASCADE_FISH_MODE" != "disable_arrow"
        __cascade_export_eval
    end
end
`

var fishHookTmpl = template.Must(template.New("fish-hook").Parse(fishHookTemplate))

func (f *fishShell) Name() string {
	return "fish"
}

func (f *fishShell) Hook(selfPath string) string {
	var buf bytes.Buffer
	data := struct {
		SelfPath string
	}{
		SelfPath: selfPath,
	}
	// Template is validated at init time, so this cannot fail.
	_ = fishHookTmpl.Execute(&buf, data)
	return buf.String()
}

func (f *fishShell) Export(e ShellExport) string {
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
			fmt.Fprintf(&sb, "set -e %s;\n", key)
		} else {
			fmt.Fprintf(&sb, "set -gx %s '%s';\n", key, FishEscape(*value))
		}
	}

	return sb.String()
}

func (f *fishShell) Dump(env map[string]string) string {
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
		fmt.Fprintf(&sb, "set -gx %s '%s';\n", key, FishEscape(env[key]))
	}

	return sb.String()
}
