package shell

import (
	"strings"
	"testing"
)

func TestZshName(t *testing.T) {
	if got := Zsh.Name(); got != "zsh" {
		t.Errorf("Name() = %q, want %q", got, "zsh")
	}
}

func TestZshHook(t *testing.T) {
	hook := Zsh.Hook("/usr/local/bin/cascade")

	t.Run("contains _cascade_hook function", func(t *testing.T) {
		if !strings.Contains(hook, "_cascade_hook()") {
			t.Error("hook should contain _cascade_hook function definition")
		}
	})

	t.Run("contains selfPath", func(t *testing.T) {
		if !strings.Contains(hook, "/usr/local/bin/cascade") {
			t.Error("hook should contain the selfPath")
		}
	})

	t.Run("adds to precmd_functions", func(t *testing.T) {
		if !strings.Contains(hook, "precmd_functions") {
			t.Error("hook should reference precmd_functions")
		}
		// Hook is appended (not prepended) so it runs after _cascade_precmd_seq
		if !strings.Contains(hook, "precmd_functions+=(_cascade_hook)") {
			t.Error("hook should append _cascade_hook to precmd_functions")
		}
	})

	t.Run("adds to chpwd_functions", func(t *testing.T) {
		if !strings.Contains(hook, "chpwd_functions") {
			t.Error("hook should reference chpwd_functions")
		}
		if !strings.Contains(hook, "chpwd_functions=(_cascade_hook") {
			t.Error("hook should prepend _cascade_hook to chpwd_functions")
		}
	})

	t.Run("checks for duplicate registration", func(t *testing.T) {
		// Zsh uses (I) subscript flag to check if element exists
		if !strings.Contains(hook, "${precmd_functions[(I)_cascade_hook]}") {
			t.Error("hook should check if _cascade_hook already in precmd_functions")
		}
		if !strings.Contains(hook, "${chpwd_functions[(I)_cascade_hook]}") {
			t.Error("hook should check if _cascade_hook already in chpwd_functions")
		}
	})

	t.Run("traps SIGINT", func(t *testing.T) {
		if !strings.Contains(hook, "trap -- '' SIGINT") {
			t.Error("hook should trap SIGINT during eval")
		}
		if !strings.Contains(hook, "trap - SIGINT") {
			t.Error("hook should restore SIGINT trap after eval")
		}
	})

	t.Run("deduplicates precmd and chpwd execution", func(t *testing.T) {
		// Sequence incrementer must be registered
		if !strings.Contains(hook, "_cascade_precmd_seq()") {
			t.Error("hook should define _cascade_precmd_seq function")
		}
		if !strings.Contains(hook, "precmd_functions=(_cascade_precmd_seq") {
			t.Error("hook should prepend _cascade_precmd_seq to precmd_functions")
		}
		// Guard check must be present in _cascade_hook
		if !strings.Contains(hook, `[[ "$_cascade_last_run" == "$_cascade_prompt_seq" ]] && return`) {
			t.Error("hook should skip if already ran this prompt cycle")
		}
		if !strings.Contains(hook, "_cascade_last_run=$_cascade_prompt_seq") {
			t.Error("hook should record when it ran")
		}
	})
}

func TestZshExport(t *testing.T) {
	tests := []struct {
		name     string
		export   ShellExport
		contains []string
	}{
		{
			name:     "empty export",
			export:   ShellExport{},
			contains: nil,
		},
		{
			name: "set single variable",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("FOO", "bar")
				return e
			}(),
			contains: []string{`export FOO="bar";`},
		},
		{
			name: "unset single variable",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Unset("FOO")
				return e
			}(),
			contains: []string{`unset FOO;`},
		},
		{
			name: "set and unset multiple",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("PATH", "/usr/bin")
				e.Unset("OLD_VAR")
				e.Set("HOME", "/home/user")
				return e
			}(),
			contains: []string{
				`export PATH="/usr/bin";`,
				`unset OLD_VAR;`,
				`export HOME="/home/user";`,
			},
		},
		{
			name: "value with special characters",
			export: func() ShellExport {
				e := make(ShellExport)
				e.Set("MSG", `hello "world" $HOME`)
				return e
			}(),
			contains: []string{`export MSG="hello \"world\" \$HOME";`},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Zsh.Export(tt.export)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Export() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

func TestZshExportMatchesBash(t *testing.T) {
	// Zsh and bash use the same export/unset syntax
	e := make(ShellExport)
	e.Set("FOO", "bar")
	e.Set("PATH", "/usr/bin:/bin")
	e.Unset("OLD_VAR")

	zshOut := Zsh.Export(e)
	bashOut := Bash.Export(e)

	if zshOut != bashOut {
		t.Errorf("Zsh.Export() = %q, Bash.Export() = %q, should match", zshOut, bashOut)
	}
}

func TestZshExportDeterministic(t *testing.T) {
	e := make(ShellExport)
	e.Set("Z_VAR", "last")
	e.Set("A_VAR", "first")
	e.Set("M_VAR", "middle")

	got := Zsh.Export(e)

	// Check that A comes before M comes before Z
	aIdx := strings.Index(got, "A_VAR")
	mIdx := strings.Index(got, "M_VAR")
	zIdx := strings.Index(got, "Z_VAR")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("Export() output not sorted: A at %d, M at %d, Z at %d", aIdx, mIdx, zIdx)
	}
}

func TestZshDump(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		contains []string
	}{
		{
			name:     "empty env",
			env:      map[string]string{},
			contains: nil,
		},
		{
			name: "single variable",
			env: map[string]string{
				"FOO": "bar",
			},
			contains: []string{`export FOO="bar";`},
		},
		{
			name: "multiple variables",
			env: map[string]string{
				"PATH": "/usr/bin",
				"HOME": "/home/user",
			},
			contains: []string{
				`export PATH="/usr/bin";`,
				`export HOME="/home/user";`,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Zsh.Dump(tt.env)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Dump() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

func TestZshDumpMatchesBash(t *testing.T) {
	// Zsh and bash use the same dump syntax
	env := map[string]string{
		"FOO":  "bar",
		"PATH": "/usr/bin:/bin",
	}

	zshOut := Zsh.Dump(env)
	bashOut := Bash.Dump(env)

	if zshOut != bashOut {
		t.Errorf("Zsh.Dump() = %q, Bash.Dump() = %q, should match", zshOut, bashOut)
	}
}

func TestZshDumpDeterministic(t *testing.T) {
	env := map[string]string{
		"Z_VAR": "last",
		"A_VAR": "first",
		"M_VAR": "middle",
	}

	got := Zsh.Dump(env)

	// Check that A comes before M comes before Z
	aIdx := strings.Index(got, "A_VAR")
	mIdx := strings.Index(got, "M_VAR")
	zIdx := strings.Index(got, "Z_VAR")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("Dump() output not sorted: A at %d, M at %d, Z at %d", aIdx, mIdx, zIdx)
	}
}

func TestGetZsh(t *testing.T) {
	got := Get("zsh")
	if got == nil {
		t.Fatal("Get(\"zsh\") returned nil")
	}
	if got.Name() != "zsh" {
		t.Errorf("Get(\"zsh\").Name() = %q, want %q", got.Name(), "zsh")
	}
}

func TestSupportedIncludesZsh(t *testing.T) {
	supported := Supported()

	found := false
	for _, s := range supported {
		if s == "zsh" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Supported() should include 'zsh'")
	}
}
