package shell

import (
	"strings"
	"testing"
)

func TestBashName(t *testing.T) {
	if got := Bash.Name(); got != "bash" {
		t.Errorf("Name() = %q, want %q", got, "bash")
	}
}

func TestBashHook(t *testing.T) {
	hook := Bash.Hook("/usr/local/bin/cascade")

	t.Run("contains _cascade_hook function", func(t *testing.T) {
		if !strings.Contains(hook, "_cascade_hook()") {
			t.Error("hook should contain _cascade_hook function definition")
		}
	})

	t.Run("preserves exit status", func(t *testing.T) {
		if !strings.Contains(hook, "previous_exit_status=$?") {
			t.Error("hook should save previous exit status")
		}
		if !strings.Contains(hook, "return $previous_exit_status") {
			t.Error("hook should return previous exit status")
		}
	})

	t.Run("contains selfPath", func(t *testing.T) {
		if !strings.Contains(hook, "/usr/local/bin/cascade") {
			t.Error("hook should contain the selfPath")
		}
	})

	t.Run("handles PROMPT_COMMAND array", func(t *testing.T) {
		if !strings.Contains(hook, "declare -p PROMPT_COMMAND") {
			t.Error("hook should check if PROMPT_COMMAND is an array")
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
}

func TestBashExport(t *testing.T) {
	tests := []struct {
		name     string
		export   ShellExport
		contains []string
		excludes []string
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
			got := Bash.Export(tt.export)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Export() = %q, should contain %q", got, want)
				}
			}
			for _, exclude := range tt.excludes {
				if strings.Contains(got, exclude) {
					t.Errorf("Export() = %q, should not contain %q", got, exclude)
				}
			}
		})
	}
}

func TestBashExportDeterministic(t *testing.T) {
	e := make(ShellExport)
	e.Set("Z_VAR", "last")
	e.Set("A_VAR", "first")
	e.Set("M_VAR", "middle")

	got := Bash.Export(e)

	// Check that A comes before M comes before Z
	aIdx := strings.Index(got, "A_VAR")
	mIdx := strings.Index(got, "M_VAR")
	zIdx := strings.Index(got, "Z_VAR")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("Export() output not sorted: A at %d, M at %d, Z at %d", aIdx, mIdx, zIdx)
	}
}

func TestBashDump(t *testing.T) {
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
			got := Bash.Dump(tt.env)
			for _, want := range tt.contains {
				if !strings.Contains(got, want) {
					t.Errorf("Dump() = %q, should contain %q", got, want)
				}
			}
		})
	}
}

func TestBashDumpDeterministic(t *testing.T) {
	env := map[string]string{
		"Z_VAR": "last",
		"A_VAR": "first",
		"M_VAR": "middle",
	}

	got := Bash.Dump(env)

	// Check that A comes before M comes before Z
	aIdx := strings.Index(got, "A_VAR")
	mIdx := strings.Index(got, "M_VAR")
	zIdx := strings.Index(got, "Z_VAR")

	if aIdx > mIdx || mIdx > zIdx {
		t.Errorf("Dump() output not sorted: A at %d, M at %d, Z at %d", aIdx, mIdx, zIdx)
	}
}

func TestBashEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "simple string",
			input: "hello",
			want:  "hello",
		},
		{
			name:  "double quotes",
			input: `say "hello"`,
			want:  `say \"hello\"`,
		},
		{
			name:  "backslash",
			input: `path\to\file`,
			want:  `path\\to\\file`,
		},
		{
			name:  "dollar sign",
			input: "$HOME/bin",
			want:  `\$HOME/bin`,
		},
		{
			name:  "backtick",
			input: "echo `date`",
			want:  "echo \\`date\\`",
		},
		{
			name:  "newline",
			input: "line1\nline2",
			want:  `line1\nline2`,
		},
		{
			name:  "carriage return",
			input: "line1\rline2",
			want:  `line1\rline2`,
		},
		{
			name:  "tab",
			input: "col1\tcol2",
			want:  `col1\tcol2`,
		},
		{
			name:  "combined special chars",
			input: "echo \"$HOME\"\n`date`",
			want:  `echo \"\$HOME\"\n\` + "`date\\`",
		},
		{
			name:  "empty string",
			input: "",
			want:  "",
		},
		{
			name:  "unicode",
			input: "héllo wörld 日本語",
			want:  "héllo wörld 日本語",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BashEscape(tt.input)
			if got != tt.want {
				t.Errorf("BashEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGet(t *testing.T) {
	tests := []struct {
		name      string
		shellName string
		wantNil   bool
	}{
		{"bash", "bash", false},
		{"unsupported", "powershell", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Get(tt.shellName)
			if (got == nil) != tt.wantNil {
				t.Errorf("Get(%q) nil = %v, want nil = %v", tt.shellName, got == nil, tt.wantNil)
			}
			if got != nil && got.Name() != tt.shellName {
				t.Errorf("Get(%q).Name() = %q, want %q", tt.shellName, got.Name(), tt.shellName)
			}
		})
	}
}

func TestSupported(t *testing.T) {
	supported := Supported()

	// Should contain bash
	found := false
	for _, s := range supported {
		if s == "bash" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Supported() should include 'bash'")
	}
}

func TestShellExportSetUnset(t *testing.T) {
	e := make(ShellExport)

	// Test Set
	e.Set("FOO", "bar")
	if e["FOO"] == nil {
		t.Error("Set should store non-nil pointer")
	}
	if *e["FOO"] != "bar" {
		t.Errorf("Set stored %q, want %q", *e["FOO"], "bar")
	}

	// Test Unset
	e.Unset("BAZ")
	if _, ok := e["BAZ"]; !ok {
		t.Error("Unset should add key to map")
	}
	if e["BAZ"] != nil {
		t.Error("Unset should store nil pointer")
	}

	// Test overwrite with Unset
	e.Unset("FOO")
	if e["FOO"] != nil {
		t.Error("Unset should overwrite previous Set with nil")
	}
}
